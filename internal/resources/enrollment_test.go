package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/airbytehq/airbyte-cli/internal/auth"
	"github.com/airbytehq/airbyte-cli/internal/client"
)

func newTestTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"token_type":   "bearer",
			"expires_in":   1200,
		})
	}))
}

func newTestClient(t *testing.T, apiServer *httptest.Server) (c *client.Client, cleanup func()) {
	t.Helper()
	tokenServer := newTestTokenServer(t)
	creds := &auth.Credentials{ClientID: "id", ClientSecret: "secret"}
	tm := auth.NewTokenManager(tokenServer.URL, "", creds)
	c = client.New(apiServer.URL, "org-123", "test", tm)
	return c, func() { tokenServer.Close() }
}

func TestEnrollmentStatus(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/internal/account/enrollment-status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"is_enrolled": true, "provisioning_state": "COMPLETED"}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	res := &enrollmentResource{}
	ops := res.Operations()
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Name != "status" {
		t.Errorf("expected operation name 'status', got %q", ops[0].Name)
	}

	result, err := ops[0].Run(context.Background(), c, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw, ok := result.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", result)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}
	if parsed["is_enrolled"] != true {
		t.Errorf("expected is_enrolled=true, got %v", parsed["is_enrolled"])
	}
}

func TestOrganizationsList(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/internal/account/organizations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "org-1", "name": "Test Org"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	res := &organizationsResource{}
	ops := res.Operations()

	result, err := ops[0].Run(context.Background(), c, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw, ok := result.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", result)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	data, ok := parsed["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected 1 org, got %v", parsed["data"])
	}
}

func TestWorkspacesListPagination(t *testing.T) {
	callCount := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount == 1 {
			next := "http://" + r.Host + "/api/v1/workspaces?cursor=page2"
			resp := map[string]any{
				"data": []map[string]string{{"name": "ws-1"}},
				"next": next,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		resp := map[string]any{
			"data": []map[string]string{{"name": "ws-2"}},
			"next": nil,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	result, err := listWorkspaces(context.Background(), c, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	data, ok := resultMap["data"].([]json.RawMessage)
	if !ok {
		t.Fatalf("expected []json.RawMessage, got %T", resultMap["data"])
	}

	if len(data) != 2 {
		t.Fatalf("expected 2 workspaces across pages, got %d", len(data))
	}

	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestWorkspacesListWithFilters(t *testing.T) {
	var gotQuery string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [], "next": null}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	_, err := listWorkspaces(context.Background(), c, map[string]any{
		"name_contains": "test",
		"status":        "active",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotQuery == "" {
		t.Error("expected query params, got empty")
	}
}

func TestResourceMetadata(t *testing.T) {
	tests := []struct {
		resource interface {
			Name() string
			Description() string
		}
		wantName string
		wantDesc string
	}{
		{&enrollmentResource{}, "enrollment", "Manage account enrollment"},
		{&organizationsResource{}, "organizations", "Manage organizations"},
		{&workspacesResource{}, "workspaces", "Manage workspaces"},
		{&connectorsResource{}, "connectors", "Manage connectors"},
		{&skillsResource{}, "skills", "Agent skill documents"},
	}

	for _, tt := range tests {
		if tt.resource.Name() != tt.wantName {
			t.Errorf("expected name %q, got %q", tt.wantName, tt.resource.Name())
		}
		if tt.resource.Description() != tt.wantDesc {
			t.Errorf("expected desc %q, got %q", tt.wantDesc, tt.resource.Description())
		}
	}
}

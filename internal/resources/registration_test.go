package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/airbytehq/airbyte-agent-cli/internal/auth"
	"github.com/airbytehq/airbyte-agent-cli/internal/client"
	"github.com/airbytehq/airbyte-agent-cli/internal/registry"
	"github.com/airbytehq/airbyte-agent-cli/internal/spec"
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

func TestUseWorkspace_UpdatesSettingsFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Pre-populate settings.json with an existing config so 'use' has
	// something to update.
	if err := auth.WriteSettingsFile(&auth.Settings{
		Credentials:    auth.Credentials{ClientID: "id", ClientSecret: "secret"},
		OrganizationID: "org",
		Workspace:      "old-ws",
	}); err != nil {
		t.Fatalf("seeding settings: %v", err)
	}

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// useWorkspace verifies the name exists by calling the workspaces list.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "ws-1", "name": "Production"}, {"id": "ws-2", "name": "staging"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	// Type "production" (lowercase). The API returned "Production" — we
	// should save the canonical case.
	result, err := useWorkspace(context.Background(), c, map[string]any{"name": "production"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if resMap["workspace"] != "Production" {
		t.Errorf("expected canonical workspace=Production, got %v", resMap["workspace"])
	}

	// Confirm it persisted to disk.
	loaded, err := auth.ReadSettingsFile()
	if err != nil {
		t.Fatalf("re-reading settings: %v", err)
	}
	if loaded.Workspace != "Production" {
		t.Errorf("settings.json workspace = %q, want %q", loaded.Workspace, "Production")
	}
	// Other fields preserved.
	if loaded.Credentials.ClientID != "id" || loaded.OrganizationID != "org" {
		t.Errorf("other fields lost: %+v", loaded)
	}
}

func TestUseWorkspace_NotFound(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := auth.WriteSettingsFile(&auth.Settings{
		Credentials:    auth.Credentials{ClientID: "id", ClientSecret: "secret"},
		OrganizationID: "org",
	}); err != nil {
		t.Fatalf("seeding settings: %v", err)
	}

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": []}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	_, err := useWorkspace(context.Background(), c, map[string]any{"name": "missing"})
	if err == nil {
		t.Fatal("expected not_found error, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
}

func TestUseWorkspace_MissingName(t *testing.T) {
	_, err := useWorkspace(context.Background(), nil, map[string]any{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
}

func TestUseWorkspace_NoSettingsFile(t *testing.T) {
	// Empty HOME → settings.json doesn't exist.
	t.Setenv("HOME", t.TempDir())

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "ws-1", "name": "Production"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	_, err := useWorkspace(context.Background(), c, map[string]any{"name": "Production"})
	if err == nil {
		t.Fatal("expected error when settings.json doesn't exist")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
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
		{&organizationsResource{}, "organizations", "Manage organizations"},
		{&workspacesResource{}, "workspaces", "Manage workspaces"},
		{&connectorsResource{}, "connectors", "Create, manage, and execute connectors"},
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

// TestEveryOpSpecRefResolves asserts that every registered Operation with a
// non-zero SpecRef points at a route that was extracted into the embedded
// schema map. Catches the case where someone adds an op but forgets to
// re-run `go generate ./...`.
func TestEveryOpSpecRefResolves(t *testing.T) {
	resources := []registry.Resource{
		&organizationsResource{},
		&workspacesResource{},
		&connectorsResource{},
	}
	for _, res := range resources {
		for _, op := range res.Operations() {
			if op.SpecRef.IsZero() {
				continue
			}
			if _, ok := spec.Lookup(op.SpecRef.Key()); !ok {
				t.Errorf("operation %q on resource %q has SpecRef %q but no extracted schema; run `go generate ./...`",
					op.Name, res.Name(), op.SpecRef.Key())
			}
		}
	}
}

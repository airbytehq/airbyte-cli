package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/airbytehq/airbyte-cli/internal/client"
)

func TestApplyDefaultWorkspace_Empty(t *testing.T) {
	var stderr bytes.Buffer
	prev := statusWriter
	statusWriter = &stderr
	defer func() { statusWriter = prev }()

	params := map[string]any{}
	got := applyDefaultWorkspace(params)
	if got != "default" {
		t.Errorf("expected 'default', got %q", got)
	}
	if params["workspace"] != "default" {
		t.Errorf("expected params['workspace']='default', got %v", params["workspace"])
	}

	var notice map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &notice); err != nil {
		t.Fatalf("expected JSON notice on stderr, got %q (err: %v)", stderr.String(), err)
	}
	if notice["workspace"] != "default" {
		t.Errorf("notice missing workspace=default: %v", notice)
	}
}

func TestApplyDefaultWorkspace_Provided(t *testing.T) {
	var stderr bytes.Buffer
	prev := statusWriter
	statusWriter = &stderr
	defer func() { statusWriter = prev }()

	params := map[string]any{"workspace": "explicit-ws"}
	got := applyDefaultWorkspace(params)
	if got != "explicit-ws" {
		t.Errorf("expected 'explicit-ws', got %q", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no notice when workspace provided, got %q", stderr.String())
	}
}

func TestResolveConnectorID_ByID(t *testing.T) {
	params := map[string]any{"id": "conn-123"}
	result, err := resolveConnectorID(context.Background(), nil, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "conn-123" {
		t.Errorf("expected id=conn-123, got %v", result["id"])
	}
}

func TestResolveConnectorID_MissingNameAndID(t *testing.T) {
	params := map[string]any{}
	_, err := resolveConnectorID(context.Background(), nil, params)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
}

func TestResolveConnectorID_DefaultsWorkspaceWhenMissing(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("workspace_name"); got != "default" {
			t.Errorf("expected workspace_name=default, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "conn-xyz", "name": "my-connector"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	var stderr bytes.Buffer
	prev := statusWriter
	statusWriter = &stderr
	defer func() { statusWriter = prev }()

	params := map[string]any{"name": "my-connector"}
	result, err := resolveConnectorID(context.Background(), c, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "conn-xyz" {
		t.Errorf("expected id=conn-xyz, got %v", result["id"])
	}
	if result["workspace"] != "default" {
		t.Errorf("expected workspace='default' on params after fallback, got %v", result["workspace"])
	}
	if !strings.Contains(stderr.String(), "falling back") {
		t.Errorf("expected fallback notice on stderr, got %q", stderr.String())
	}
}

func TestResolveConnectorID_FoundOne(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("workspace_name") != "my-workspace" {
			t.Errorf("expected workspace_name=my-workspace, got %s", r.URL.Query().Get("workspace_name"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "conn-abc", "name": "My Connector"}, {"id": "conn-def", "name": "Other"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	params := map[string]any{"name": "my connector", "workspace": "my-workspace"}
	result, err := resolveConnectorID(context.Background(), c, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "conn-abc" {
		t.Errorf("expected id=conn-abc, got %v", result["id"])
	}
}

func TestResolveConnectorID_NotFound(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "conn-abc", "name": "Other Connector"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	params := map[string]any{"name": "missing", "workspace": "ws"}
	_, err := resolveConnectorID(context.Background(), c, params)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
}

func TestResolveConnectorID_Ambiguous(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "conn-1", "name": "Dup"}, {"id": "conn-2", "name": "dup"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	params := map[string]any{"name": "dup", "workspace": "ws"}
	_, err := resolveConnectorID(context.Background(), c, params)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
	if apiErr.Type != "validation_error" {
		t.Errorf("expected type validation_error, got %s", apiErr.Type)
	}
}

func TestConnectorsList(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integrations/connectors" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("workspace_name") != "test-ws" {
			t.Errorf("expected workspace_name=test-ws, got %s", r.URL.Query().Get("workspace_name"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "c1", "name": "Connector 1"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	result, err := connectorsList(context.Background(), c, map[string]any{"workspace": "test-ws"})
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
		t.Errorf("expected 1 connector, got %v", parsed["data"])
	}
}

func TestConnectorsListDefaultsWorkspace(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("workspace_name"); got != "default" {
			t.Errorf("expected workspace_name=default, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": []}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	var stderr bytes.Buffer
	prev := statusWriter
	statusWriter = &stderr
	defer func() { statusWriter = prev }()

	if _, err := connectorsList(context.Background(), c, map[string]any{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var notice map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &notice); err != nil {
		t.Fatalf("expected JSON notice on stderr, got %q (err: %v)", stderr.String(), err)
	}
	if notice["workspace"] != "default" {
		t.Errorf("expected workspace=default in notice, got %q", notice["workspace"])
	}
	if !strings.Contains(notice["message"], "falling back") {
		t.Errorf("expected message to mention fallback, got %q", notice["message"])
	}
}

func TestConnectorsListAvailable(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integrations/templates/sources/global" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "tmpl-1", "name": "Salesforce"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	result, err := connectorsListAvailable(context.Background(), c, map[string]any{})
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
		t.Errorf("expected 1 template, got %v", parsed["data"])
	}
}

func TestConnectorsDescribe(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/integrations/connectors/conn-1":
			_, _ = w.Write([]byte(`{"id": "conn-1", "name": "Test Connector"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/integrations/connectors/conn-1/execute":
			_, _ = w.Write([]byte(`{"entities": [{"name": "contacts"}]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	result, err := connectorsDescribe(context.Background(), c, map[string]any{"id": "conn-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	if m["id"] != "conn-1" {
		t.Errorf("expected id=conn-1, got %v", m["id"])
	}
	if m["name"] != "Test Connector" {
		t.Errorf("expected name=Test Connector, got %v", m["name"])
	}
	if m["schema"] == nil {
		t.Error("expected schema to be populated from describe")
	}
}

func TestConnectorsExecute(t *testing.T) {
	var gotBody map[string]any
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integrations/connectors/conn-1/execute" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"name": "John"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	result, err := connectorsExecute(context.Background(), c, map[string]any{
		"id":             "conn-1",
		"entity":         "contacts",
		"action":         "list",
		"select_fields":  []string{"name", "email"},
		"exclude_fields": []string{"phone"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotBody["entity"] != "contacts" {
		t.Errorf("expected entity=contacts, got %v", gotBody["entity"])
	}
	if gotBody["action"] != "list" {
		t.Errorf("expected action=list, got %v", gotBody["action"])
	}
	if gotBody["select_fields"] == nil {
		t.Error("expected select_fields in body")
	}
	if gotBody["exclude_fields"] == nil {
		t.Error("expected exclude_fields in body")
	}

	raw, ok := result.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", result)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}
}

func TestConnectorsDelete(t *testing.T) {
	var gotMethod, gotPath string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	_, err := connectorsDelete(context.Background(), c, map[string]any{"id": "conn-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if gotPath != "/api/v1/integrations/connectors/conn-1" {
		t.Errorf("expected path /api/v1/integrations/connectors/conn-1, got %s", gotPath)
	}
}

func TestConnectorPathEscapesID(t *testing.T) {
	got := connectorPath("conn/1?x=y")
	want := "/api/v1/integrations/connectors/conn%2F1%3Fx=y"
	if got != want {
		t.Errorf("connectorPath = %q, want %q", got, want)
	}
}

func TestConnectorsDescribeReturnsSchemaError(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/integrations/connectors/conn-1":
			_, _ = w.Write([]byte(`{"id": "conn-1", "name": "Test Connector"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/integrations/connectors/conn-1/execute":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"detail": "describe failed"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	_, err := connectorsDescribe(context.Background(), c, map[string]any{"id": "conn-1"})
	if err == nil {
		t.Fatal("expected describe schema error")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", apiErr.StatusCode)
	}
}

func TestConnectorsResourceOperations(t *testing.T) {
	res := &connectorsResource{}
	ops := res.Operations()

	expected := map[string]bool{
		"create":         false,
		"list":           false,
		"list-available": false,
		"describe":       false,
		"execute":        false,
		"delete":         false,
	}

	for _, op := range ops {
		if _, ok := expected[op.Name]; ok {
			expected[op.Name] = true
		} else {
			t.Errorf("unexpected operation: %s", op.Name)
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("missing expected operation: %s", name)
		}
	}
}

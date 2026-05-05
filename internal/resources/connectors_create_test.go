package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/airbytehq/airbyte-cli/internal/client"
)

func TestResolveTemplateID_ByID(t *testing.T) {
	id, err := resolveTemplateID(context.Background(), nil, map[string]any{"id": "tmpl-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "tmpl-123" {
		t.Errorf("expected tmpl-123, got %s", id)
	}
}

func TestResolveTemplateID_MissingBoth(t *testing.T) {
	_, err := resolveTemplateID(context.Background(), nil, map[string]any{})
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

func TestResolveTemplateID_ByName(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "tmpl-sf", "name": "Salesforce"}, {"id": "tmpl-hb", "name": "HubSpot"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	id, err := resolveTemplateID(context.Background(), c, map[string]any{"name": "salesforce"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "tmpl-sf" {
		t.Errorf("expected tmpl-sf, got %s", id)
	}
}

func TestResolveTemplateID_NameNotFound(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "tmpl-1", "name": "Salesforce"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	_, err := resolveTemplateID(context.Background(), c, map[string]any{"name": "missing"})
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

func TestResolveWorkspaceID(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"workspace_id": "ws-123", "name": "My Workspace"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	id, err := resolveWorkspaceID(context.Background(), c, "My Workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "ws-123" {
		t.Errorf("expected ws-123, got %s", id)
	}
}

func TestResolveWorkspaceID_NotFound(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": []}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	_, err := resolveWorkspaceID(context.Background(), c, "Missing")
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

func TestConnectorsCreateInteractive_Success(t *testing.T) {
	old := openBrowserFunc
	openBrowserFunc = func(url string) {}
	defer func() { openBrowserFunc = old }()

	t.Setenv("AIRBYTE_CREDENTIAL_TIMEOUT", "10")

	var pollCount atomic.Int32
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/integrations/templates/sources/global":
			_, _ = w.Write([]byte(`{"data": [{"id": "tmpl-1", "name": "Salesforce"}]}`))

		case r.URL.Path == "/api/v1/integrations/templates/sources/tmpl-1" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id": "tmpl-1", "source_definition_id": "sdef-1", "name": "Salesforce"}`))

		case r.URL.Path == "/api/v1/workspaces" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"data": [{"workspace_id": "ws-1", "name": "test-ws"}]}`))

		case r.URL.Path == "/api/v1/account/applications/widget-token" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"token": "widget-tok"}`))

		case r.URL.Path == "/api/v1/internal/mcp_oauth/sessions" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"session_id": "sess-1"}`))

		case r.URL.Path == "/api/v1/internal/mcp_oauth/sessions/sess-1" && r.Method == http.MethodGet:
			count := pollCount.Add(1)
			if count < 2 {
				_, _ = w.Write([]byte(`{"status": "pending"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status": "completed", "credentials": {"api_key": "secret"}}`))

		case r.URL.Path == "/api/v1/integrations/connectors" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id": "conn-new", "name": "Salesforce", "status": "active"}`))

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	t.Setenv("AIRBYTE_WEBAPP_URL", apiServer.URL)

	result, err := connectorsCreateInteractive(context.Background(), c, map[string]any{
		"name":      "Salesforce",
		"workspace": "test-ws",
	})
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

	if parsed["id"] != "conn-new" {
		t.Errorf("expected id=conn-new, got %v", parsed["id"])
	}
}

func TestConnectorsCreateInteractive_Timeout(t *testing.T) {
	old := openBrowserFunc
	openBrowserFunc = func(url string) {}
	defer func() { openBrowserFunc = old }()

	t.Setenv("AIRBYTE_CREDENTIAL_TIMEOUT", "3")

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/integrations/templates/sources/tmpl-1" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id": "tmpl-1", "source_definition_id": "sdef-1"}`))

		case r.URL.Path == "/api/v1/workspaces":
			_, _ = w.Write([]byte(`{"data": [{"workspace_id": "ws-1", "name": "test-ws"}]}`))

		case r.URL.Path == "/api/v1/account/applications/widget-token":
			_, _ = w.Write([]byte(`{"token": "tok"}`))

		case r.URL.Path == "/api/v1/internal/mcp_oauth/sessions" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"session_id": "sess-1"}`))

		case r.URL.Path == "/api/v1/internal/mcp_oauth/sessions/sess-1" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"status": "pending"}`))

		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	t.Setenv("AIRBYTE_WEBAPP_URL", apiServer.URL)

	start := time.Now()
	result, err := connectorsCreateInteractive(context.Background(), c, map[string]any{
		"id":        "tmpl-1",
		"workspace": "test-ws",
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string, got %T", result)
	}

	if resultMap["error"] != "timeout" {
		t.Errorf("expected error=timeout, got %v", resultMap["error"])
	}

	if elapsed > 15*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestConnectorsCreateInteractive_CredentialFlowFailed(t *testing.T) {
	old := openBrowserFunc
	openBrowserFunc = func(url string) {}
	defer func() { openBrowserFunc = old }()

	t.Setenv("AIRBYTE_CREDENTIAL_TIMEOUT", "10")

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/integrations/templates/sources/tmpl-1" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id": "tmpl-1", "source_definition_id": "sdef-1"}`))

		case r.URL.Path == "/api/v1/workspaces":
			_, _ = w.Write([]byte(`{"data": [{"workspace_id": "ws-1", "name": "test-ws"}]}`))

		case r.URL.Path == "/api/v1/account/applications/widget-token":
			_, _ = w.Write([]byte(`{"token": "tok"}`))

		case r.URL.Path == "/api/v1/internal/mcp_oauth/sessions" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"session_id": "sess-1"}`))

		case r.URL.Path == "/api/v1/internal/mcp_oauth/sessions/sess-1" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"status": "failed"}`))

		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	t.Setenv("AIRBYTE_WEBAPP_URL", apiServer.URL)

	_, err := connectorsCreateInteractive(context.Background(), c, map[string]any{
		"id":        "tmpl-1",
		"workspace": "test-ws",
	})
	if err == nil {
		t.Fatal("expected error for failed credential flow")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
}

func TestCredentialTimeout(t *testing.T) {
	t.Setenv("AIRBYTE_CREDENTIAL_TIMEOUT", "120")
	if got := credentialTimeout(); got != 120*time.Second {
		t.Errorf("expected 120s, got %v", got)
	}

	t.Setenv("AIRBYTE_CREDENTIAL_TIMEOUT", "")
	if got := credentialTimeout(); got != defaultCredentialTimeout {
		t.Errorf("expected default %v, got %v", defaultCredentialTimeout, got)
	}

	t.Setenv("AIRBYTE_CREDENTIAL_TIMEOUT", "invalid")
	if got := credentialTimeout(); got != defaultCredentialTimeout {
		t.Errorf("expected default %v for invalid input, got %v", defaultCredentialTimeout, got)
	}
}

func TestCredentialURL(t *testing.T) {
	got, err := credentialURL("https://cloud.airbyte.com/base", "session/1", "tok&en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "https://cloud.airbyte.com/base/embedded-widget/credentials?session_id=session%2F1&token=tok%26en"
	if got != want {
		t.Errorf("credentialURL = %q, want %q", got, want)
	}
}

func TestWebAppBaseURL(t *testing.T) {
	t.Setenv("AIRBYTE_WEBAPP_URL", "https://custom.airbyte.com")
	if got := webAppBaseURL(); got != "https://custom.airbyte.com" {
		t.Errorf("expected custom URL, got %s", got)
	}

	t.Setenv("AIRBYTE_WEBAPP_URL", "")
	if got := webAppBaseURL(); got != defaultWebAppBaseURL {
		t.Errorf("expected default URL, got %s", got)
	}
}

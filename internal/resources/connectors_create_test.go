package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/airbytehq/airbyte-agents-cli/internal/client"
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

// fastPolling overrides the credential-flow polling cadence so tests don't
// have to wait the production 30-second initial delay. Returns a cleanup
// function the caller should defer.
func fastPolling(t *testing.T) func() {
	t.Helper()
	oldInitial := initialCredentialPollDelay
	oldInterval := credentialPollInterval
	initialCredentialPollDelay = 10 * time.Millisecond
	credentialPollInterval = 10 * time.Millisecond
	return func() {
		initialCredentialPollDelay = oldInitial
		credentialPollInterval = oldInterval
	}
}

// makeWidgetTokenB64 builds a base64-encoded widget token whose decoded
// widgetUrl carries workspaceId=<id>, matching what the real widget-token
// endpoint returns.
func makeWidgetTokenB64(t *testing.T, workspaceID, innerToken string) string {
	t.Helper()
	payload := map[string]any{
		"widgetUrl": "https://example.com/widget?workspaceId=" + workspaceID,
		"token":     innerToken,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshaling widget token payload: %v", err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func TestDecodeWidgetToken(t *testing.T) {
	b64 := makeWidgetTokenB64(t, "ws-abc", "inner-tok")
	got, err := decodeWidgetToken(b64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Token != "inner-tok" {
		t.Errorf("expected token=inner-tok, got %q", got.Token)
	}
	if !strings.Contains(got.WidgetURL, "workspaceId=ws-abc") {
		t.Errorf("expected widgetUrl to contain workspaceId=ws-abc, got %q", got.WidgetURL)
	}
}

func TestDecodeWidgetToken_BadBase64(t *testing.T) {
	if _, err := decodeWidgetToken("not!base64!"); err == nil {
		t.Fatal("expected error on invalid base64")
	}
}

func TestExtractWorkspaceID(t *testing.T) {
	id, err := extractWorkspaceID("https://example.com/widget?workspaceId=ws-xyz&other=1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "ws-xyz" {
		t.Errorf("expected ws-xyz, got %q", id)
	}
}

func TestExtractWorkspaceID_Missing(t *testing.T) {
	if _, err := extractWorkspaceID("https://example.com/widget?other=1"); err == nil {
		t.Fatal("expected error when workspaceId missing")
	}
}

func TestExtractWorkspaceID_Empty(t *testing.T) {
	if _, err := extractWorkspaceID(""); err == nil {
		t.Fatal("expected error on empty URL")
	}
}

func TestConnectorsCreateInteractive_Success(t *testing.T) {
	old := openBrowserFunc
	openBrowserFunc = func(url string) {}
	defer func() { openBrowserFunc = old }()

	defer fastPolling(t)()

	t.Setenv("AIRBYTE_CREDENTIAL_TIMEOUT", "10")

	widgetTokenB64 := makeWidgetTokenB64(t, "ws-1", "inner-tok")

	var pollCount atomic.Int32
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/integrations/templates/sources/global":
			_, _ = w.Write([]byte(`{"data": [{"id": "tmpl-1", "name": "Salesforce"}]}`))

		case r.URL.Path == "/api/v1/integrations/templates/sources/tmpl-1" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id": "tmpl-1", "actor_definition_id": "sdef-1", "name": "Salesforce"}`))

		case r.URL.Path == "/api/v1/account/applications/widget-token" && r.Method == http.MethodPost:
			_, _ = fmt.Fprintf(w, `{"token": %q}`, widgetTokenB64)

		case r.URL.Path == "/api/v1/internal/mcp_oauth/sessions" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"session_id": "sess-1"}`))

		case r.URL.Path == "/api/v1/internal/mcp_oauth/sessions/sess-1" && r.Method == http.MethodGet:
			count := pollCount.Add(1)
			if count < 2 {
				_, _ = w.Write([]byte(`{"status": "pending"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status": "completed", "auth_payload": {"api_key": "secret"}, "source_template_id": "tmpl-1"}`))

		case r.URL.Path == "/api/v1/internal/mcp_oauth/sessions/sess-1" && r.Method == http.MethodPatch:
			_, _ = w.Write([]byte(`{"session_id": "sess-1", "source_id": "conn-new"}`))

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

	defer fastPolling(t)()

	t.Setenv("AIRBYTE_CREDENTIAL_TIMEOUT", "3")

	widgetTokenB64 := makeWidgetTokenB64(t, "ws-1", "inner-tok")

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/integrations/templates/sources/tmpl-1" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id": "tmpl-1", "actor_definition_id": "sdef-1"}`))

		case r.URL.Path == "/api/v1/account/applications/widget-token":
			_, _ = fmt.Fprintf(w, `{"token": %q}`, widgetTokenB64)

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

	defer fastPolling(t)()

	t.Setenv("AIRBYTE_CREDENTIAL_TIMEOUT", "10")

	widgetTokenB64 := makeWidgetTokenB64(t, "ws-1", "inner-tok")

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/integrations/templates/sources/tmpl-1" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id": "tmpl-1", "actor_definition_id": "sdef-1"}`))

		case r.URL.Path == "/api/v1/account/applications/widget-token":
			_, _ = fmt.Fprintf(w, `{"token": %q}`, widgetTokenB64)

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
	got, err := credentialURL("https://cloud.airbyte.com", credentialURLParams{
		WidgetTokenB64:     "abc&def",
		SessionID:          "sess/1",
		TemplateID:         "tmpl-1",
		UseGlobalTemplates: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parsing result: %v", err)
	}
	if u.Path != "/widget-bridge" {
		t.Errorf("expected path /widget-bridge, got %q", u.Path)
	}
	q := u.Query()
	if q.Get("widget_token") != "abc&def" {
		t.Errorf("widget_token wrong: %q", q.Get("widget_token"))
	}
	if q.Get("session") != "sess/1" {
		t.Errorf("session wrong: %q", q.Get("session"))
	}
	if q.Get("selectedTemplateId") != "tmpl-1" {
		t.Errorf("selectedTemplateId wrong: %q", q.Get("selectedTemplateId"))
	}
	if q.Get("showEntityPicker") != "true" {
		t.Errorf("showEntityPicker wrong: %q", q.Get("showEntityPicker"))
	}
	if q.Get("useGlobalTemplates") != "true" {
		t.Errorf("useGlobalTemplates wrong: %q", q.Get("useGlobalTemplates"))
	}
}

func TestCredentialURL_OmitsGlobalWhenFalse(t *testing.T) {
	got, err := credentialURL("https://cloud.airbyte.com", credentialURLParams{
		WidgetTokenB64: "tok", SessionID: "s", TemplateID: "t",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	u, _ := url.Parse(got)
	if u.Query().Has("useGlobalTemplates") {
		t.Errorf("useGlobalTemplates should be absent when UseGlobalTemplates=false; got %q", u.Query().Get("useGlobalTemplates"))
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

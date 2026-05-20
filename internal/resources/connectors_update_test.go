package resources

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/airbytehq/airbyte-agent-cli/internal/auth"
	"github.com/airbytehq/airbyte-agent-cli/internal/client"
)

// stubConfirmOpenBrowser swaps the package confirmation hook for the
// duration of the test, recording the message/url it was called with and
// returning the supplied answer. Restores the original on cleanup.
func stubConfirmOpenBrowser(t *testing.T, answer bool) *struct {
	called    bool
	gotURL    string
	gotMsg    string
	callCount int
} {
	t.Helper()
	state := &struct {
		called    bool
		gotURL    string
		gotMsg    string
		callCount int
	}{}
	prev := confirmOpenBrowser
	confirmOpenBrowser = func(message, url string) bool {
		state.called = true
		state.callCount++
		state.gotURL = url
		state.gotMsg = message
		return answer
	}
	t.Cleanup(func() { confirmOpenBrowser = prev })
	return state
}

func TestCredentialsPageURL(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		orgID   string
		want    string
		wantErr bool
	}{
		{
			name:    "default base URL",
			baseURL: "https://app.airbyte.ai",
			orgID:   "org-123",
			want:    "https://app.airbyte.ai/organizations/org-123/credentials",
		},
		{
			name:    "base URL with trailing slash collapses",
			baseURL: "https://app.airbyte.ai/",
			orgID:   "org-123",
			want:    "https://app.airbyte.ai/organizations/org-123/credentials",
		},
		{
			name:    "env-override style staging URL",
			baseURL: "https://staging.airbyte.ai",
			orgID:   "org-123",
			want:    "https://staging.airbyte.ai/organizations/org-123/credentials",
		},
		{
			name:    "org id with characters needing escape",
			baseURL: "https://app.airbyte.ai",
			orgID:   "org/special",
			want:    "https://app.airbyte.ai/organizations/org%2Fspecial/credentials",
		},
		{
			name:    "invalid base URL returns error",
			baseURL: "://broken",
			orgID:   "org-123",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := credentialsPageURL(tc.baseURL, tc.orgID)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConnectorsUpdate_HappyPath_NameAndWorkspace(t *testing.T) {
	t.Setenv("AIRBYTE_WEBAPP_URL", "https://app.airbyte.ai")

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected API call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	prevOpen := openBrowserFunc
	var capturedURL string
	openBrowserFunc = func(u string) { capturedURL = u }
	defer func() { openBrowserFunc = prevOpen }()

	prevStatus := statusWriter
	var stderr bytes.Buffer
	statusWriter = &stderr
	defer func() { statusWriter = prevStatus }()

	confirmState := stubConfirmOpenBrowser(t, true)

	params := map[string]any{
		"id":        "conn-1",
		"name":      "acme",
		"workspace": "production",
	}
	result, err := connectorsUpdate(context.Background(), c, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantURL := "https://app.airbyte.ai/organizations/org-123/credentials"
	if capturedURL != wantURL {
		t.Errorf("openBrowser URL = %q, want %q", capturedURL, wantURL)
	}
	if !confirmState.called {
		t.Error("confirmOpenBrowser was not called")
	}
	if confirmState.gotURL != wantURL {
		t.Errorf("confirmOpenBrowser url = %q, want %q", confirmState.gotURL, wantURL)
	}

	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any result, got %T", result)
	}
	if resMap["url"] != wantURL {
		t.Errorf("result url = %q, want %q", resMap["url"], wantURL)
	}
	if resMap["connector_id"] != "conn-1" {
		t.Errorf("result connector_id = %q, want %q", resMap["connector_id"], "conn-1")
	}
	if resMap["browser_opened"] != true {
		t.Errorf("result browser_opened = %v, want true", resMap["browser_opened"])
	}
	msg, _ := resMap["message"].(string)
	if !strings.Contains(msg, "acme") {
		t.Errorf("message missing connector name: %q", msg)
	}
	if !strings.Contains(msg, "production") {
		t.Errorf("message missing workspace name: %q", msg)
	}
	if !strings.Contains(msg, "conn-1") {
		t.Errorf("message missing connector id: %q", msg)
	}
	// The lead disclaimer is the user-visible signal that the CLI does not
	// edit the connector itself — keep it in front of the actionable phrasing.
	if !strings.HasPrefix(msg, "Connectors cannot be edited through the CLI") {
		t.Errorf("message must lead with CLI-cannot-edit disclaimer, got %q", msg)
	}

	// No stderr envelope: the same data is returned as the result and printed
	// to stdout once by the standard writer. Writing it to stderr too would
	// duplicate the output (an artifact of the connectors-create polling
	// pattern that doesn't apply here). The confirm prompt's own output goes
	// through confirmWriter, not statusWriter.
	if got := bytes.TrimSpace(stderr.Bytes()); len(got) != 0 {
		t.Errorf("expected empty stderr (statusWriter), got %q", got)
	}
}

func TestConnectorsUpdate_DeclinedConfirmation_SkipsBrowser(t *testing.T) {
	t.Setenv("AIRBYTE_WEBAPP_URL", "https://app.airbyte.ai")

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected API call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	prevOpen := openBrowserFunc
	browserCalled := false
	openBrowserFunc = func(string) { browserCalled = true }
	defer func() { openBrowserFunc = prevOpen }()

	stubConfirmOpenBrowser(t, false)

	params := map[string]any{"id": "conn-1"}
	result, err := connectorsUpdate(context.Background(), c, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if browserCalled {
		t.Error("openBrowser was invoked despite declined confirmation")
	}

	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any result, got %T", result)
	}
	if resMap["browser_opened"] != false {
		t.Errorf("result browser_opened = %v, want false", resMap["browser_opened"])
	}
	// URL is still surfaced so the user/agent can act on it.
	if resMap["url"] != "https://app.airbyte.ai/organizations/org-123/credentials" {
		t.Errorf("URL must still be returned even when browser is skipped, got %v", resMap["url"])
	}
}

func TestConnectorsUpdate_HappyPath_IDOnly(t *testing.T) {
	t.Setenv("AIRBYTE_WEBAPP_URL", "https://app.airbyte.ai")

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected API call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	prevOpen := openBrowserFunc
	var capturedURL string
	openBrowserFunc = func(u string) { capturedURL = u }
	defer func() { openBrowserFunc = prevOpen }()

	prevStatus := statusWriter
	var stderr bytes.Buffer
	statusWriter = &stderr
	defer func() { statusWriter = prevStatus }()

	stubConfirmOpenBrowser(t, true)

	params := map[string]any{"id": "conn-1"}
	result, err := connectorsUpdate(context.Background(), c, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantURL := "https://app.airbyte.ai/organizations/org-123/credentials"
	if capturedURL != wantURL {
		t.Errorf("openBrowser URL = %q, want %q", capturedURL, wantURL)
	}

	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any result, got %T", result)
	}
	msg, _ := resMap["message"].(string)
	// Id-only fallback phrasing — no quoted name token, no workspace label.
	if !strings.Contains(msg, "the connector with id conn-1") {
		t.Errorf("message should use id-only fallback phrasing, got %q", msg)
	}
	if strings.Contains(msg, "\"") {
		t.Errorf("id-only message should not contain quoted name token, got %q", msg)
	}
	if resMap["browser_opened"] != true {
		t.Errorf("result browser_opened = %v, want true", resMap["browser_opened"])
	}

	if got := bytes.TrimSpace(stderr.Bytes()); len(got) != 0 {
		t.Errorf("expected empty stderr (statusWriter), got %q", got)
	}
}

func TestConnectorsUpdate_MissingOrgID(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected API call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer apiServer.Close()

	tokenServer := newTestTokenServer(t)
	defer tokenServer.Close()
	creds := &auth.Credentials{ClientID: "id", ClientSecret: "secret"}
	tm := auth.NewTokenManager(tokenServer.URL, "", creds)
	c := client.New(apiServer.URL, "", "test", tm)

	prevOpen := openBrowserFunc
	called := false
	openBrowserFunc = func(string) { called = true }
	defer func() { openBrowserFunc = prevOpen }()

	prevStatus := statusWriter
	var stderr bytes.Buffer
	statusWriter = &stderr
	defer func() { statusWriter = prevStatus }()

	confirmState := stubConfirmOpenBrowser(t, true)

	_, err := connectorsUpdate(context.Background(), c, map[string]any{"id": "conn-1"})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.Type != "validation_error" {
		t.Errorf("expected type validation_error, got %q", apiErr.Type)
	}
	if called {
		t.Error("openBrowserFunc was invoked despite missing org id")
	}
	if confirmState.called {
		t.Error("confirmOpenBrowser was invoked despite missing org id — validation must short-circuit first")
	}
}

func TestConnectorsUpdate_InvalidWebAppURL(t *testing.T) {
	t.Setenv("AIRBYTE_WEBAPP_URL", "://broken")

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected API call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	prevOpen := openBrowserFunc
	called := false
	openBrowserFunc = func(string) { called = true }
	defer func() { openBrowserFunc = prevOpen }()

	prevStatus := statusWriter
	var stderr bytes.Buffer
	statusWriter = &stderr
	defer func() { statusWriter = prevStatus }()

	confirmState := stubConfirmOpenBrowser(t, true)

	_, err := connectorsUpdate(context.Background(), c, map[string]any{"id": "conn-1"})
	if err == nil {
		t.Fatal("expected error from invalid web app URL, got nil")
	}
	if called {
		t.Error("openBrowserFunc was invoked despite invalid web app URL")
	}
	if confirmState.called {
		t.Error("confirmOpenBrowser was invoked despite URL parse failure — URL build must short-circuit first")
	}
}

func TestConnectorsUpdate_ResolveConnectorIDIntegration(t *testing.T) {
	t.Setenv("AIRBYTE_WEBAPP_URL", "https://app.airbyte.ai")

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integrations/connectors" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.URL.Query().Get("workspace_name"); got != "production" {
			t.Errorf("expected workspace_name=production, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"id": "conn-1", "name": "acme"}]}`))
	}))
	defer apiServer.Close()

	c, cleanup := newTestClient(t, apiServer)
	defer cleanup()

	prevOpen := openBrowserFunc
	var capturedURL string
	openBrowserFunc = func(u string) { capturedURL = u }
	defer func() { openBrowserFunc = prevOpen }()

	prevStatus := statusWriter
	var stderr bytes.Buffer
	statusWriter = &stderr
	defer func() { statusWriter = prevStatus }()

	stubConfirmOpenBrowser(t, true)

	params := map[string]any{"name": "acme", "workspace": "production"}
	resolved, err := resolveConnectorID(context.Background(), c, params)
	if err != nil {
		t.Fatalf("resolveConnectorID error: %v", err)
	}
	if resolved["id"] != "conn-1" {
		t.Fatalf("expected resolved id=conn-1, got %v", resolved["id"])
	}

	result, err := connectorsUpdate(context.Background(), c, resolved)
	if err != nil {
		t.Fatalf("connectorsUpdate error: %v", err)
	}

	wantURL := "https://app.airbyte.ai/organizations/org-123/credentials"
	if capturedURL != wantURL {
		t.Errorf("openBrowser URL = %q, want %q", capturedURL, wantURL)
	}

	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any result, got %T", result)
	}
	if resMap["connector_id"] != "conn-1" {
		t.Errorf("result connector_id = %q, want %q", resMap["connector_id"], "conn-1")
	}
	msg, _ := resMap["message"].(string)
	if !strings.Contains(msg, "acme") {
		t.Errorf("message missing connector name: %q", msg)
	}
	if !strings.Contains(msg, "production") {
		t.Errorf("message missing workspace name: %q", msg)
	}
	if resMap["browser_opened"] != true {
		t.Errorf("result browser_opened = %v, want true", resMap["browser_opened"])
	}

	// resolveConnectorID was called with workspace=production, so
	// applyDefaultWorkspace doesn't print a fallback notice. statusWriter
	// must be clean — connectorsUpdate emits no envelope and the confirm
	// prompt writes elsewhere (confirmWriter).
	if got := bytes.TrimSpace(stderr.Bytes()); len(got) != 0 {
		t.Errorf("expected empty stderr (statusWriter), got %q", got)
	}
}

// blockingReader satisfies io.Reader by blocking on Read forever (until the
// process exits). Used to exercise the confirmOpenBrowser timeout path.
type blockingReader struct{}

func (blockingReader) Read(p []byte) (int, error) {
	select {} // block forever; the test goroutine cleans up on process exit
}

func TestConfirmOpenBrowser_YesOpens(t *testing.T) {
	withConfirmIO(t, "yes\n")
	if !confirmOpenBrowser("msg", "https://example/credentials") {
		t.Error("expected true on 'yes' input")
	}
}

func TestConfirmOpenBrowser_YesIsCaseAndWhitespaceInsensitive(t *testing.T) {
	for _, input := range []string{"YES\n", "  yes  \n", "Yes\n"} {
		t.Run(input, func(t *testing.T) {
			withConfirmIO(t, input)
			if !confirmOpenBrowser("msg", "https://example/credentials") {
				t.Errorf("expected true on %q", input)
			}
		})
	}
}

func TestConfirmOpenBrowser_NonYesSkips(t *testing.T) {
	for _, input := range []string{"no\n", "y\n", "yeah\n", "\n", "1\n"} {
		t.Run(input, func(t *testing.T) {
			withConfirmIO(t, input)
			if confirmOpenBrowser("msg", "https://example/credentials") {
				t.Errorf("expected false on %q", input)
			}
		})
	}
}

func TestConfirmOpenBrowser_EOFSkipsQuietly(t *testing.T) {
	prevReader := confirmReader
	prevWriter := confirmWriter
	confirmReader = strings.NewReader("") // immediate EOF
	var w bytes.Buffer
	confirmWriter = &w
	defer func() {
		confirmReader = prevReader
		confirmWriter = prevWriter
	}()

	if confirmOpenBrowser("msg", "https://example/credentials") {
		t.Error("expected false on EOF")
	}
	if strings.Contains(w.String(), "not opening browser") {
		t.Errorf("EOF path should skip the chatty notice; got: %q", w.String())
	}
}

func TestConfirmOpenBrowser_TimeoutSkips(t *testing.T) {
	prevReader := confirmReader
	prevWriter := confirmWriter
	prevTimeout := confirmOpenBrowserTimeout
	confirmReader = blockingReader{}
	var w bytes.Buffer
	confirmWriter = &w
	confirmOpenBrowserTimeout = 50 * time.Millisecond
	defer func() {
		confirmReader = prevReader
		confirmWriter = prevWriter
		confirmOpenBrowserTimeout = prevTimeout
	}()

	start := time.Now()
	if confirmOpenBrowser("msg", "https://example/credentials") {
		t.Error("expected false on timeout")
	}
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("timeout should fire within ~50ms, took %s", elapsed)
	}
	if !strings.Contains(w.String(), "timed out") {
		t.Errorf("timeout path should print a timeout notice; got: %q", w.String())
	}
}

// withConfirmIO swaps confirmReader / confirmWriter for the test and
// restores them after. The reader is seeded with `input` (typically
// "yes\n"); the writer captures whatever the prompt emits.
func withConfirmIO(t *testing.T, input string) {
	t.Helper()
	prevReader := confirmReader
	prevWriter := confirmWriter
	confirmReader = strings.NewReader(input)
	confirmWriter = &bytes.Buffer{}
	t.Cleanup(func() {
		confirmReader = prevReader
		confirmWriter = prevWriter
	})
}

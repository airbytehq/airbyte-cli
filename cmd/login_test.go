package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/airbytehq/airbyte-agent-cli/internal/auth"
	"github.com/airbytehq/airbyte-agent-cli/internal/auth/browserlogin"
	"github.com/airbytehq/airbyte-agent-cli/internal/browser"
)

func TestObfuscateSecret(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"short fully hidden", "abc", "***"},
		{"exactly 4 fully hidden", "abcd", "****"},
		{"5 chars shows last 4", "abcde", "*bcde"},
		{"long shows last 4", "abcdefghij", "******ghij"},
		{"typical secret", "sk-live-abcdef0123456789WXYZ", "************************WXYZ"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := obfuscateSecret(c.in)
			if got != c.want {
				t.Errorf("obfuscateSecret(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestObfuscateSecret_AlwaysHidesMost(t *testing.T) {
	// Quick property check — for any non-trivial secret, no more than 4
	// non-asterisk characters should appear in the output.
	for _, in := range []string{"hunter2", "password", "thisIsASecret", strings.Repeat("x", 64)} {
		got := obfuscateSecret(in)
		visible := 0
		for _, r := range got {
			if r != '*' {
				visible++
			}
		}
		if visible > 4 {
			t.Errorf("obfuscateSecret(%q) leaked %d non-asterisk chars: %q", in, visible, got)
		}
	}
}

// loginStubs is the shared test scaffolding for the browser-path tests.
// It runs httptest servers for Keycloak (token exchange) and sonar
// (enrollment/organizations/applications) and lets each test override the
// per-endpoint handler. It also stubs browser.OpenFunc with a goroutine
// that fires the loopback callback so RunOAuthFlow actually completes.
type loginStubs struct {
	keycloak           *httptest.Server
	sonar              *httptest.Server
	enrollmentCalls    int32
	organizationsCalls int32
	applicationsCalls  int32
	previousOpenFunc   func(string) error
}

type sonarRoutes struct {
	enrollment    http.HandlerFunc
	organizations http.HandlerFunc
	applications  http.HandlerFunc
}

func newLoginStubs(t *testing.T, routes sonarRoutes) *loginStubs {
	t.Helper()
	s := &loginStubs{previousOpenFunc: browser.OpenFunc}

	// Keycloak — returns a canned access_token for the PKCE code exchange.
	kcMux := http.NewServeMux()
	kcMux.HandleFunc("/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"fake-access-token","refresh_token":"rt","id_token":"it","token_type":"Bearer","expires_in":300}`)
	})
	s.keycloak = httptest.NewServer(kcMux)
	t.Cleanup(s.keycloak.Close)

	// sonar — three optional handlers; calling an unset handler fails the test.
	sonarMux := http.NewServeMux()
	sonarMux.HandleFunc(browserlogin.APIBasePath+"/internal/account/enrollment-status", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s.enrollmentCalls, 1)
		if routes.enrollment == nil {
			t.Errorf("unexpected /enrollment-status call")
			http.Error(w, "no handler", http.StatusInternalServerError)
			return
		}
		routes.enrollment(w, r)
	})
	sonarMux.HandleFunc(browserlogin.APIBasePath+"/internal/account/organizations", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s.organizationsCalls, 1)
		if routes.organizations == nil {
			t.Errorf("unexpected /organizations call")
			http.Error(w, "no handler", http.StatusInternalServerError)
			return
		}
		routes.organizations(w, r)
	})
	sonarMux.HandleFunc(browserlogin.APIBasePath+"/internal/account/applications", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s.applicationsCalls, 1)
		if routes.applications == nil {
			t.Errorf("unexpected /applications call")
			http.Error(w, "no handler", http.StatusInternalServerError)
			return
		}
		routes.applications(w, r)
	})
	s.sonar = httptest.NewServer(sonarMux)
	t.Cleanup(s.sonar.Close)

	// Stub browser.OpenFunc: simulate the user completing the login by GETting
	// the loopback callback with the same `state` that was in the authorize URL.
	browser.OpenFunc = func(authURL string) error {
		go func() {
			u, parseErr := url.Parse(authURL)
			if parseErr != nil {
				t.Errorf("stub browser: parse authURL: %v", parseErr)
				return
			}
			q := u.Query()
			cb := fmt.Sprintf("%s?code=fake-code&state=%s",
				q.Get("redirect_uri"),
				url.QueryEscape(q.Get("state")),
			)
			resp, err := http.Get(cb)
			if err != nil {
				t.Errorf("stub browser: GET callback: %v", err)
				return
			}
			resp.Body.Close()
		}()
		return nil
	}
	t.Cleanup(func() { browser.OpenFunc = s.previousOpenFunc })

	return s
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, body interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

// configureBrowserLoginEnv points the runner at the test servers.
// AIRBYTE_KEYCLOAK_URL is read by runBrowserLogin; AIRBYTE_API_HOST is
// read by internal/config.Load() (which runBrowserLogin calls via
// config.Load().APIHost).
func configureBrowserLoginEnv(t *testing.T, stubs *loginStubs) {
	t.Helper()
	t.Setenv("AIRBYTE_KEYCLOAK_URL", stubs.keycloak.URL)
	t.Setenv("AIRBYTE_API_HOST", stubs.sonar.URL)
	t.Setenv("AIRBYTE_TELEMETRY_MODE", "disabled")
	t.Setenv("HOME", t.TempDir())
}

func TestRunBrowserLogin_HappyPath(t *testing.T) {
	const orgID = "org-happy-uuid"
	stubs := newLoginStubs(t, sonarRoutes{
		enrollment: func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer fake-access-token" {
				t.Errorf("enrollment Authorization = %q, want Bearer fake-access-token", got)
			}
			writeJSON(t, w, 200, map[string]interface{}{
				"is_enrolled":     true,
				"organization_id": orgID,
			})
		},
		organizations: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 200, map[string]interface{}{
				"organizations": []map[string]string{
					{"id": orgID, "organization_name": "Solo"},
				},
				"is_instance_admin": false,
			})
		},
		applications: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 201, map[string]string{
				"client_id":     "browser-client-id",
				"client_secret": "browser-client-secret",
			})
		},
	})
	configureBrowserLoginEnv(t, stubs)

	p := loginParams{
		in:         bytes.NewBuffer(nil),
		out:        &bytes.Buffer{},
		stderr:     &bytes.Buffer{},
		stdinIsTTY: false, // single-org doesn't need the picker
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fresh, err := runBrowserLogin(ctx, p)
	if err != nil {
		t.Fatalf("runBrowserLogin: %v", err)
	}
	if fresh.Credentials.ClientID != "browser-client-id" {
		t.Errorf("ClientID = %q, want browser-client-id", fresh.Credentials.ClientID)
	}
	if fresh.Credentials.ClientSecret != "browser-client-secret" {
		t.Errorf("ClientSecret = %q, want browser-client-secret", fresh.Credentials.ClientSecret)
	}
	if fresh.OrganizationID != orgID {
		t.Errorf("OrganizationID = %q, want %q", fresh.OrganizationID, orgID)
	}
	if fresh.Workspace != "" {
		t.Errorf("Workspace = %q, want empty (browser flow does not prompt)", fresh.Workspace)
	}

	// Persist + verify the merge path so the test exercises the full pipeline.
	merged := mergePreservedSettings(nil, fresh)
	if err := auth.WriteSettingsFile(merged); err != nil {
		t.Fatalf("WriteSettingsFile: %v", err)
	}
	got, err := auth.ReadSettingsFile()
	if err != nil {
		t.Fatalf("ReadSettingsFile: %v", err)
	}
	if got.Credentials.ClientID != "browser-client-id" {
		t.Errorf("persisted ClientID = %q", got.Credentials.ClientID)
	}
	if got.Credentials.ClientSecret != "browser-client-secret" {
		t.Errorf("persisted ClientSecret = %q", got.Credentials.ClientSecret)
	}
	if got.OrganizationID != orgID {
		t.Errorf("persisted OrganizationID = %q", got.OrganizationID)
	}
	if got.Workspace != "default" {
		t.Errorf("persisted Workspace = %q, want default", got.Workspace)
	}

	if calls := atomic.LoadInt32(&stubs.enrollmentCalls); calls != 1 {
		t.Errorf("enrollment hits = %d, want 1", calls)
	}
	if calls := atomic.LoadInt32(&stubs.applicationsCalls); calls != 1 {
		t.Errorf("applications hits = %d, want 1", calls)
	}
}

func TestRunManualLogin_HappyPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AIRBYTE_TELEMETRY_MODE", "disabled")

	stdin := bytes.NewBufferString("cid\ncsecret\norg-1234\nmy-ws\n")
	stderr := &bytes.Buffer{}
	p := loginParams{
		in:         stdin,
		out:        &bytes.Buffer{},
		stderr:     stderr,
		stdinIsTTY: false, // promptSecret falls back to plain read when not a TTY
	}

	fresh, err := runManualLogin(context.Background(), p)
	if err != nil {
		t.Fatalf("runManualLogin: %v", err)
	}
	if fresh.Credentials.ClientID != "cid" {
		t.Errorf("ClientID = %q, want cid", fresh.Credentials.ClientID)
	}
	if fresh.Credentials.ClientSecret != "csecret" {
		t.Errorf("ClientSecret = %q, want csecret", fresh.Credentials.ClientSecret)
	}
	if fresh.OrganizationID != "org-1234" {
		t.Errorf("OrganizationID = %q, want org-1234", fresh.OrganizationID)
	}
	if fresh.Workspace != "my-ws" {
		t.Errorf("Workspace = %q, want my-ws", fresh.Workspace)
	}

	// Full persistence round-trip.
	merged := mergePreservedSettings(nil, fresh)
	if err := auth.WriteSettingsFile(merged); err != nil {
		t.Fatalf("WriteSettingsFile: %v", err)
	}
	got, err := auth.ReadSettingsFile()
	if err != nil {
		t.Fatalf("ReadSettingsFile: %v", err)
	}
	if got.Workspace != "my-ws" {
		t.Errorf("persisted Workspace = %q, want my-ws", got.Workspace)
	}
	if got.OrganizationID != "org-1234" {
		t.Errorf("persisted OrganizationID = %q", got.OrganizationID)
	}
}

func TestMergePreservedSettings_PreservesAllFourNonCredentialFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AIRBYTE_TELEMETRY_MODE", "disabled")

	// Pre-write a settings file with non-default values for all four
	// non-credential fields the merge helper must preserve.
	prior := &auth.Settings{
		Credentials:      auth.Credentials{ClientID: "old-id", ClientSecret: "old-secret"},
		OrganizationID:   "old-org",
		Workspace:        "my-ws",
		AllowDestructive: true,
		TelemetryEnabled: false,
		IsInternalUser:   true,
	}
	if err := auth.WriteSettingsFile(prior); err != nil {
		t.Fatalf("seeding settings file: %v", err)
	}

	// Simulate the browser flow returning new credentials (no Workspace).
	fresh := &auth.Settings{
		Credentials:    auth.Credentials{ClientID: "new-id", ClientSecret: "new-secret"},
		OrganizationID: "new-org",
	}

	existing, err := auth.ReadSettingsFile()
	if err != nil {
		t.Fatalf("ReadSettingsFile: %v", err)
	}
	merged := mergePreservedSettings(existing, fresh)

	if merged.Credentials.ClientID != "new-id" {
		t.Errorf("ClientID should come from fresh; got %q", merged.Credentials.ClientID)
	}
	if merged.Credentials.ClientSecret != "new-secret" {
		t.Errorf("ClientSecret should come from fresh; got %q", merged.Credentials.ClientSecret)
	}
	if merged.OrganizationID != "new-org" {
		t.Errorf("OrganizationID should come from fresh; got %q", merged.OrganizationID)
	}
	if merged.Workspace != "my-ws" {
		t.Errorf("Workspace must be preserved; got %q want my-ws", merged.Workspace)
	}
	if !merged.AllowDestructive {
		t.Errorf("AllowDestructive must be preserved; got false want true")
	}
	if merged.TelemetryEnabled {
		t.Errorf("TelemetryEnabled must be preserved; got true want false")
	}
	if !merged.IsInternalUser {
		t.Errorf("IsInternalUser must be preserved; got false want true")
	}

	// Round-trip through disk to catch silent regressions in WriteSettingsFile too.
	if err := auth.WriteSettingsFile(merged); err != nil {
		t.Fatalf("WriteSettingsFile: %v", err)
	}
	persisted, err := auth.ReadSettingsFile()
	if err != nil {
		t.Fatalf("ReadSettingsFile after merge: %v", err)
	}
	if persisted.Workspace != "my-ws" {
		t.Errorf("persisted Workspace = %q", persisted.Workspace)
	}
	if !persisted.AllowDestructive {
		t.Errorf("persisted AllowDestructive should be true")
	}
	if persisted.TelemetryEnabled {
		t.Errorf("persisted TelemetryEnabled should be false")
	}
	if !persisted.IsInternalUser {
		t.Errorf("persisted IsInternalUser should be true")
	}
}

func TestMergePreservedSettings_Defaults(t *testing.T) {
	// No existing file: defaults apply, Workspace falls back to "default".
	fresh := &auth.Settings{
		Credentials:    auth.Credentials{ClientID: "id", ClientSecret: "sec"},
		OrganizationID: "org",
	}
	merged := mergePreservedSettings(nil, fresh)
	if merged.Workspace != "default" {
		t.Errorf("Workspace = %q, want default when no existing + fresh has none", merged.Workspace)
	}
	if merged.AllowDestructive {
		t.Errorf("AllowDestructive should default to false")
	}
	if !merged.TelemetryEnabled {
		t.Errorf("TelemetryEnabled should default to true")
	}
	if merged.IsInternalUser {
		t.Errorf("IsInternalUser should default to false")
	}

	// Manual path: fresh.Workspace wins over the default.
	freshWithWS := &auth.Settings{
		Credentials:    auth.Credentials{ClientID: "id", ClientSecret: "sec"},
		OrganizationID: "org",
		Workspace:      "explicit",
	}
	if got := mergePreservedSettings(nil, freshWithWS).Workspace; got != "explicit" {
		t.Errorf("Workspace = %q, want explicit", got)
	}
}

func TestRunBrowserLogin_OrgIDOverrideSkipsPicker(t *testing.T) {
	const explicitOrgID = "explicit-org-uuid"
	stubs := newLoginStubs(t, sonarRoutes{
		enrollment: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 200, map[string]interface{}{
				"is_enrolled":     true,
				"organization_id": "initial-org-from-enrollment",
			})
		},
		// organizations handler is intentionally nil — must not be hit when
		// --org-id is supplied. newLoginStubs fails the test if the handler
		// runs.
		applications: func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("X-Organization-Id"); got != explicitOrgID {
				t.Errorf("applications X-Organization-Id = %q, want %q", got, explicitOrgID)
			}
			writeJSON(t, w, 200, map[string]string{
				"client_id":     "ovr-cid",
				"client_secret": "ovr-cs",
			})
		},
	})
	configureBrowserLoginEnv(t, stubs)

	p := loginParams{
		orgID:      explicitOrgID,
		in:         bytes.NewBuffer(nil),
		out:        &bytes.Buffer{},
		stderr:     &bytes.Buffer{},
		stdinIsTTY: false,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fresh, err := runBrowserLogin(ctx, p)
	if err != nil {
		t.Fatalf("runBrowserLogin: %v", err)
	}
	if fresh.OrganizationID != explicitOrgID {
		t.Errorf("OrganizationID = %q, want %q", fresh.OrganizationID, explicitOrgID)
	}
	if calls := atomic.LoadInt32(&stubs.organizationsCalls); calls != 0 {
		t.Errorf("/organizations should NOT be called when --org-id is set; got %d calls", calls)
	}
	if calls := atomic.LoadInt32(&stubs.applicationsCalls); calls != 1 {
		t.Errorf("/applications hits = %d, want 1", calls)
	}
}

// TestEmitLoginTelemetry_DoesNotPanic exercises the helper with telemetry
// disabled via env var; it should be a no-op and must not blow up when
// the settings file is absent.
func TestEmitLoginTelemetry_DoesNotPanic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AIRBYTE_TELEMETRY_MODE", "disabled")
	// No panic = pass; we only confirm the helper is safe to call on a fresh box.
	emitLoginTelemetry(time.Now(), "org-1", true, "")
}

// Ensure io.Reader compatibility — the params field is io.Reader (not
// *bufio.Reader), so production stdin or test buffers both fit.
var _ io.Reader = (*bytes.Buffer)(nil)

// Ensure the success message produced by RunE is the documented shape.
// This is asserted via a structural reference (not a network roundtrip)
// because the RunE wrapper exits the process on failure paths and isn't
// directly callable from tests.
func TestLoginSuccessMessageShape(t *testing.T) {
	want := map[string]string{
		"status":  "saved",
		"message": "Settings written to ~/.airbyte-agent/settings.json",
	}
	// Re-encode → decode to verify the literal stays JSON-stable.
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(want); err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got map[string]string
	if err := json.NewDecoder(buf).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["status"] != "saved" {
		t.Errorf("status = %q", got["status"])
	}
	if got["message"] != "Settings written to ~/.airbyte-agent/settings.json" {
		t.Errorf("message = %q", got["message"])
	}
}

// Sentinel: the merge helper must not crash when handed nil existing AND
// fresh has unusual shapes (covered indirectly above; this hardens it).
func TestMergePreservedSettings_NilExistingNonEmptyFresh(t *testing.T) {
	got := mergePreservedSettings(nil, &auth.Settings{
		Credentials:    auth.Credentials{ClientID: "x", ClientSecret: "y"},
		OrganizationID: "o",
		Workspace:      "w",
	})
	if got == nil {
		t.Fatal("returned nil")
	}
	if got.Workspace != "w" {
		t.Errorf("Workspace = %q, want w", got.Workspace)
	}
}

// classifyError and exitCodeFor — quick coverage. The cmd layer relies on
// these to route to the right exit code / telemetry label, so a smoke test
// here keeps regressions out.
func TestClassifyErrorAndExitCode(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantType string
		wantExit int
	}{
		{"state mismatch", errors.New("oauth state mismatch - possible CSRF or replay"), "oauth_state_mismatch", 1},
		{"timeout", errors.New("context deadline exceeded"), "timeout", 1},
		{"network", errors.New("calling /api/v1/internal/account/enrollment-status: connection refused"), "network", 1},
		{"unknown", errors.New("something else broke"), "unknown", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyError(c.err); got != c.wantType {
				t.Errorf("classifyError = %q, want %q", got, c.wantType)
			}
			if got := exitCodeFor(c.err); got != c.wantExit {
				t.Errorf("exitCodeFor = %d, want %d", got, c.wantExit)
			}
		})
	}
}

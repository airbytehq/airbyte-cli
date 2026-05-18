package browserlogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/airbytehq/airbyte-cli/internal/client"
)

// stubSonar builds an httptest.Server that dispatches sonar bootstrap endpoints
// by path. Each handler is optional — unset handlers fail the test if hit.
type stubSonar struct {
	srv             *httptest.Server
	enrollmentCalls int32
	orgsCalls       int32
	appsCalls       int32
}

type sonarHandlers struct {
	enrollment http.HandlerFunc
	orgs       http.HandlerFunc
	apps       http.HandlerFunc
}

func newStubSonar(t *testing.T, h sonarHandlers) *stubSonar {
	t.Helper()
	s := &stubSonar{}
	mux := http.NewServeMux()
	mux.HandleFunc(APIBasePath+enrollmentStatusPath, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s.enrollmentCalls, 1)
		if h.enrollment == nil {
			t.Errorf("unexpected enrollment call")
			http.Error(w, "no handler", http.StatusInternalServerError)
			return
		}
		h.enrollment(w, r)
	})
	mux.HandleFunc(APIBasePath+organizationsListPath, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s.orgsCalls, 1)
		if h.orgs == nil {
			t.Errorf("unexpected organizations call")
			http.Error(w, "no handler", http.StatusInternalServerError)
			return
		}
		h.orgs(w, r)
	})
	mux.HandleFunc(APIBasePath+applicationsPath, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s.appsCalls, 1)
		if h.apps == nil {
			t.Errorf("unexpected applications call")
			http.Error(w, "no handler", http.StatusInternalServerError)
			return
		}
		h.apps(w, r)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
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

func TestBootstrap_SingleOrgHappyPath(t *testing.T) {
	const orgID = "org-uuid-single"
	s := newStubSonar(t, sonarHandlers{
		enrollment: func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer access-token-abc" {
				t.Errorf("missing/wrong Authorization header: %q", got)
			}
			if got := r.Header.Get(headerAgentCLI); got != "test-version" {
				t.Errorf("missing X-ADP-Agent-CLI header: %q", got)
			}
			if got := r.Header.Get(headerOrgID); got != "" {
				t.Errorf("enrollment-status should NOT send X-Organization-Id, got %q", got)
			}
			writeJSON(t, w, 200, map[string]interface{}{
				"is_new_user":       false,
				"is_enrolled":       true,
				"is_instance_admin": false,
				"organization_id":   orgID,
			})
		},
		orgs: func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get(headerOrgID); got != orgID {
				t.Errorf("organizations missing X-Organization-Id; got %q", got)
			}
			writeJSON(t, w, 200, map[string]interface{}{
				"organizations": []map[string]string{
					{"id": orgID, "organization_name": "Solo"},
				},
				"is_instance_admin": false,
			})
		},
		apps: func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get(headerOrgID); got != orgID {
				t.Errorf("applications missing X-Organization-Id; got %q", got)
			}
			if r.Method != http.MethodPost {
				t.Errorf("applications wants POST, got %s", r.Method)
			}
			writeJSON(t, w, 201, map[string]string{
				"id":            "app-1",
				"name":          "Airbyte Agents",
				"client_id":     "ci",
				"client_secret": "cs",
			})
		},
	})

	res, err := Bootstrap(context.Background(), &BootstrapOptions{
		APIHost:     s.srv.URL,
		AccessToken: "access-token-abc",
		CLIVersion:  "test-version",
		HTTPClient:  s.srv.Client(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ClientID != "ci" {
		t.Errorf("ClientID = %q, want %q", res.ClientID, "ci")
	}
	if res.ClientSecret != "cs" {
		t.Errorf("ClientSecret = %q, want %q", res.ClientSecret, "cs")
	}
	if res.OrganizationID != orgID {
		t.Errorf("OrganizationID = %q, want %q", res.OrganizationID, orgID)
	}
	if got := atomic.LoadInt32(&s.enrollmentCalls); got != 1 {
		t.Errorf("enrollment calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&s.orgsCalls); got != 1 {
		t.Errorf("orgs calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&s.appsCalls); got != 1 {
		t.Errorf("apps calls = %d, want 1", got)
	}
}

func TestBootstrap_MultiOrgWithOverride(t *testing.T) {
	const initialOrg = "org-initial"
	const explicit = "explicit-org-uuid"
	s := newStubSonar(t, sonarHandlers{
		enrollment: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 200, map[string]interface{}{
				"is_enrolled":     true,
				"organization_id": initialOrg,
			})
		},
		// orgs handler intentionally left nil — must not be called.
		apps: func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get(headerOrgID); got != explicit {
				t.Errorf("applications got X-Organization-Id %q, want %q", got, explicit)
			}
			writeJSON(t, w, 200, map[string]string{
				"client_id":     "ci2",
				"client_secret": "cs2",
			})
		},
	})

	res, err := Bootstrap(context.Background(), &BootstrapOptions{
		APIHost:       s.srv.URL,
		AccessToken:   "tok",
		OrgIDOverride: explicit,
		HTTPClient:    s.srv.Client(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OrganizationID != explicit {
		t.Errorf("OrganizationID = %q, want %q", res.OrganizationID, explicit)
	}
	if got := atomic.LoadInt32(&s.orgsCalls); got != 0 {
		t.Errorf("organizations endpoint should NOT be called when --org-id is set, got %d calls", got)
	}
}

func TestBootstrap_MultiOrgInteractivePicker(t *testing.T) {
	orgs := []map[string]string{
		{"id": "uuid-1", "organization_name": "Alpha"},
		{"id": "uuid-2", "organization_name": "Beta"},
		{"id": "uuid-3", "organization_name": "Gamma"},
	}
	s := newStubSonar(t, sonarHandlers{
		enrollment: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 200, map[string]interface{}{
				"is_enrolled":     true,
				"organization_id": "uuid-1",
			})
		},
		orgs: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 200, map[string]interface{}{
				"organizations":     orgs,
				"is_instance_admin": false,
			})
		},
		apps: func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get(headerOrgID); got != "uuid-2" {
				t.Errorf("applications X-Organization-Id = %q, want uuid-2", got)
			}
			writeJSON(t, w, 201, map[string]string{
				"client_id":     "ci3",
				"client_secret": "cs3",
			})
		},
	})

	stderr := &bytes.Buffer{}
	res, err := Bootstrap(context.Background(), &BootstrapOptions{
		APIHost:     s.srv.URL,
		AccessToken: "tok",
		Stdin:       bytes.NewBufferString("2\n"),
		Stderr:      stderr,
		StdinIsTTY:  true,
		HTTPClient:  s.srv.Client(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OrganizationID != "uuid-2" {
		t.Errorf("OrganizationID = %q, want uuid-2", res.OrganizationID)
	}
	if out := stderr.String(); !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") || !strings.Contains(out, "Gamma") {
		t.Errorf("stderr missing org list: %q", out)
	}
}

func TestBootstrap_MultiOrgNonTTY(t *testing.T) {
	s := newStubSonar(t, sonarHandlers{
		enrollment: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 200, map[string]interface{}{
				"is_enrolled":     true,
				"organization_id": "uuid-1",
			})
		},
		orgs: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 200, map[string]interface{}{
				"organizations": []map[string]string{
					{"id": "uuid-1", "organization_name": "Alpha"},
					{"id": "uuid-2", "organization_name": "Beta"},
				},
				"is_instance_admin": false,
			})
		},
	})

	_, err := Bootstrap(context.Background(), &BootstrapOptions{
		APIHost:     s.srv.URL,
		AccessToken: "tok",
		StdinIsTTY:  false,
		HTTPClient:  s.srv.Client(),
	})
	if err == nil {
		t.Fatal("expected error when stdin is not a TTY and there are multiple orgs")
	}
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *client.APIError: %T %v", err, err)
	}
	if apiErr.Type != "validation_error" {
		t.Errorf("Type = %q, want validation_error", apiErr.Type)
	}
	if !strings.Contains(apiErr.Hint, "--org-id") {
		t.Errorf("Hint missing --org-id guidance: %q", apiErr.Hint)
	}
	if !strings.Contains(apiErr.Hint, "Alpha") || !strings.Contains(apiErr.Hint, "Beta") {
		t.Errorf("Hint missing org names: %q", apiErr.Hint)
	}
}

func TestBootstrap_Unenrolled(t *testing.T) {
	s := newStubSonar(t, sonarHandlers{
		enrollment: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 200, map[string]interface{}{
				"is_enrolled": false,
			})
		},
	})

	_, err := Bootstrap(context.Background(), &BootstrapOptions{
		APIHost:     s.srv.URL,
		AccessToken: "tok",
		HTTPClient:  s.srv.Client(),
	})
	if err == nil {
		t.Fatal("expected error for unenrolled user")
	}
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *client.APIError: %T %v", err, err)
	}
	if apiErr.Type != "validation_error" {
		t.Errorf("Type = %q, want validation_error", apiErr.Type)
	}
	if !strings.Contains(strings.ToLower(apiErr.Hint), "enrollment") {
		t.Errorf("Hint should mention enrollment, got %q", apiErr.Hint)
	}
	if apiErr.ExitCode() != client.ExitValidation {
		t.Errorf("ExitCode = %d, want %d", apiErr.ExitCode(), client.ExitValidation)
	}
}

func TestBootstrap_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	// Close immediately so all subsequent requests fail with connection refused.
	srv.Close()

	_, err := Bootstrap(context.Background(), &BootstrapOptions{
		APIHost:     srv.URL,
		AccessToken: "tok",
		HTTPClient:  &http.Client{},
	})
	if err == nil {
		t.Fatal("expected error from closed server")
	}
	// Network errors must NOT be typed APIErrors — the caller's exit-code logic
	// should only fire for HTTP-status-based failures.
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		t.Errorf("network error should not be *client.APIError, got %v", err)
	}
}

func TestBootstrap_ApplicationsForbiddenRemappedToValidation(t *testing.T) {
	s := newStubSonar(t, sonarHandlers{
		enrollment: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 200, map[string]interface{}{
				"is_enrolled":     true,
				"organization_id": "uuid-1",
			})
		},
		orgs: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 200, map[string]interface{}{
				"organizations": []map[string]string{
					{"id": "uuid-1", "organization_name": "Alpha"},
				},
				"is_instance_admin": false,
			})
		},
		apps: func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, 403, map[string]string{"detail": "subscription inactive"})
		},
	})

	_, err := Bootstrap(context.Background(), &BootstrapOptions{
		APIHost:     s.srv.URL,
		AccessToken: "tok",
		HTTPClient:  s.srv.Client(),
	})
	if err == nil {
		t.Fatal("expected error on 403 from applications")
	}
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *client.APIError: %T %v", err, err)
	}
	if apiErr.Type != "validation_error" {
		t.Errorf("403 should be remapped to validation_error, got %q", apiErr.Type)
	}
	if !strings.Contains(strings.ToLower(apiErr.Hint), "onboarding") {
		t.Errorf("Hint should mention onboarding, got %q", apiErr.Hint)
	}
}

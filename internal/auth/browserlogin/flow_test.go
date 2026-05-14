package browserlogin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeKeycloak builds an httptest.Server that handles
// /protocol/openid-connect/token. The handler is provided by the caller so each
// test can customize the response (happy path, error response, etc.).
func fakeKeycloak(t *testing.T, tokenHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/protocol/openid-connect/token", tokenHandler)
	srv := httptest.NewServer(mux)
	return srv
}

// fakeBrowser returns an OpenFunc that mimics the user completing the
// authorize step: it extracts redirect_uri and state from the authorize URL
// and POSTs (well, GETs — OAuth uses GET for the redirect) to the loopback
// with the supplied callbackState (which may differ from the real state to
// exercise the mismatch path) and the supplied code.
func fakeBrowser(t *testing.T, callbackCode string, useReturnedState bool, overrideState string) func(string) error {
	t.Helper()
	return func(authURL string) error {
		go func() {
			u, err := url.Parse(authURL)
			if err != nil {
				t.Errorf("fake browser: parse authURL: %v", err)
				return
			}
			q := u.Query()
			redirectURI := q.Get("redirect_uri")
			state := q.Get("state")
			if !useReturnedState {
				state = overrideState
			}
			cb := fmt.Sprintf("%s?code=%s&state=%s",
				redirectURI,
				url.QueryEscape(callbackCode),
				url.QueryEscape(state),
			)
			resp, err := http.Get(cb)
			if err != nil {
				t.Errorf("fake browser: GET callback: %v", err)
				return
			}
			resp.Body.Close()
		}()
		return nil
	}
}

func TestRunOAuthFlow_HappyPath(t *testing.T) {
	t.Parallel()

	var tokenHits int32
	var capturedForm url.Values
	srv := fakeKeycloak(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenHits, 1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		capturedForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"at","refresh_token":"rt","id_token":"it","token_type":"Bearer","expires_in":300}`)
	})
	defer srv.Close()

	opts := &Options{
		KeycloakBase: srv.URL,
		ClientID:     "test-client",
		Scopes:       "openid",
		Timeout:      5 * time.Second,
		OpenFunc:     fakeBrowser(t, "fake-code", true, ""),
		HTTPClient:   srv.Client(),
	}

	tokens, err := RunOAuthFlow(context.Background(), opts)
	if err != nil {
		t.Fatalf("RunOAuthFlow: %v", err)
	}
	if tokens == nil {
		t.Fatal("expected non-nil tokens")
	}
	if tokens.AccessToken != "at" {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, "at")
	}
	if tokens.RefreshToken != "rt" {
		t.Errorf("RefreshToken = %q, want %q", tokens.RefreshToken, "rt")
	}
	if tokens.IDToken != "it" {
		t.Errorf("IDToken = %q, want %q", tokens.IDToken, "it")
	}
	if tokens.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want Bearer", tokens.TokenType)
	}
	if d := time.Until(tokens.ExpiresAt); d < 4*time.Minute || d > 6*time.Minute {
		t.Errorf("ExpiresAt should be ~5min in the future, got %v", d)
	}
	if got := atomic.LoadInt32(&tokenHits); got != 1 {
		t.Errorf("token endpoint hit %d times, want 1", got)
	}
	if capturedForm.Get("grant_type") != "authorization_code" {
		t.Errorf("grant_type = %q, want authorization_code", capturedForm.Get("grant_type"))
	}
	if capturedForm.Get("code") != "fake-code" {
		t.Errorf("code = %q, want fake-code", capturedForm.Get("code"))
	}
	if capturedForm.Get("client_id") != "test-client" {
		t.Errorf("client_id = %q, want test-client", capturedForm.Get("client_id"))
	}
	if capturedForm.Get("code_verifier") == "" {
		t.Error("code_verifier should be non-empty")
	}
	if !strings.HasPrefix(capturedForm.Get("redirect_uri"), "http://127.0.0.1:") {
		t.Errorf("redirect_uri = %q, want loopback URI", capturedForm.Get("redirect_uri"))
	}
}

func TestRunOAuthFlow_StateMismatch(t *testing.T) {
	t.Parallel()

	srv := fakeKeycloak(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("token endpoint should NOT be hit on state mismatch")
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	opts := &Options{
		KeycloakBase: srv.URL,
		ClientID:     "test-client",
		Timeout:      5 * time.Second,
		// Use a state value that won't match the random one the flow generates.
		OpenFunc:   fakeBrowser(t, "fake-code", false, "definitely-wrong-state"),
		HTTPClient: srv.Client(),
	}

	tokens, err := RunOAuthFlow(context.Background(), opts)
	if err == nil {
		t.Fatal("expected state-mismatch error, got nil")
	}
	if tokens != nil {
		t.Errorf("expected nil tokens on error, got %+v", tokens)
	}
	if !strings.Contains(err.Error(), "state mismatch") {
		t.Errorf("error %q should mention state mismatch", err.Error())
	}
}

func TestRunOAuthFlow_TokenEndpointError(t *testing.T) {
	t.Parallel()

	srv := fakeKeycloak(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant","error_description":"bad code"}`)
	})
	defer srv.Close()

	opts := &Options{
		KeycloakBase: srv.URL,
		ClientID:     "test-client",
		Timeout:      5 * time.Second,
		OpenFunc:     fakeBrowser(t, "fake-code", true, ""),
		HTTPClient:   srv.Client(),
	}

	tokens, err := RunOAuthFlow(context.Background(), opts)
	if err == nil {
		t.Fatal("expected token-endpoint error, got nil")
	}
	if tokens != nil {
		t.Errorf("expected nil tokens on error, got %+v", tokens)
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error %q should wrap upstream error code", err.Error())
	}
	if !strings.Contains(err.Error(), "bad code") {
		t.Errorf("error %q should include upstream description", err.Error())
	}
}

func TestRunOAuthFlow_Timeout(t *testing.T) {
	t.Parallel()

	srv := fakeKeycloak(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("token endpoint should NOT be hit on timeout")
	})
	defer srv.Close()

	opts := &Options{
		KeycloakBase: srv.URL,
		ClientID:     "test-client",
		Timeout:      100 * time.Millisecond,
		// No fake browser callback — Wait will time out.
		OpenFunc:   func(string) error { return nil },
		HTTPClient: srv.Client(),
	}

	start := time.Now()
	tokens, err := RunOAuthFlow(context.Background(), opts)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if tokens != nil {
		t.Errorf("expected nil tokens on timeout, got %+v", tokens)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error should wrap context.DeadlineExceeded, got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("flow took %v, expected ~100ms", elapsed)
	}
}

func TestBuildAuthorizeURL(t *testing.T) {
	t.Parallel()

	got := buildAuthorizeURL(
		"https://example.com/auth/realms/r",
		"client",
		"http://127.0.0.1:12345/callback",
		"openid email",
		"abc",
		"xyz",
	)
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Path != "/auth/realms/r/protocol/openid-connect/auth" {
		t.Errorf("path = %q", u.Path)
	}
	q := u.Query()
	cases := map[string]string{
		"response_type":         "code",
		"client_id":             "client",
		"redirect_uri":          "http://127.0.0.1:12345/callback",
		"scope":                 "openid email",
		"state":                 "abc",
		"code_challenge":        "xyz",
		"code_challenge_method": "S256",
	}
	for k, want := range cases {
		if got := q.Get(k); got != want {
			t.Errorf("query %s = %q, want %q", k, got, want)
		}
	}
}

func TestOptions_Defaults(t *testing.T) {
	t.Parallel()

	// Nil receiver path — keeps RunOAuthFlow callers from having to allocate.
	var opts *Options
	if opts.keycloakBase() != DefaultKeycloakBase {
		t.Errorf("keycloakBase() = %q, want default", opts.keycloakBase())
	}
	if opts.clientID() != DefaultClientID {
		t.Errorf("clientID() = %q, want default", opts.clientID())
	}
	if opts.scopes() != DefaultScopes {
		t.Errorf("scopes() = %q, want default", opts.scopes())
	}
	if opts.timeout() != DefaultTimeout {
		t.Errorf("timeout() = %v, want default", opts.timeout())
	}
	// Zero-value struct path.
	zero := &Options{}
	if zero.keycloakBase() != DefaultKeycloakBase {
		t.Errorf("zero.keycloakBase() = %q", zero.keycloakBase())
	}
	if zero.clientID() != DefaultClientID {
		t.Errorf("zero.clientID() = %q", zero.clientID())
	}
}

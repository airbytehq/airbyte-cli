package browserlogin

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/airbytehq/airbyte-cli/internal/browser"
)

// Tokens is the result of a successful Keycloak login. Tokens are transient
// in-memory only — the caller (cmd/login.go in Phase 4) uses AccessToken to
// call sonar and then discards them. We do NOT persist Keycloak tokens to
// disk; the CLI's long-lived credential is the workspace API key, not the
// Keycloak refresh token.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	TokenType    string
	ExpiresAt    time.Time
}

// Options configures RunOAuthFlow. Production callers may leave the struct
// zero-valued; defaults are derived from the Default* constants. Tests use
// OpenFunc and HTTPClient as injection seams.
type Options struct {
	// KeycloakBase is the Keycloak realm authority (no trailing slash, no
	// trailing /protocol/... path). Defaults to DefaultKeycloakBase.
	KeycloakBase string
	// ClientID is the public Keycloak client id. Defaults to DefaultClientID.
	ClientID string
	// Scopes is a space-separated OIDC scope string. Defaults to DefaultScopes.
	Scopes string
	// Timeout caps the entire flow. Defaults to DefaultTimeout.
	Timeout time.Duration
	// OpenFunc is a test seam for substituting the browser launcher.
	// Production callers should leave this nil — the package-level
	// browser.Open is used instead.
	OpenFunc func(string) error
	// HTTPClient is an optional override for the Keycloak token-exchange call.
	// Tests use this to point at an httptest.Server; nil means a default
	// client whose Timeout matches Options.timeout().
	HTTPClient *http.Client
	// Stderr receives user-facing messages emitted by the flow (currently only
	// the "couldn't open browser" fallback). nil → os.Stderr.
	Stderr io.Writer
}

func (o *Options) keycloakBase() string {
	if o != nil && o.KeycloakBase != "" {
		return o.KeycloakBase
	}
	return DefaultKeycloakBase
}

func (o *Options) clientID() string {
	if o != nil && o.ClientID != "" {
		return o.ClientID
	}
	return DefaultClientID
}

func (o *Options) scopes() string {
	if o != nil && o.Scopes != "" {
		return o.Scopes
	}
	return DefaultScopes
}

func (o *Options) timeout() time.Duration {
	if o != nil && o.Timeout > 0 {
		return o.Timeout
	}
	return DefaultTimeout
}

func (o *Options) openFunc() func(string) error {
	if o != nil && o.OpenFunc != nil {
		return o.OpenFunc
	}
	return browser.Open
}

func (o *Options) httpClient() *http.Client {
	if o != nil && o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: o.timeout()}
}

func (o *Options) stderr() io.Writer {
	if o != nil && o.Stderr != nil {
		return o.Stderr
	}
	return os.Stderr
}

// tokenResponse mirrors the Keycloak OIDC token-endpoint JSON. The Error and
// ErrorDesc fields cover the RFC 6749 error response shape; both are empty on
// success.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// RunOAuthFlow performs a complete browser-based Keycloak login using PKCE
// (S256). On success the user has completed the redirect, the loopback
// captured the authorization code, and we have exchanged it at the Keycloak
// token endpoint. The returned Tokens are valid only for the lifetime of the
// caller — nothing is persisted.
func RunOAuthFlow(ctx context.Context, opts *Options) (*Tokens, error) {
	verifier, err := GenerateVerifier()
	if err != nil {
		return nil, fmt.Errorf("pkce verifier: %w", err)
	}
	challenge := Challenge(verifier)
	state, err := GenerateState()
	if err != nil {
		return nil, fmt.Errorf("oauth state: %w", err)
	}

	lb, err := startLoopback()
	if err != nil {
		return nil, err
	}
	defer lb.Shutdown()

	flowCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", lb.Port, CallbackPath)
	authorizeURL := buildAuthorizeURL(
		opts.keycloakBase(), opts.clientID(), redirectURI,
		opts.scopes(), state, challenge,
	)

	// If the browser launcher itself fails (no browser binary, headless machine,
	// etc.) we still wait on the loopback so the user can paste the URL manually,
	// but we surface the failure and print the URL so they know what to do
	// instead of staring at a frozen terminal for three minutes.
	if err := opts.openFunc()(authorizeURL); err != nil {
		fmt.Fprintf(opts.stderr(),
			"could not auto-open browser (%v).\nPaste this URL manually:\n  %s\n",
			err, authorizeURL)
	}

	code, returnedState, err := lb.Wait(flowCtx)
	if err != nil {
		return nil, fmt.Errorf("waiting for browser callback: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(state), []byte(returnedState)) != 1 {
		return nil, errors.New("oauth state mismatch - possible CSRF or replay")
	}

	tokens, err := exchangeCodeForTokens(flowCtx, opts, code, verifier, redirectURI)
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

// buildAuthorizeURL composes the Keycloak /protocol/openid-connect/auth URL
// with all PKCE + OIDC parameters URL-encoded.
func buildAuthorizeURL(base, clientID, redirectURI, scopes, state, challenge string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scopes)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return strings.TrimRight(base, "/") + "/protocol/openid-connect/auth?" + q.Encode()
}

// exchangeCodeForTokens POSTs the authorization code + PKCE verifier to
// Keycloak's token endpoint and decodes the response.
func exchangeCodeForTokens(ctx context.Context, opts *Options, code, verifier, redirectURI string) (*Tokens, error) {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", code)
	body.Set("redirect_uri", redirectURI)
	body.Set("client_id", opts.clientID())
	body.Set("code_verifier", verifier)

	tokenURL := strings.TrimRight(opts.keycloakBase(), "/") + "/protocol/openid-connect/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := opts.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	var tr tokenResponse
	if jsonErr := json.Unmarshal(raw, &tr); jsonErr != nil {
		// Surface status code in the wrapper so non-JSON 5xx pages are
		// debuggable even though we can't parse them.
		return nil, fmt.Errorf("parsing token response (status %d): %w", resp.StatusCode, jsonErr)
	}
	if resp.StatusCode != http.StatusOK || tr.Error != "" {
		if tr.Error != "" {
			return nil, fmt.Errorf("token endpoint error %s: %s", tr.Error, tr.ErrorDesc)
		}
		return nil, fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}

	return &Tokens{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		IDToken:      tr.IDToken,
		TokenType:    tr.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}

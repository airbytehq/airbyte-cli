package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	tokenPath        = "/api/v1/account/applications/token"
	refreshThreshold = 1 * time.Minute
)

type tokenRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type tokenResponse struct {
	AccessToken    string `json:"access_token"`
	TokenType      string `json:"token_type"`
	ExpiresIn      int    `json:"expires_in"`
	OrganizationID string `json:"organization_id"`
}

type TokenManager struct {
	apiHost        string
	organizationID string
	credentials    *Credentials
	httpClient     *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func NewTokenManager(apiHost, organizationID string, creds *Credentials) *TokenManager {
	return &TokenManager{
		apiHost:        apiHost,
		organizationID: organizationID,
		credentials:    creds,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (tm *TokenManager) GetToken() (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.token != "" && time.Now().Before(tm.expiresAt.Add(-refreshThreshold)) {
		return tm.token, nil
	}

	return tm.refresh()
}

func (tm *TokenManager) refresh() (string, error) {
	body, err := json.Marshal(tokenRequest{
		ClientID:     tm.credentials.ClientID,
		ClientSecret: tm.credentials.ClientSecret,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling token request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, tm.apiHost+tokenPath, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if tm.organizationID != "" {
		req.Header.Set("X-Organization-Id", tm.organizationID)
	}

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token exchange returned empty access_token")
	}

	tm.token = tokenResp.AccessToken
	tm.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return tm.token, nil
}

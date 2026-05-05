package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenManager_Exchange(t *testing.T) {
	var received tokenRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != tokenPath {
			t.Errorf("expected path %s, got %s", tokenPath, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decoding request: %v", err)
		}

		resp := tokenResponse{
			AccessToken: "test-token-123",
			TokenType:   "bearer",
			ExpiresIn:   1200,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds := &Credentials{ClientID: "my-id", ClientSecret: "my-secret"}
	tm := NewTokenManager(server.URL, "", creds)

	token, err := tm.GetToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "test-token-123" {
		t.Errorf("token = %q, want %q", token, "test-token-123")
	}
	if received.ClientID != "my-id" {
		t.Errorf("received ClientID = %q, want %q", received.ClientID, "my-id")
	}
	if received.ClientSecret != "my-secret" {
		t.Errorf("received ClientSecret = %q, want %q", received.ClientSecret, "my-secret")
	}
}

func TestTokenManager_OrgIDHeader(t *testing.T) {
	var gotOrgID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrgID = r.Header.Get("X-Organization-Id")
		resp := tokenResponse{AccessToken: "tok", ExpiresIn: 1200}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds := &Credentials{ClientID: "id", ClientSecret: "secret"}
	tm := NewTokenManager(server.URL, "org-abc", creds)

	_, err := tm.GetToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOrgID != "org-abc" {
		t.Errorf("X-Organization-Id = %q, want %q", gotOrgID, "org-abc")
	}
}

func TestTokenManager_Caching(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		resp := tokenResponse{AccessToken: "cached-token", ExpiresIn: 1200}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds := &Credentials{ClientID: "id", ClientSecret: "secret"}
	tm := NewTokenManager(server.URL, "", creds)

	for i := 0; i < 5; i++ {
		token, err := tm.GetToken()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if token != "cached-token" {
			t.Errorf("call %d: token = %q, want %q", i, token, "cached-token")
		}
	}

	if count := callCount.Load(); count != 1 {
		t.Errorf("server called %d times, want 1", count)
	}
}

func TestTokenManager_RefreshOnExpiry(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		resp := tokenResponse{
			AccessToken: "token-" + string(rune('0'+count)),
			ExpiresIn:   1200,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds := &Credentials{ClientID: "id", ClientSecret: "secret"}
	tm := NewTokenManager(server.URL, "", creds)

	_, err := tm.GetToken()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	tm.mu.Lock()
	tm.expiresAt = time.Now().Add(30 * time.Second)
	tm.mu.Unlock()

	_, err = tm.GetToken()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if count := callCount.Load(); count != 2 {
		t.Errorf("server called %d times, want 2", count)
	}
}

func TestTokenManager_ConcurrentAccess(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		time.Sleep(10 * time.Millisecond)
		resp := tokenResponse{AccessToken: "concurrent-token", ExpiresIn: 1200}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds := &Credentials{ClientID: "id", ClientSecret: "secret"}
	tm := NewTokenManager(server.URL, "", creds)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := tm.GetToken()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if token != "concurrent-token" {
				t.Errorf("token = %q, want %q", token, "concurrent-token")
			}
		}()
	}
	wg.Wait()

	if count := callCount.Load(); count != 1 {
		t.Errorf("server called %d times, want 1 (mutex should prevent concurrent exchanges)", count)
	}
}

func TestTokenManager_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"detail": "server broke"}`))
	}))
	defer server.Close()

	creds := &Credentials{ClientID: "id", ClientSecret: "secret"}
	tm := NewTokenManager(server.URL, "", creds)

	_, err := tm.GetToken()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

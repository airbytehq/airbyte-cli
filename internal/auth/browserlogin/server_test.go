package browserlogin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestStartLoopback_HappyPath(t *testing.T) {
	t.Parallel()

	lb, err := startLoopback()
	if err != nil {
		t.Fatalf("startLoopback: %v", err)
	}
	defer lb.Shutdown()

	if lb.Port <= 0 {
		t.Fatalf("expected positive ephemeral port, got %d", lb.Port)
	}

	callbackURL := fmt.Sprintf("http://127.0.0.1:%d%s?code=abc&state=xyz", lb.Port, CallbackPath)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Login successful") {
		t.Errorf("expected success HTML body, got %q", string(body))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	code, state, waitErr := lb.Wait(ctx)
	if waitErr != nil {
		t.Fatalf("Wait returned error: %v", waitErr)
	}
	if code != "abc" {
		t.Errorf("code = %q, want %q", code, "abc")
	}
	if state != "xyz" {
		t.Errorf("state = %q, want %q", state, "xyz")
	}
}

func TestStartLoopback_ErrorCallback(t *testing.T) {
	t.Parallel()

	lb, err := startLoopback()
	if err != nil {
		t.Fatalf("startLoopback: %v", err)
	}
	defer lb.Shutdown()

	callbackURL := fmt.Sprintf(
		"http://127.0.0.1:%d%s?error=access_denied&error_description=user+cancelled",
		lb.Port, CallbackPath,
	)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("GET error callback: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Login failed") {
		t.Errorf("expected failure HTML body, got %q", string(body))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, waitErr := lb.Wait(ctx)
	if waitErr == nil {
		t.Fatal("expected non-nil error from Wait on error callback")
	}
	if !strings.Contains(waitErr.Error(), "access_denied") {
		t.Errorf("error %q should mention access_denied", waitErr.Error())
	}
	if !strings.Contains(waitErr.Error(), "user cancelled") {
		t.Errorf("error %q should include description", waitErr.Error())
	}
}

func TestStartLoopback_MissingCodeOrState(t *testing.T) {
	t.Parallel()

	lb, err := startLoopback()
	if err != nil {
		t.Fatalf("startLoopback: %v", err)
	}
	defer lb.Shutdown()

	callbackURL := fmt.Sprintf("http://127.0.0.1:%d%s?code=onlycode", lb.Port, CallbackPath)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 on missing state, got %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, waitErr := lb.Wait(ctx)
	if waitErr == nil {
		t.Fatal("expected non-nil error from Wait when callback missing state")
	}
	if !strings.Contains(waitErr.Error(), "missing code or state") {
		t.Errorf("error %q should mention missing code or state", waitErr.Error())
	}
}

func TestLoopback_WaitContextCancelled(t *testing.T) {
	t.Parallel()

	lb, err := startLoopback()
	if err != nil {
		t.Fatalf("startLoopback: %v", err)
	}
	defer lb.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, waitErr := lb.Wait(ctx)
	if waitErr == nil {
		t.Fatal("expected context error from Wait when ctx is cancelled")
	}
	if waitErr != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", waitErr)
	}
}

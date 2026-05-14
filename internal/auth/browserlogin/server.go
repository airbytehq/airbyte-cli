package browserlogin

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"sync"
	"time"
)

// callbackResult carries the outcome of the single OAuth redirect we expect
// the loopback server to receive. Exactly one of (Code+State) or Err is set.
type callbackResult struct {
	Code  string
	State string
	Err   error
}

// loopback is a one-shot HTTP server bound to 127.0.0.1 on an ephemeral port.
// It accepts a single /callback request, decodes the OAuth response into a
// callbackResult, and signals waiters via the result channel. Subsequent
// requests after the first delivery are still served the success page but
// do not overwrite the captured result.
type loopback struct {
	// Port is the ephemeral port chosen by the kernel; the OAuth redirect_uri
	// embedded in the authorize URL must reference this port.
	Port int

	server *http.Server
	result chan callbackResult
	once   sync.Once
}

// startLoopback binds 127.0.0.1:0 and starts serving the /callback handler in
// a background goroutine. The caller is responsible for calling Shutdown when
// the flow is complete (whether successful or not).
//
// Binding only to 127.0.0.1 (never 0.0.0.0) is required by RFC 8252 §7.3: the
// authorization code must not be exposed to other hosts on the network.
func startLoopback() (*loopback, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("loopback listen: %w", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return nil, fmt.Errorf("loopback listener returned unexpected addr type %T", ln.Addr())
	}

	lb := &loopback{
		Port:   addr.Port,
		result: make(chan callbackResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, lb.handleCallback)

	lb.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		// Serve returns http.ErrServerClosed once Shutdown is invoked — that's
		// the expected exit, so we drop it silently.
		_ = lb.server.Serve(ln)
	}()

	return lb, nil
}

// handleCallback parses the OAuth response query string and delivers a
// callbackResult exactly once. The HTTP response is a minimal self-contained
// HTML page — no external assets — so it renders even with strict CSP, ad
// blockers, or air-gapped browsers.
func (l *loopback) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	if errStr := q.Get("error"); errStr != "" {
		desc := q.Get("error_description")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w,
			"<!doctype html><html><body><h1>Login failed</h1><p>%s: %s</p><p>You can close this window and return to the terminal.</p></body></html>",
			html.EscapeString(errStr), html.EscapeString(desc),
		)
		l.deliver(callbackResult{Err: fmt.Errorf("oauth error: %s: %s", errStr, desc)})
		return
	}

	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w,
			"<!doctype html><html><body><h1>Login failed</h1><p>Missing code or state in callback.</p><p>You can close this window and return to the terminal.</p></body></html>",
		)
		l.deliver(callbackResult{Err: errors.New("callback missing code or state")})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w,
		"<!doctype html><html><body><h1>Login successful</h1><p>You can close this window and return to the terminal.</p></body></html>",
	)
	l.deliver(callbackResult{Code: code, State: state})
}

// deliver pushes a result onto the channel exactly once. Later callbacks (the
// browser sometimes retries) are silently ignored so we never block the
// handler goroutine.
func (l *loopback) deliver(r callbackResult) {
	l.once.Do(func() {
		l.result <- r
	})
}

// Wait blocks until the callback arrives or ctx is cancelled. The caller is
// responsible for invoking Shutdown afterwards (typically via defer at the
// call site that obtained the loopback).
func (l *loopback) Wait(ctx context.Context) (string, string, error) {
	select {
	case r := <-l.result:
		return r.Code, r.State, r.Err
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
}

// Shutdown stops the loopback server. It is safe to call multiple times.
func (l *loopback) Shutdown() {
	if l == nil || l.server == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = l.server.Shutdown(shutdownCtx)
}

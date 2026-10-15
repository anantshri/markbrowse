package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestServeShutsDownOnContextCancel: cancelling the context (what a signal
// does) stops the listener, and serve reports a clean exit rather than
// ErrServerClosed. Before this, the process was killed outright and any request
// in flight was dropped.
func TestServeShutsDownOnContextCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})}

	// Keep-alives off so the post-shutdown request below really has to open a
	// new connection; a pooled one would still be usable and prove nothing.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	url := "http://" + ln.Addr().String() + "/"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, ln) }()

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("request before shutdown: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status before shutdown = %d, want 200", resp.StatusCode)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve returned %v, want nil after a graceful shutdown", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not return after the context was cancelled")
	}

	if resp, err := client.Get(url); err == nil {
		_ = resp.Body.Close()
		t.Error("the server still accepts requests after shutdown")
	}
}

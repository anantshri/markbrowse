package main

import (
	"net"
	"testing"
)

func TestListenWithFallback(t *testing.T) {
	// Occupy a port so the fallback path is exercised.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	got, actual, err := listenWithFallback(port)
	if err != nil {
		t.Fatalf("listenWithFallback: %v", err)
	}
	defer got.Close()
	if actual == port {
		t.Errorf("fallback returned the busy port %d", port)
	}
	if actual < 10000 && actual <= port {
		t.Errorf("fallback port %d unexpected", actual)
	}
}

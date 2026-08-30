package main

import (
	"net"
	"testing"
)

func TestListenWithFallback(t *testing.T) {
	// Occupy a wildcard port with the same bind spec listenWithFallback
	// uses (":port", not "127.0.0.1:port") so the conflict is guaranteed on
	// every platform. An IPv4-loopback holder does not conflict with the
	// IPv6 dual-stack wildcard bind on macOS/BSD, which made the "busy"
	// port bindable and the test fail there.
	// nosemgrep: go.lang.security.audit.net.bind_all.avoid-bind-to-all-interfaces -- test-only transient holder on an ephemeral wildcard port, closed at test end; mirrors the production bind spec deliberately
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	if port >= 65535 {
		t.Skipf("ephemeral port %d leaves no room above it", port)
	}

	got, actual, err := listenWithFallback(port)
	if err != nil {
		t.Fatalf("listenWithFallback: %v", err)
	}
	defer got.Close()
	if actual <= port {
		t.Errorf("fallback returned busy-or-lower port %d (busy port was %d)", actual, port)
	}
	if actual < 10000 {
		t.Errorf("fallback port %d below the 10000 floor", actual)
	}
}

func TestListenWithFallbackPrefersRequestedPort(t *testing.T) {
	// Nothing holds a random wildcard port once we close its listener, so
	// asking for it again should succeed without fallback. (Tiny TOCTOU
	// race with the OS handing the port elsewhere; skip if lost.)
	// nosemgrep: go.lang.security.audit.net.bind_all.avoid-bind-to-all-interfaces -- test-only transient holder on an ephemeral wildcard port, closed immediately; mirrors the production bind spec deliberately
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	got, actual, err := listenWithFallback(port)
	if err != nil {
		t.Skipf("port %d got taken between close and rebind: %v", port, err)
	}
	defer got.Close()
	if actual != port {
		t.Errorf("listenWithFallback(free %d) = %d, want the requested port", port, actual)
	}
}

func TestListenWithFallbackErrorPath(t *testing.T) {
	// Sanity: the function reports an error rather than panicking when
	// handed an invalid port. With -1 the initial bind fails and the scan
	// starts at the 10000 floor — where a bind can legitimately succeed —
	// so assert on the documented behavior instead of assuming failure.
	l, actual, err := listenWithFallback(-1)
	if err != nil {
		return // some platforms reject before scanning
	}
	l.Close()
	if actual < 10000 {
		t.Errorf("fallback from invalid port landed on %d, below the 10000 floor", actual)
	}
}

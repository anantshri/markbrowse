package main

import (
	"net"
	"testing"
)

// The busy-port holder must bind the SAME host:port spec that
// listenWithFallback will try (see logs/2026-08-30-macos-ci-listen-test.md):
// on macOS/BSD an IPv4-loopback holder and a wildcard bind coexist as
// separate sockets, so a mismatched holder makes the "busy" port bindable
// and the test platform-dependent. listenWithFallback now defaults to
// 127.0.0.1, so the holders here use "127.0.0.1:0" to match.

func TestListenWithFallback(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	if port >= 65535 {
		t.Skipf("ephemeral port %d leaves no room above it", port)
	}

	got, actual, err := listenWithFallback(defaultListenHost, port)
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
	// Nothing holds a random port once we close its listener, so asking for
	// it again should succeed without fallback. (Tiny TOCTOU race with the
	// OS handing the port elsewhere; skip if lost.)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	got, actual, err := listenWithFallback(defaultListenHost, port)
	if err != nil {
		t.Skipf("port %d got taken between close and rebind: %v", port, err)
	}
	defer got.Close()
	if actual != port {
		t.Errorf("listenWithFallback(free %d) = %d, want the requested port", port, actual)
	}
}

func TestListenWithFallbackErrorPath(t *testing.T) {
	// Sanity: the function reports an error rather than panicking when handed
	// an invalid port. With -1 the initial bind fails and the scan starts at
	// the 10000 floor — where a bind can legitimately succeed — so assert on
	// the documented behavior instead of assuming failure.
	l, actual, err := listenWithFallback(defaultListenHost, -1)
	if err != nil {
		return // some platforms reject before scanning
	}
	l.Close()
	if actual < 10000 {
		t.Errorf("fallback from invalid port landed on %d, below the 10000 floor", actual)
	}
}

func TestDefaultListenHostIsLoopback(t *testing.T) {
	// Guards the security default: the server is unauthenticated, so it must
	// not bind a network-reachable address unless --listen says so
	// (secreports/report1.md finding 7).
	if defaultListenHost != "127.0.0.1" {
		t.Errorf("defaultListenHost = %q, want 127.0.0.1", defaultListenHost)
	}
}

func TestListenWithFallbackWildcardHost(t *testing.T) {
	// --listen 0.0.0.0 must actually reach the wildcard address (the flag
	// existing is not enough; it has to flow into the bind).
	// nosemgrep: go.lang.security.audit.net.bind_all.avoid-bind-to-all-interfaces -- test-only transient holder on an ephemeral port, closed at test end; verifies the operator opt-in path
	got, actual, err := listenWithFallback("0.0.0.0", 0)
	if err != nil {
		t.Skipf("cannot bind wildcard: %v", err)
	}
	defer got.Close()
	if actual < 10000 && actual != 0 {
		// port 0 asks the OS for an ephemeral port; any value is fine
		t.Logf("bound port %d", actual)
	}
	if ip := got.Addr().(*net.TCPAddr).IP; !ip.IsUnspecified() {
		t.Errorf("wildcard bind got %v, want an unspecified address", ip)
	}
}

func TestListenWithFallbackIPv6Loopback(t *testing.T) {
	// JoinHostPort must bracket IPv6 literals ("::1" -> "[::1]:port");
	// Sprintf("%s:%d") would produce an invalid spec.
	got, _, err := listenWithFallback("::1", 0)
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer got.Close()
	if got.Addr().(*net.TCPAddr).IP.String() != "::1" {
		t.Errorf("bind got %v, want ::1", got.Addr())
	}
}

func TestDisplayHost(t *testing.T) {
	tests := map[string]string{
		"127.0.0.1":   "127.0.0.1",
		"localhost":   "localhost",
		"0.0.0.0":     "localhost",
		"::":          "localhost",
		"":            "localhost",
		"::1":         "::1",
		"192.168.1.5": "192.168.1.5",
	}
	for in, want := range tests {
		if got := displayHost(in); got != want {
			t.Errorf("displayHost(%q) = %q, want %q", in, got, want)
		}
	}
}

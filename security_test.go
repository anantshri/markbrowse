package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// securityVault builds a tree containing the file shapes that decide whether
// the response headers are right.
func securityVault(t *testing.T) *fileHandler {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"README.md":   "# Notes\n\nSee the [report](report.html).\n",
		"report.html": "<html><script>fetch('/__mdview/tree.json')</script></html>",
		"evil.svg":    `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"data.xml":    `<?xml version="1.0"?><root/>`,
		"noextension": "<html><script>alert(1)</script></html>",
		"pic.png":     "\x89PNG\r\n\x1a\n",
		"notes.txt":   "plain text",
		"archive.zip": "PK\x03\x04",
		"sub/deep.md": "# Deep",
	}
	for name, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}
}

func get(t *testing.T, h *fileHandler, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec
}

// TestBaseSecurityHeadersOnEveryResponse: nosniff is what stops a file with no
// extension whose contents begin with markup from being sniffed to text/html
// and executed, so it has to be on everything, errors included.
func TestBaseSecurityHeadersOnEveryResponse(t *testing.T) {
	h := securityVault(t)

	for _, url := range []string{
		"/README.md",           // rendered markdown
		"/",                    // directory index
		"/sub/",                // nested index-less listing
		"/pic.png",             // raw file
		"/noextension",         // sniffed file
		"/__mdview/sidebar.js", // embedded asset
		"/__mdview/tree.json",  // tree API
		"/nope.md",             // 404
		"/.git/config",         // blocked path
	} {
		t.Run(url, func(t *testing.T) {
			rec := get(t, h, url)
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff (status %d)", got, rec.Code)
			}
			if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
				t.Errorf("Referrer-Policy = %q, want no-referrer", got)
			}
		})
	}
}

// TestExecutableFilesAreSandboxed is the fix for the headline finding: a
// .html or .svg beside the markdown would otherwise run in the viewer's
// origin, with read access to the whole served tree, defeating the point of
// omitting raw HTML from markdown.
func TestExecutableFilesAreSandboxed(t *testing.T) {
	h := securityVault(t)

	for _, tc := range []struct {
		url           string
		wantSandboxed bool
	}{
		{"/report.html", true},
		{"/evil.svg", true},
		{"/data.xml", true},    // XSLT can script
		{"/noextension", true}, // sniffs to text/html
		// Non-executable types must be untouched, or images and downloads break.
		{"/pic.png", false},
		{"/notes.txt", false},
		{"/archive.zip", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			rec := get(t, h, tc.url)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200", tc.url, rec.Code)
			}
			csp := rec.Header().Get("Content-Security-Policy")
			if tc.wantSandboxed && csp != "sandbox" {
				t.Errorf("%s (%s) has CSP %q, want sandbox",
					tc.url, rec.Header().Get("Content-Type"), csp)
			}
			if !tc.wantSandboxed && csp != "" {
				t.Errorf("%s (%s) got CSP %q, want none",
					tc.url, rec.Header().Get("Content-Type"), csp)
			}
		})
	}
}

// TestContentTypeIsDecidedNotSniffed: the type defended against has to be the
// type sent, so it is resolved before the response is written rather than left
// to ServeContent.
func TestContentTypeIsDecidedNotSniffed(t *testing.T) {
	h := securityVault(t)

	for url, wantPrefix := range map[string]string{
		"/pic.png":     "image/png",
		"/evil.svg":    "image/svg+xml",
		"/report.html": "text/html",
		"/noextension": "text/html",
	} {
		rec := get(t, h, url)
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, wantPrefix) {
			t.Errorf("%s Content-Type = %q, want prefix %q", url, got, wantPrefix)
		}
	}
}

func TestIsExecutableType(t *testing.T) {
	for _, ctype := range []string{
		"text/html", "text/html; charset=utf-8", "TEXT/HTML",
		"image/svg+xml", "application/xhtml+xml", "application/xml", "text/xml",
	} {
		if !isExecutableType(ctype) {
			t.Errorf("isExecutableType(%q) = false, want true", ctype)
		}
	}
	for _, ctype := range []string{
		"image/png", "image/jpeg", "application/pdf", "text/plain",
		"application/zip", "application/octet-stream", "",
	} {
		if isExecutableType(ctype) {
			t.Errorf("isExecutableType(%q) = true, want false", ctype)
		}
	}
}

// TestPageCSPUsesAFreshNonce checks the policy is present, that the inline
// mermaid bootstrap carries the matching nonce (otherwise the policy silently
// breaks diagrams), and that the nonce is not reused between requests — a
// fixed nonce is no better than 'unsafe-inline'.
func TestPageCSPUsesAFreshNonce(t *testing.T) {
	h := securityVault(t)

	nonces := map[string]bool{}
	for i := 0; i < 5; i++ {
		rec := get(t, h, "/README.md")
		csp := rec.Header().Get("Content-Security-Policy")
		if csp == "" {
			t.Fatal("no Content-Security-Policy on a rendered page")
		}
		for _, want := range []string{
			"default-src 'none'",
			"script-src 'self' 'nonce-",
			"connect-src 'self'",
			"base-uri 'none'",
			"frame-ancestors 'none'",
		} {
			if !strings.Contains(csp, want) {
				t.Errorf("CSP missing %q:\n%s", want, csp)
			}
		}

		nonce := between(csp, "'nonce-", "'")
		if nonce == "" {
			t.Fatalf("no nonce in CSP: %s", csp)
		}
		if nonces[nonce] {
			t.Errorf("nonce %q reused across requests", nonce)
		}
		nonces[nonce] = true

		// The page has a mermaid diagram only if the markdown had one; this
		// fixture does not, so assert the wiring on a page that does.
		if got := between(rec.Body.String(), `<script nonce="`, `"`); got != "" && got != nonce {
			t.Errorf("inline script nonce %q does not match CSP nonce %q", got, nonce)
		}
	}
}

// TestMermaidScriptCarriesTheNonce is the one that would catch a CSP that
// silently stops diagrams rendering.
func TestMermaidScriptCarriesTheNonce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "d.md"),
		[]byte("# D\n\n```mermaid\ngraph TD;\n A-->B;\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	rec := get(t, h, "/d.md")
	body := rec.Body.String()
	if !strings.Contains(body, "mermaid.initialize") {
		t.Fatalf("no mermaid bootstrap on a page with a diagram:\n%s", body)
	}
	nonce := between(rec.Header().Get("Content-Security-Policy"), "'nonce-", "'")
	tag := between(body, `<script nonce="`, `"`)
	if nonce == "" || tag != nonce {
		t.Errorf("inline script nonce %q does not match CSP nonce %q", tag, nonce)
	}
}

// TestMarkdownSizeLimit: rendering is linear in file size and happens per
// request, so an enormous document is a cheap way to exhaust the process.
func TestMarkdownSizeLimit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.md"), []byte("# fine"), 0o644); err != nil {
		t.Fatal(err)
	}
	big := filepath.Join(dir, "big.md")
	if err := os.WriteFile(big, []byte("#"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(big, maxMarkdownBytes+1); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	if rec := get(t, h, "/big.md"); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized markdown = %d, want 413", rec.Code)
	}
	if rec := get(t, h, "/ok.md"); rec.Code != http.StatusOK {
		t.Errorf("normal markdown = %d, want 200", rec.Code)
	}
}

// TestNonceIsUnpredictable is a smoke check on the generator, not a statistical
// test: distinct values of a sane length across many calls.
func TestNonceIsUnpredictable(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		n, err := newNonce()
		if err != nil {
			t.Fatalf("newNonce: %v", err)
		}
		if len(n) < 16 {
			t.Fatalf("nonce %q is too short to be useful", n)
		}
		if seen[n] {
			t.Fatalf("newNonce repeated %q after %d calls", n, i)
		}
		seen[n] = true
	}
}

// between returns the text between the first open and the next close after it.
func between(s, open, close string) string {
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

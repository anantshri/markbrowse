package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The three tests below cover the upgrade-staleness fix. markbrowse serves its
// scripts from URLs that are fixed at compile time and its pages from an ETag
// derived only from the file on disk, so without a build identity a browser
// keeps running the previous build's JavaScript and showing the previous
// build's rendering until its caches happen to expire.

// TestServeAssetRevalidates pins both halves of the asset fix: assets are no
// longer served with a 24h max-age under a URL that never changes, and the
// ETag is the build identity so an upgrade invalidates the cached copy.
func TestServeAssetRevalidates(t *testing.T) {
	h := &fileHandler{root: t.TempDir()}

	req := httptest.NewRequest(http.MethodGet, "/__mdview/sidebar.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache: a max-age outlives an upgrade", cc)
	}
	etag := rec.Header().Get("ETag")
	if want := `"` + serverID + `"`; etag != want {
		t.Errorf("ETag = %q, want the build identity %q", etag, want)
	}
	if len(rec.Body.Bytes()) == 0 {
		t.Error("asset body is empty")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/__mdview/sidebar.js", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("conditional asset request = %d, want 304", rec2.Code)
	}
	if rec2.Body.Len() != 0 {
		t.Errorf("304 asset response should have empty body, got %d bytes", rec2.Body.Len())
	}

	// An ETag minted by a different build must not revalidate: this is the
	// case the fix exists for.
	req3 := httptest.NewRequest(http.MethodGet, "/__mdview/sidebar.js", nil)
	req3.Header.Set("If-None-Match", `"previous-build"`)
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Errorf("an ETag from another build revalidated: status = %d, want 200", rec3.Code)
	}
}

// TestMarkdownETagIncludesBuildIdentity pins the page half: the rendered page
// depends on the build's templates and stylesheet as much as on the file, so
// the ETag has to change when the binary does.
func TestMarkdownETagIncludesBuildIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Doc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/doc.md", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if !strings.HasPrefix(etag, `"`+serverID+`-`) {
		t.Errorf("ETag = %q, want it to lead with the build identity %q", etag, serverID)
	}
}

// TestPagesVersionScriptURLs pins the cache-busting half: every script URL the
// templates emit carries the build identity, so the URL a previous build
// cached is not a URL this build serves.
func TestPagesVersionScriptURLs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Doc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	// The markdown page and the directory listing are separate templates.
	for _, path := range []string{"/doc.md", "/"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
		body := rec.Body.String()
		for _, asset := range []string{"sidebar.js", "tablesort.js"} {
			want := "/__mdview/" + asset + "?v=" + serverID
			if !strings.Contains(body, want) {
				t.Errorf("%s: script URL is not versioned, want it to contain %q", path, want)
			}
		}
	}
}

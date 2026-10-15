package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fetchAsset requests an embedded asset with the given Accept-Encoding.
func fetchAsset(t *testing.T, path, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()

	h := &fileHandler{root: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200", path, rec.Code)
	}
	return rec
}

// TestMermaidBundleIsCompressed: the bundle is 5.4 MB of minified JavaScript and
// every page containing a diagram fetches all of it, so it is sent gzipped when
// the client says it can take it.
func TestMermaidBundleIsCompressed(t *testing.T) {
	rec := fetchAsset(t, "/__mdview/mermaid.js", "gzip, deflate, br")

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Errorf("Vary = %q, want it to include Accept-Encoding", got)
	}

	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("body is not valid gzip: %v", err)
	}
	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompressing body: %v", err)
	}
	if !bytes.Equal(got, mermaidJS) {
		t.Errorf("decompressed bundle differs from the embedded asset (%d vs %d bytes)", len(got), len(mermaidJS))
	}
	if len(rec.Body.Bytes()) >= len(mermaidJS) {
		t.Errorf("compressed body is %d bytes, not smaller than the %d byte asset",
			len(rec.Body.Bytes()), len(mermaidJS))
	}
}

// TestMermaidBundleUncompressedWithoutAcceptEncoding: compression is an offer,
// not an assumption.
func TestMermaidBundleUncompressedWithoutAcceptEncoding(t *testing.T) {
	rec := fetchAsset(t, "/__mdview/mermaid.js", "")

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want none without Accept-Encoding", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), mermaidJS) {
		t.Error("body differs from the embedded asset")
	}
}

// TestSmallAssetServedUncompressed: the small scripts sit below the threshold, so
// they are served as-is even when gzip is on offer.
func TestSmallAssetServedUncompressed(t *testing.T) {
	if len(sidebarJS) >= gzipMinBytes {
		t.Fatalf("sidebar.js is %d bytes, at or above the %d byte threshold; this test no longer covers the small-asset branch",
			len(sidebarJS), gzipMinBytes)
	}

	rec := fetchAsset(t, "/__mdview/sidebar.js", "gzip")

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want none for an asset below the threshold", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), sidebarJS) {
		t.Error("body differs from the embedded sidebar asset")
	}
}

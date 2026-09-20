package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{2621440, "2.5 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := formatSize(tt.input)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildBreadcrumbs(t *testing.T) {
	tests := []struct {
		path string
		want []breadcrumb
	}{
		{"/", nil},
		{"/README.md", nil},
		{"/docs/README.md", []breadcrumb{{Name: "docs", Path: "/docs/"}}},
		{"/a/b/c.md", []breadcrumb{
			{Name: "a", Path: "/a/"},
			{Name: "b", Path: "/a/b/"},
		}},
	}
	for _, tt := range tests {
		got := buildBreadcrumbs(tt.path)
		if len(got) != len(tt.want) {
			t.Errorf("buildBreadcrumbs(%q) = %v, want %v", tt.path, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("buildBreadcrumbs(%q)[%d] = %v, want %v", tt.path, i, got[i], tt.want[i])
			}
		}
	}
}

func TestBuildFileIndex(t *testing.T) {
	idx := buildFileIndex("testdata")
	if len(idx) == 0 {
		t.Fatal("buildFileIndex returned empty index")
	}

	if p, ok := idx["readme"]; !ok || p != "/README.md" {
		t.Errorf("idx[readme] = %q, ok=%v, want /README.md", p, ok)
	}
	if p, ok := idx["notes"]; !ok || p != "/notes.md" {
		t.Errorf("idx[notes] = %q, ok=%v, want /notes.md", p, ok)
	}
}

func TestBuildFileIndexIncludesDotDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vault"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".vault", "secret.md"), []byte("# s"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := buildFileIndex(dir)
	if p, ok := idx["secret"]; !ok || p != "/.vault/secret.md" {
		t.Errorf("idx[secret] = %q, ok=%v, want /.vault/secret.md", p, ok)
	}
}

func TestPruneEmptyDirs(t *testing.T) {
	root := treeEntry{
		Name:  "root",
		Path:  "/",
		IsDir: true,
		Children: []treeEntry{
			{Name: "empty", Path: "/empty/", IsDir: true},
			{Name: "hasmd", Path: "/hasmd/", IsDir: true, Children: []treeEntry{
				{Name: "file.md", Path: "/hasmd/file.md"},
			}},
			{Name: "top.md", Path: "/top.md"},
		},
	}
	pruneEmptyDirs(&root)

	if len(root.Children) != 2 {
		t.Fatalf("pruneEmptyDirs: got %d children, want 2", len(root.Children))
	}
	for _, c := range root.Children {
		if c.Name == "empty" {
			t.Error("pruneEmptyDirs: empty directory should have been pruned")
		}
	}
}

func TestSortTree(t *testing.T) {
	root := treeEntry{
		Name:  "root",
		Path:  "/",
		IsDir: true,
		Children: []treeEntry{
			{Name: "zebra.md", Path: "/zebra.md"},
			{Name: "alpha", Path: "/alpha/", IsDir: true},
			{Name: "beta.md", Path: "/beta.md"},
		},
	}
	sortTree(&root)

	if len(root.Children) != 3 {
		t.Fatalf("sortTree: got %d children, want 3", len(root.Children))
	}
	if root.Children[0].Name != "alpha" {
		t.Errorf("sortTree: first child = %q, want alpha (dir first)", root.Children[0].Name)
	}
	if root.Children[1].Name != "beta.md" {
		t.Errorf("sortTree: second child = %q, want beta.md (alpha sort)", root.Children[1].Name)
	}
}

func TestInsertIntoTree(t *testing.T) {
	root := treeEntry{Name: "root", Path: "/", IsDir: true}
	insertIntoTree(&root, []string{"sub", "file.md"}, "/sub/file.md", false)

	if len(root.Children) != 1 {
		t.Fatalf("insertIntoTree: got %d children, want 1", len(root.Children))
	}
	sub := root.Children[0]
	if sub.Name != "sub" || !sub.IsDir {
		t.Errorf("insertIntoTree: sub = {%q, isDir=%v}, want {sub, true}", sub.Name, sub.IsDir)
	}
	if len(sub.Children) != 1 || sub.Children[0].Name != "file.md" {
		t.Errorf("insertIntoTree: sub children = %v, want [file.md]", sub.Children)
	}
}

func TestServeTreeIncludesDotDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden", "secret.md"), []byte("# s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.md"), []byte("# t"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A dot directory with no markdown must NOT appear (pruned as empty).
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// .git with markdown inside must be skipped outright (VCS dirs are never
	// listed), not just pruned-as-empty — this is what exercises the SkipDir.
	if err := os.WriteFile(filepath.Join(dir, ".git", "hook.md"), []byte("# hook"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := &fileHandler{root: dir}
	req := httptest.NewRequest(http.MethodGet, "/__mdview/tree.json", nil)
	rec := httptest.NewRecorder()
	h.serveTreeJSON(rec, req)

	var root treeEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &root); err != nil {
		t.Fatalf("unmarshal tree: %v", err)
	}
	found := false
	for _, c := range root.Children {
		if c.Name == ".hidden" {
			found = true
			if !c.IsDir {
				t.Error(".hidden should be a directory")
			}
			if len(c.Children) != 1 || c.Children[0].Name != "secret.md" {
				t.Errorf(".hidden children = %v, want [secret.md]", c.Children)
			}
		}
		if c.Name == ".git" {
			t.Error(".git (no markdown) should have been pruned from the tree")
		}
	}
	if !found {
		t.Error("tree should include dot directory .hidden")
	}
}

func TestServeHTTPForbiddenOnUnreadableFile(t *testing.T) {
	// Unix permission bits don't model readability on Windows (chmod 000
	// only toggles the read-only attribute on files and is a no-op on
	// directories), so the EACCES path can't be simulated there.
	if runtime.GOOS == "windows" {
		t.Skip("cannot simulate unreadable files on windows")
	}
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.md")
	if err := os.WriteFile(secret, []byte("# s"), 0o000); err != nil {
		t.Fatal(err)
	}
	// Ensure cleanup can still remove the file.
	defer os.Chmod(secret, 0o644)

	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}
	req := httptest.NewRequest(http.MethodGet, "/secret.md", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("ServeHTTP unreadable file = %d, want 403", rec.Code)
	}
}

func TestTreeJSONCachedRebuildsAfterTTL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# a"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir}

	first, firstETag := h.treeJSONCached()
	if first == nil || !strings.Contains(string(first), "a.md") {
		t.Fatalf("first tree missing a.md: %s", first)
	}
	if firstETag == "" {
		t.Fatal("rebuilt tree should carry an ETag")
	}

	// A second call inside the TTL must return the cached bytes (no rebuild).
	again, againETag := h.treeJSONCached()
	if !bytes.Equal(first, again) {
		t.Fatal("cached tree should be identical within TTL")
	}
	if againETag != firstETag {
		t.Errorf("ETag changed without a rebuild: %s -> %s", firstETag, againETag)
	}

	// Add a file, then force expiry and confirm the tree is rebuilt.
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("# b"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.treeAt = time.Now().Add(-treeCacheTTL - time.Second)
	rebuilt, rebuiltETag := h.treeJSONCached()
	if !strings.Contains(string(rebuilt), "b.md") {
		t.Fatalf("rebuilt tree missing b.md: %s", rebuilt)
	}
	// A changed tree must change the ETag, or browsers keep a stale sidebar.
	if rebuiltETag == firstETag {
		t.Errorf("ETag %s unchanged after the tree gained a file", rebuiltETag)
	}
}

// TestServeTreeJSONETagNotModified covers the request path: the sidebar
// refetches the tree on every page navigation, so an unchanged tree must come
// back as a bodiless 304 rather than as a few hundred KB of JSON.
func TestServeTreeJSONETagNotModified(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# a"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir}

	req := httptest.NewRequest(http.MethodGet, "/__mdview/tree.json", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the tree response")
	}
	if rec.Body.Len() == 0 {
		t.Fatal("first request returned an empty body")
	}

	req = httptest.NewRequest(http.MethodGet, "/__mdview/tree.json", nil)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("revalidation = %d, want 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("304 must have an empty body, got %d bytes", rec.Body.Len())
	}

	// A stale validator must still get the full tree.
	req = httptest.NewRequest(http.MethodGet, "/__mdview/tree.json", nil)
	req.Header.Set("If-None-Match", `"stale"`)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Errorf("stale validator = %d with %d bytes, want 200 with a body", rec.Code, rec.Body.Len())
	}
}

func TestServeMarkdownETagNotModified(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Doc\n\ncontent"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	req := httptest.NewRequest(http.MethodGet, "/doc.md", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("first response should carry an ETag")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/doc.md", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("conditional request = %d, want 304", rec2.Code)
	}
	if rec2.Body.Len() != 0 {
		t.Errorf("304 response should have empty body, got %d bytes", rec2.Body.Len())
	}
}

func TestTableSortAssetServed(t *testing.T) {
	h := &fileHandler{root: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/__mdview/tablesort.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("tablesort.js status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/javascript" {
		t.Errorf("tablesort.js content-type = %q, want application/javascript", ct)
	}
	if len(rec.Body.Bytes()) == 0 {
		t.Error("tablesort.js body is empty")
	}
}

func TestTocAssetServed(t *testing.T) {
	h := &fileHandler{root: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/__mdview/toc.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("toc.js status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/javascript" {
		t.Errorf("toc.js content-type = %q, want application/javascript", ct)
	}
	if len(rec.Body.Bytes()) == 0 {
		t.Error("toc.js body is empty")
	}
}

func TestTemplatesIncludeClientScripts(t *testing.T) {
	var md, dir strings.Builder
	if err := mdTmpl.Execute(&md, pageData{}); err != nil {
		t.Fatalf("mdTmpl execute: %v", err)
	}
	if err := dirTmpl.Execute(&dir, dirData{}); err != nil {
		t.Fatalf("dirTmpl execute: %v", err)
	}
	for name, out := range map[string]string{"mdTmpl": md.String(), "dirTmpl": dir.String()} {
		if !strings.Contains(out, "/__mdview/tablesort.js") {
			t.Errorf("%s should include tablesort.js script", name)
		}
	}
	// TOC only makes sense on markdown pages.
	if !strings.Contains(md.String(), "/__mdview/toc.js") {
		t.Error("mdTmpl should include toc.js script")
	}
	if !strings.Contains(md.String(), `id="toc-root"`) {
		t.Error("mdTmpl should include the toc-root nav element")
	}
	if !strings.Contains(md.String(), `id="toc-toggle"`) {
		t.Error("mdTmpl should include the toc-toggle button")
	}
	if strings.Contains(dir.String(), "/__mdview/toc.js") {
		t.Error("dirTmpl should not include toc.js script")
	}
	if strings.Contains(dir.String(), `id="toc-toggle"`) {
		t.Error("dirTmpl should not include the toc-toggle button")
	}
}

// TestCurrentPathNamesTheRenderedFile covers what the sidebar matches against.
// A directory that has an index serves a file whose URL is not the request
// URL, and the sidebar only has a node for the file — so reporting the
// directory path left the tree collapsed with nothing highlighted, while
// clicking the same file in the sidebar (which navigates to the file's own
// URL) worked.
func TestCurrentPathNamesTheRenderedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "guides", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		"README.md":             "# Root",
		"guides/README.md":      "# Guides",
		"guides/intro.md":       "# Intro",
		"guides/deep/INDEX.md":  "# Deep",
		"guides/deep/buried.md": "# Buried",
	} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(path)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A directory with no index at all still needs a sane value.
	if err := os.MkdirAll(filepath.Join(dir, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain", "note.md"), []byte("# Note"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	for _, tc := range []struct {
		name string
		url  string
		want string
	}{
		// Directory indexes: the value must name the file on screen, not the
		// directory that was requested.
		{"root index", "/", `/README.md`},
		{"nested index", "/guides/", `/guides/README.md`},
		{"INDEX.md index", "/guides/deep/", `/guides/deep/INDEX.md`},

		// Files requested directly were already correct; keep them that way.
		{"file direct", "/guides/intro.md", `/guides/intro.md`},
		{"index file direct", "/guides/README.md", `/guides/README.md`},

		// A directory with no index renders a listing. There is no file to
		// point at, so it names itself with a trailing slash, which is what
		// lets the sidebar open that folder — and the root must not become
		// the "//" protocol-relative form.
		{"listing", "/plain/", `/plain/`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200", tc.url, rec.Code)
			}
			want := `data-current-path="` + tc.want + `"`
			if !strings.Contains(rec.Body.String(), want) {
				got := "none"
				if i := strings.Index(rec.Body.String(), `data-current-path="`); i >= 0 {
					rest := rec.Body.String()[i:]
					got = rest[:strings.Index(rest[19:], `"`)+20]
				}
				t.Errorf("GET %s carried %s, want %s", tc.url, got, want)
			}
		})
	}
}

// TestMermaidBootstrapIsHardened pins the two mermaid.initialize options that
// are not cosmetic. securityLevel:"strict" is what keeps diagram labels from
// executing script; it is mermaid's default, but the default is exactly the
// kind of thing a major version changes, so it stays explicit and asserted.
// startOnLoad:false matters because mermaid.run() is called by hand once the
// markdown body is in place.
func TestMermaidBootstrapIsHardened(t *testing.T) {
	var md strings.Builder
	if err := mdTmpl.Execute(&md, pageData{HasMermaid: true}); err != nil {
		t.Fatalf("mdTmpl execute: %v", err)
	}
	out := md.String()

	if !strings.Contains(out, "/__mdview/mermaid.js") {
		t.Fatal("mdTmpl should load mermaid.js when the page has a diagram")
	}
	for _, want := range []string{`securityLevel:"strict"`, "startOnLoad:false", "mermaid.run()"} {
		if !strings.Contains(out, want) {
			t.Errorf("mermaid bootstrap missing %s:\n%s", want, out)
		}
	}

	// A page with no diagram must not pay for the 5 MB bundle.
	var plain strings.Builder
	if err := mdTmpl.Execute(&plain, pageData{}); err != nil {
		t.Fatalf("mdTmpl execute: %v", err)
	}
	if strings.Contains(plain.String(), "/__mdview/mermaid.js") {
		t.Error("mdTmpl should not load mermaid.js when the page has no diagram")
	}
}

// static/js/mermaid.min.js is vendored verbatim from the npm package. Nothing
// else in the repo tracks it: dependabot only reads go.mod and the workflows,
// and syft's SBOM only sees Go modules — which is how it sat on 11.15.0 while
// three minors and a major shipped. These constants are the record, and the
// test below is what makes a silent swap fail.
//
//	npm pack mermaid@12.0.0 && sha256sum package/dist/mermaid.min.js
const (
	mermaidVendoredVersion = "12.0.0"
	mermaidVendoredSHA256  = "28fca7ae6ebc7ed7bb63bde63136a74bfef14f296a57e403657eeb8b32836073"
)

// TestMermaidAssetIsTheVendoredBundle pins the embedded bundle to the exact
// npm artifact named above. Bumping mermaid is meant to fail this test: update
// both constants in the same commit as the file, so the version in the tree is
// always the version someone chose.
func TestMermaidAssetIsTheVendoredBundle(t *testing.T) {
	if got := fmt.Sprintf("%x", sha256.Sum256(mermaidJS)); got != mermaidVendoredSHA256 {
		t.Fatalf("embedded mermaid bundle sha256 = %s, want %s (mermaid %s)",
			got, mermaidVendoredSHA256, mermaidVendoredVersion)
	}

	js := string(mermaidJS)
	// The template loads the bundle as a classic script and then calls into a
	// global, so the global export and the securityLevel option both have to
	// survive whatever version is vendored.
	for _, want := range []string{
		`globalThis["mermaid"]`,
		"securityLevel",
		`version:"` + mermaidVendoredVersion + `"`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("mermaid bundle missing %q", want)
		}
	}
}

func TestServeDirectoryParentRowLink(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	// First-level directory: the parent row must link to the root, never to the
	// "//" protocol-relative URL that browsers resolve against another host.
	req := httptest.NewRequest(http.MethodGet, "/sub/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sub status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `<a href="/">../</a>`) {
		t.Errorf("parent row should link to /, got:\n%s", body)
	}

	// Deeper directory: the parent row keeps the trailing-slash path.
	req = httptest.NewRequest(http.MethodGet, "/sub/nested/", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("nested status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `<a href="/sub/">../</a>`) {
		t.Errorf("parent row should link to /sub/, got:\n%s", body)
	}
}

func TestServeDirectoryListingAndIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	// Plain listing: dirs first, parent row present, tablesort included.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("listing status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "sub/") || !strings.Contains(body, "b.md") {
		t.Errorf("listing missing entries:\n%s", body)
	}
	if !strings.Contains(body, "dir-list") {
		t.Error("listing should use the dir-list table")
	}

	// A directory containing README.md serves the rendered README instead.
	if err := os.WriteFile(filepath.Join(dir, "sub", "README.md"), []byte("# Sub Readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/sub/", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("sub status = %d, want 200", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "Sub Readme") {
		t.Errorf("sub should render README.md, got:\n%s", rec2.Body.String()[:200])
	}
}

func TestServeDirectoryForbiddenOnUnreadableDir(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	// See TestServeHTTPForbiddenOnUnreadableFile: chmod can't make a
	// directory unreadable on Windows.
	if runtime.GOOS == "windows" {
		t.Skip("cannot simulate unreadable directories on windows")
	}
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "x.md"), []byte("# x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Skipf("cannot chmod: %v", err)
	}
	defer os.Chmod(blocked, 0o755)

	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}
	req := httptest.NewRequest(http.MethodGet, "/blocked/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("unreadable dir status = %d, want 403", rec.Code)
	}
}

func TestServeMarkdownTitleFromFrontMatter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte("---\ntitle: Custom Title\n---\n\n# Heading"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	req := httptest.NewRequest(http.MethodGet, "/doc.md", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>Custom Title</title>") {
		t.Errorf("page title should come from front matter, got:\n%s", rec.Body.String()[:300])
	}
}

func TestServeMarkdownEscapesScriptTag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "evil.md"), []byte("# Evil\n\n<script>alert(document.cookie)</script>\n\n<img src=x onerror=\"alert(1)\">\n\n[click](javascript:alert(document.domain))\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/evil.md", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, needle := range []string{"<script>", "onerror=", `href="javascript:`} {
		if strings.Contains(body, needle) {
			t.Errorf("rendered page must not contain %q:\n%s", needle, body)
		}
	}
	if !strings.Contains(body, "<!-- raw HTML omitted -->") {
		// goldmark omits HTML *blocks* entirely (inline HTML keeps its text —
		// see TestInlineHTMLTextPreserved); the marker proves suppression came
		// from the renderer rather than an error page.
		t.Errorf("body should carry the raw-HTML-omitted marker, got:\n%s", body)
	}
}

// --- symlink containment + VCS blocking (secreports/report1.md findings 3+4) ---

func TestUnderRoot(t *testing.T) {
	sep := string(filepath.Separator)
	tests := []struct {
		root, p string
		want    bool
	}{
		{"/tmp/vault", "/tmp/vault", true},
		{"/tmp/vault", "/tmp/vault/notes.md", true},
		{"/tmp/vault", "/tmp/vault-x/notes.md", false}, // prefix boundary
		{"/tmp/vault", "/tmp/vaul", false},             // shorter sibling
		{"/tmp/vault", "/tmp", false},                  // parent
		{"/tmp/vault", "/etc/passwd", false},
		{"/", "/etc", true}, // root is "/"
		{"/", "/etc/passwd", true},
		{"/tmp/vault", "/tmp/vault" + sep + "sub" + sep + "x", true},
	}
	if runtime.GOOS == "windows" {
		// Backslash paths only parse as separators on Windows; on POSIX they
		// are ordinary characters and Rel degrades to a ".." result.
		tests = append(tests,
			struct {
				root, p string
				want    bool
			}{"C:\\vault", "C:\\vault\\x.md", true},
			struct {
				root, p string
				want    bool
			}{"C:\\vault", "C:\\vault-x\\x.md", false},
		)
	}
	for _, tt := range tests {
		if got := underRoot(tt.root, tt.p); got != tt.want {
			t.Errorf("underRoot(%q, %q) = %v, want %v", tt.root, tt.p, got, tt.want)
		}
	}
}

func TestIsVCSName(t *testing.T) {
	for _, name := range []string{".git", ".hg", ".svn", ".bzr"} {
		if !isVCSName(name) {
			t.Errorf("isVCSName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"git", ".gitignore", ".github", ".gitx", ".obsidian", ".hidden", "notes.md", ""} {
		if isVCSName(name) {
			t.Errorf("isVCSName(%q) = true, want false", name)
		}
	}
}

func TestHasVCSSegment(t *testing.T) {
	for _, p := range []string{
		"/.git/config", "/.git/", "/.hg/x", "/.svn/x", "/.bzr/x",
		"/foo/.git/config", "/a/b/.git/objects/pack/1.pack",
	} {
		if !hasVCSSegment(p) {
			t.Errorf("hasVCSSegment(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"/", "/notes.md", "/.gitignore", "/.github/workflows/ci.yml", "/.obsidian/x.md"} {
		if hasVCSSegment(p) {
			t.Errorf("hasVCSSegment(%q) = true, want false", p)
		}
	}
}

func TestServeHTTPRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("# secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "leak.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/leak.md", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("symlink escape = %d, want 404", rec.Code)
	}
}

func TestServeHTTPRejectsSymlinkDirEscape(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("/etc", filepath.Join(dir, "etcdir")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	for _, p := range []string{"/etcdir/", "/etcdir/hostname"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
	}
}

func TestServeHTTPAllowsSymlinkInsideRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.md"), []byte("# Real"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "real.md"), filepath.Join(dir, "alias.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/alias.md", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("internal symlink = %d, want 200", rec.Code)
	}
}

func TestServeHTTPRootItselfSymlinked(t *testing.T) {
	// Encodes the macOS case: temp dirs sit behind /var -> /private/var, so
	// the root handed to the handler is already "through" a symlink on disk.
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "doc.md"), []byte("# Doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	h := &fileHandler{root: link, md: newMarkdownConverter(link, false)}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/doc.md", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("doc behind symlinked root = %d, want 200", rec.Code)
	}
}

func TestServeDirectoryIndexSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("# secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "README.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	// Directory view falls through to the listing, not the escaped file.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("dir with hostile README.md = %d, want 200 (listing)", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "# secret") {
		t.Errorf("directory view leaked escaped index file:\n%s", rec.Body.String())
	}

	// Direct request to the escaped candidate is blocked too.
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/README.md", nil))
	if rec2.Code != http.StatusNotFound {
		t.Errorf("direct GET README.md (escaping symlink) = %d, want 404", rec2.Code)
	}
}

func TestServeHTTPNotFoundUnchanged(t *testing.T) {
	dir := t.TempDir()
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing.md", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing file = %d, want 404", rec.Code)
	}
}

func TestServeHTTPBlocksVCSDirs(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{".git", ".hg", ".svn", ".bzr"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sub, "config"), []byte("secret=abc123"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sub, "notes.md"), []byte("# vcs note"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", ".git", "config"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-VCS dot-dirs stay browsable.
	if err := os.MkdirAll(filepath.Join(dir, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".obsidian", "vault.md"), []byte("# Vault"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	for _, p := range []string{
		"/.git/config", "/.git/", "/.git/notes.md",
		"/.hg/config", "/.svn/config", "/.bzr/config",
		"/sub/.git/config",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
	}
	for _, p := range []string{"/.obsidian/vault.md", "/.gitignore"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 (non-VCS dot path)", p, rec.Code)
		}
	}
}

func TestServeHTTPBlocksVCSThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config.md"), []byte("# cfg"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Clean URL path, but the target sits inside .git.
	if err := os.Symlink(filepath.Join(dir, ".git", "config.md"), filepath.Join(dir, "notes.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir, false)}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/notes.md", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("symlink into .git = %d, want 404", rec.Code)
	}
}

func TestBuildFileIndexSkipsVCSDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "hook.md"), []byte("# hook"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".vault"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".vault", "x.md"), []byte("# x"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := buildFileIndex(dir)
	if _, ok := idx["hook"]; ok {
		t.Error(".git/hook.md must not be wikilink-indexed")
	}
	if p, ok := idx["x"]; !ok || p != "/.vault/x.md" {
		t.Errorf(".vault/x.md should be indexed at /.vault/x.md, got %q ok=%v", p, ok)
	}
}

func TestUnderRootRelError(t *testing.T) {
	// filepath.Rel errors when root and path mix absolute/relative forms;
	// underRoot must treat that as "not contained".
	if underRoot("vault", "/etc/passwd") {
		t.Error(`underRoot("vault", "/etc/passwd") = true, want false (Rel error)`)
	}
	if underRoot("/tmp/vault", "etc/passwd") {
		t.Error(`underRoot("/tmp/vault", "etc/passwd") = true, want false (Rel error)`)
	}
}

func TestResolvedRootDirFallsBackWhenRootMissing(t *testing.T) {
	// EvalSymlinks fails on a nonexistent root; the fallback keeps the
	// handler constructible (requests will 404 at os.Stat long before).
	h := &fileHandler{root: "/nonexistent/markbrowse/root"}
	if got := h.resolvedRootDir(); got != filepath.Clean("/nonexistent/markbrowse/root") {
		t.Errorf("resolvedRootDir fallback = %q, want cleaned root", got)
	}
}

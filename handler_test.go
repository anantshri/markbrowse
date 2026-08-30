package main

import (
	"bytes"
	"encoding/json"
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

	h := &fileHandler{root: dir, md: newMarkdownConverter(dir)}
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

	first := h.treeJSONCached()
	if first == nil || !strings.Contains(string(first), "a.md") {
		t.Fatalf("first tree missing a.md: %s", first)
	}

	// A second call inside the TTL must return the cached bytes (no rebuild).
	again := h.treeJSONCached()
	if !bytes.Equal(first, again) {
		t.Fatal("cached tree should be identical within TTL")
	}

	// Add a file, then force expiry and confirm the tree is rebuilt.
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("# b"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.treeAt = time.Now().Add(-treeCacheTTL - time.Second)
	rebuilt := h.treeJSONCached()
	if !strings.Contains(string(rebuilt), "b.md") {
		t.Fatalf("rebuilt tree missing b.md: %s", rebuilt)
	}
}

func TestServeMarkdownETagNotModified(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Doc\n\ncontent"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir)}

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

func TestServeDirectoryListingAndIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir)}

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

	h := &fileHandler{root: dir, md: newMarkdownConverter(dir)}
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
	h := &fileHandler{root: dir, md: newMarkdownConverter(dir)}

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

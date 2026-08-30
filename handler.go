package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type fileHandler struct {
	root      string
	md        *markdownConverter
	customCSS string

	treeMu   sync.Mutex
	treeJSON []byte
	treeAt   time.Time
}

func (h *fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	relPath := path.Clean("/" + r.URL.Path)
	fsPath := filepath.Join(h.root, relPath[1:])

	if !strings.HasPrefix(fsPath, h.root) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if relPath == "/__mdview/mermaid.js" {
		serveAsset(w, "application/javascript", mermaidJS)
		return
	}

	if relPath == "/__mdview/sidebar.js" {
		serveAsset(w, "application/javascript", sidebarJS)
		return
	}

	if relPath == "/__mdview/tablesort.js" {
		serveAsset(w, "application/javascript", tablesortJS)
		return
	}

	if relPath == "/__mdview/toc.js" {
		serveAsset(w, "application/javascript", tocJS)
		return
	}

	if relPath == "/__mdview/tree.json" {
		h.serveTreeJSON(w, r)
		return
	}

	// #nosec G304 G703 -- fsPath is joined from h.root and the path.Clean'd request path, then prefix-checked against h.root above
	info, err := os.Stat(fsPath)
	if err != nil {
		if os.IsPermission(err) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if info.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			// #nosec G710 -- relPath is path.Clean'd, always starts with /
			// nosemgrep: go.lang.security.injection.open-redirect.open-redirect -- same: relPath is server-side path.Clean'd, never a full URL
			http.Redirect(w, r, relPath+"/", http.StatusMovedPermanently)
			return
		}
		h.serveDirectory(w, r, fsPath, relPath)
		return
	}

	if strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
		h.serveMarkdown(w, r, fsPath, relPath)
		return
	}

	// #nosec G304 G703 -- fsPath validated against h.root above (HasPrefix check on the joined path)
	f, err := os.Open(fsPath)
	if err != nil {
		if os.IsPermission(err) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func (h *fileHandler) serveDirectory(w http.ResponseWriter, r *http.Request, fsPath, relPath string) {
	for _, name := range []string{"README.md", "readme.md", "INDEX.md", "index.md"} {
		indexPath := filepath.Join(fsPath, name)
		// #nosec G304 G703 -- fixed candidate name joined onto fsPath already validated in ServeHTTP
		if info, err := os.Stat(indexPath); err == nil && !info.IsDir() {
			h.serveMarkdown(w, r, indexPath, relPath)
			return
		}
	}

	// #nosec G304 G703 -- fsPath validated against h.root in ServeHTTP
	entries, err := os.ReadDir(fsPath)
	if err != nil {
		if os.IsPermission(err) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	var dirEntries []entryInfo
	for _, e := range entries {
		name := e.Name()
		url := path.Join(relPath, name)
		if e.IsDir() {
			url += "/"
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		dirEntries = append(dirEntries, entryInfo{
			Name:    name,
			URL:     url,
			IsDir:   e.IsDir(),
			Size:    formatSize(info.Size()),
			ModTime: info.ModTime().Format("2006-01-02 15:04"),
		})
	}

	data := dirData{
		Path:        relPath,
		CSS:         h.css(),
		HasParent:   relPath != "/",
		ParentPath:  path.Dir(relPath) + "/",
		Entries:     dirEntries,
		CurrentPath: relPath + "/",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dirTmpl.Execute(w, data); err != nil {
		log.Printf("dir template error: %v", err)
	}
}

func (h *fileHandler) serveMarkdown(w http.ResponseWriter, r *http.Request, fsPath, relPath string) {
	// #nosec G304 G703 -- fsPath validated against h.root in ServeHTTP (or built from fixed index names in serveDirectory)
	info, err := os.Stat(fsPath)
	if err != nil {
		if os.IsPermission(err) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// ETag from file identity lets browsers revalidate cheaply and lets us
	// short-circuit with 304 without re-reading/re-rendering unchanged files.
	etag := fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size())
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// #nosec G304 G703 -- fsPath validated against h.root in ServeHTTP before dispatch
	source, err := os.ReadFile(fsPath)
	if err != nil {
		if os.IsPermission(err) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	body, err := h.md.convert(source)
	if err != nil {
		http.Error(w, "markdown conversion error", http.StatusInternalServerError)
		return
	}

	title := filepath.Base(fsPath)
	// Front matter title wins, else the first <h1>, else the filename.
	if t := h.md.metaTitleOf(source); t != "" {
		title = t
	} else if strings.HasPrefix(body, "<h1") {
		start := strings.Index(body, ">")
		end := strings.Index(body, "</h1>")
		if start != -1 && end != -1 && end > start {
			title = strings.TrimSpace(body[start+1 : end])
		}
	}

	data := pageData{
		Title: title,
		CSS:   h.css(),
		// #nosec G203 -- goldmark output with raw HTML omitted and dangerous
		// URLs filtered (unless --raw-html opts back in); template.HTML is
		// still required so goldmark's own tags (<table>, <pre class="mermaid">,
		// alert divs) render instead of printing as source.
		// nosemgrep: go.lang.security.audit.xss.template-html-does-not-escape.unsafe-template-type -- markdown body rendered by goldmark with WithUnsafe off by default (raw HTML omitted, javascript:/data: URLs filtered); --raw-html is an explicit operator opt-in
		Body:        template.HTML(body),
		Breadcrumbs: buildBreadcrumbs(relPath),
		HasMermaid:  strings.Contains(body, `class="mermaid"`),
		CurrentPath: relPath,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("ETag", etag)
	if err := mdTmpl.Execute(w, data); err != nil {
		log.Printf("md template error: %v", err)
	}
}

func (h *fileHandler) css() template.CSS {
	if h.customCSS != "" {
		// #nosec G203 -- user-supplied CSS is intentional
		// nosemgrep: go.lang.security.audit.xss.template-html-does-not-escape.unsafe-template-type -- operator-provided -css flag, intentional
		return template.CSS(h.customCSS)
	}
	// #nosec G203 -- hardcoded CSS is safe
	// nosemgrep: go.lang.security.audit.xss.template-html-does-not-escape.unsafe-template-type -- compile-time constant
	return template.CSS(defaultCSS)
}

func buildBreadcrumbs(relPath string) []breadcrumb {
	parts := strings.Split(strings.Trim(relPath, "/"), "/")
	var crumbs []breadcrumb
	accumulated := ""
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "" {
			continue
		}
		accumulated += "/" + parts[i]
		crumbs = append(crumbs, breadcrumb{Name: parts[i], Path: accumulated + "/"})
	}
	return crumbs
}

func formatSize(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

type treeEntry struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"isDir,omitempty"`
	Children []treeEntry `json:"children,omitempty"`
}

// treeCacheTTL bounds how long the sidebar tree stays cached. The tree can
// change as files are added/removed on disk, so it is rebuilt after this
// window instead of holding a stale snapshot forever.
const treeCacheTTL = 5 * time.Second

// treeJSONCached returns the serialized file tree, rebuilding it at most
// once per treeCacheTTL. The walk, prune and sort are the most expensive
// work the server does on every page load, so caching avoids repeating it
// for rapid successive requests (e.g. the sidebar fetching on each page).
func (h *fileHandler) treeJSONCached() []byte {
	h.treeMu.Lock()
	defer h.treeMu.Unlock()
	if h.treeJSON != nil && time.Since(h.treeAt) < treeCacheTTL {
		return h.treeJSON
	}

	root := treeEntry{
		Name:  filepath.Base(h.root),
		Path:  "/",
		IsDir: true,
	}
	if err := filepath.WalkDir(h.root, func(walkPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Dot directories are walked like any other so they appear in the
		// sidebar (e.g. .obsidian vaults); directories with no markdown
		// anywhere beneath them are pruned later by pruneEmptyDirs.
		if !d.IsDir() && !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(h.root, walkPath)
		if err != nil {
			return nil
		}
		urlPath := "/" + filepath.ToSlash(rel)
		if d.IsDir() {
			urlPath += "/"
		}
		segments := strings.Split(filepath.ToSlash(rel), "/")
		insertIntoTree(&root, segments, urlPath, d.IsDir())
		return nil
	}); err != nil {
		log.Printf("tree walk error: %v", err)
	}
	pruneEmptyDirs(&root)
	sortTree(&root)

	data, err := json.Marshal(root)
	if err != nil {
		log.Printf("tree json encode error: %v", err)
		return nil
	}
	h.treeJSON = data
	h.treeAt = time.Now()
	return data
}

func (h *fileHandler) serveTreeJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	data := h.treeJSONCached()
	if data == nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- JSON payload of server-walked filenames, serialized by encoding/json
	if _, err := w.Write(data); err != nil {
		log.Printf("tree json write error: %v", err)
	}
}

// serveAsset writes an embedded static asset. These are go:embed'd files
// compiled into the binary, not user input.
//
// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- assets are build-time embedded, never request-derived
func serveAsset(w http.ResponseWriter, contentType string, asset []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- asset is compile-time embedded bytes
	_, _ = w.Write(asset)
}

func insertIntoTree(root *treeEntry, segments []string, urlPath string, isDir bool) {
	current := root
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		found := false
		for j := range current.Children {
			if current.Children[j].Name == seg {
				current = &current.Children[j]
				found = true
				break
			}
		}
		if !found {
			childPath := "/" + strings.Join(segments[:i+1], "/")
			if isDir || i < len(segments)-1 {
				childPath += "/"
			}
			child := treeEntry{
				Name:  seg,
				Path:  childPath,
				IsDir: isDir || i < len(segments)-1,
			}
			current.Children = append(current.Children, child)
			current = &current.Children[len(current.Children)-1]
		}
	}
}

func pruneEmptyDirs(node *treeEntry) {
	var kept []treeEntry
	for i := range node.Children {
		if node.Children[i].IsDir {
			pruneEmptyDirs(&node.Children[i])
			if len(node.Children[i].Children) == 0 {
				continue
			}
		}
		kept = append(kept, node.Children[i])
	}
	node.Children = kept
}

func sortTree(node *treeEntry) {
	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].IsDir != node.Children[j].IsDir {
			return node.Children[i].IsDir
		}
		return node.Children[i].Name < node.Children[j].Name
	})
	for i := range node.Children {
		sortTree(&node.Children[i])
	}
}

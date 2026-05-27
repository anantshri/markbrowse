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
)

type fileHandler struct {
	root      string
	md        *markdownConverter
	customCSS string
}

func (h *fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	relPath := path.Clean("/" + r.URL.Path)
	fsPath := filepath.Join(h.root, relPath[1:])

	if !strings.HasPrefix(fsPath, h.root) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if relPath == "/__mdview/mermaid.js" {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(mermaidJS)
		return
	}

	if relPath == "/__mdview/sidebar.js" {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(sidebarJS)
		return
	}

	if relPath == "/__mdview/tree.json" {
		h.serveTreeJSON(w, r)
		return
	}

	info, err := os.Stat(fsPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if info.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, relPath+"/", http.StatusMovedPermanently) // #nosec G710 -- relPath is path.Clean'd, always starts with /
			return
		}
		h.serveDirectory(w, r, fsPath, relPath)
		return
	}

	if strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
		h.serveMarkdown(w, r, fsPath, relPath)
		return
	}

	f, err := os.Open(fsPath) // #nosec G304 -- fsPath validated against h.root above
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func (h *fileHandler) serveDirectory(w http.ResponseWriter, r *http.Request, fsPath, relPath string) {
	for _, name := range []string{"README.md", "readme.md", "INDEX.md", "index.md"} {
		indexPath := filepath.Join(fsPath, name)
		if info, err := os.Stat(indexPath); err == nil && !info.IsDir() {
			h.serveMarkdown(w, r, indexPath, relPath)
			return
		}
	}

	entries, err := os.ReadDir(fsPath)
	if err != nil {
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
	source, err := os.ReadFile(fsPath) // #nosec G304 -- fsPath validated against h.root in ServeHTTP
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	body, err := h.md.convert(source)
	if err != nil {
		http.Error(w, "markdown conversion error", http.StatusInternalServerError)
		return
	}

	title := filepath.Base(fsPath)
	if strings.HasPrefix(body, "<h1") {
		start := strings.Index(body, ">")
		end := strings.Index(body, "</h1>")
		if start != -1 && end != -1 && end > start {
			title = strings.TrimSpace(body[start+1 : end])
		}
	}

	data := pageData{
		Title:       title,
		CSS:         h.css(),
		Body:        template.HTML(body), // #nosec G203 -- goldmark output is trusted
		Breadcrumbs: buildBreadcrumbs(relPath),
		HasMermaid:  strings.Contains(body, `class="mermaid"`),
		CurrentPath: relPath,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := mdTmpl.Execute(w, data); err != nil {
		log.Printf("md template error: %v", err)
	}
}

func (h *fileHandler) css() template.CSS {
	if h.customCSS != "" {
		return template.CSS(h.customCSS) // #nosec G203 -- user-supplied CSS is intentional
	}
	return template.CSS(defaultCSS) // #nosec G203 -- hardcoded CSS is safe
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

func (h *fileHandler) serveTreeJSON(w http.ResponseWriter, _ *http.Request) {
	root := treeEntry{
		Name:  filepath.Base(h.root),
		Path:  "/",
		IsDir: true,
	}
	if err := filepath.WalkDir(h.root, func(walkPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(root); err != nil {
		log.Printf("tree json encode error: %v", err)
	}
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

package main

import (
	"fmt"
	"html/template"
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
		w.Write(mermaidJS)
		return
	}

	info, err := os.Stat(fsPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if info.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
		h.serveDirectory(w, r, fsPath, relPath)
		return
	}

	if strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
		h.serveMarkdown(w, r, fsPath, relPath)
		return
	}

	f, err := os.Open(fsPath)
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
		Path:       relPath,
		CSS:        h.css(),
		HasParent:  relPath != "/",
		ParentPath: path.Dir(relPath) + "/",
		Entries:    dirEntries,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	dirTmpl.Execute(w, data)
}

func (h *fileHandler) serveMarkdown(w http.ResponseWriter, r *http.Request, fsPath, relPath string) {
	source, err := os.ReadFile(fsPath)
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
		Body:        template.HTML(body),
		Breadcrumbs: buildBreadcrumbs(relPath),
		HasMermaid:  strings.Contains(body, `class="mermaid"`),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	mdTmpl.Execute(w, data)
}

func (h *fileHandler) css() template.CSS {
	if h.customCSS != "" {
		return template.CSS(h.customCSS)
	}
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

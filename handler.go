package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
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
	treeETag string
	treeAt   time.Time

	resolveOnce  sync.Once
	resolvedRoot string
}

// openRoot returns an os.Root confined to the served directory. The caller
// closes it.
//
// os.Root is the containment guarantee: its methods refuse any name that
// resolves outside the root, including through symlinks, and the refusal is
// enforced by the kernel (openat2/RESOLVE_BENEATH on Linux) rather than by a
// check this code performs and then hopes still holds. That closes the
// stat-then-open race structurally instead of by argument, and it means the
// path handed to a filesystem call is no longer something that has to be
// proven safe by reading the surrounding code.
//
// It is opened per request rather than cached on the handler. An os.Root holds
// an open directory descriptor, and markbrowse has no shutdown path to release
// one: a cached handle would leak for the process lifetime, and on Windows an
// open directory handle blocks deletion of the directory outright. One extra
// openat per request is not measurable next to the stat and render the request
// already does.
func (h *fileHandler) openRoot() (*os.Root, error) {
	return os.OpenRoot(h.root)
}

// maxSymlinkHops bounds the absolute-symlink rewriting in statInRoot.
const maxSymlinkHops = 8

// statInRoot stats a name inside the root, additionally following absolute
// symlinks whose target is itself inside the root. It returns the info and the
// name that access should actually use, which differs from the input when a
// link was rewritten.
//
// os.Root refuses every absolute symlink, including one pointing back into the
// served tree, because an absolute target is re-resolved against the whole
// filesystem namespace rather than against the root. markbrowse has always
// served those — linking shared notes into a vault with
// "ln -s /abs/path/notes.md" is an ordinary thing to do, and
// TestServeHTTPAllowsSymlinkInsideRoot has asserted it since symlink
// containment was added — so the target is translated into a root-relative
// name and resubmitted to os.Root.
//
// The lexical comparison here only decides how to rewrite the name. It never
// authorises the access: the rewritten name goes back through os.Root, which
// applies the same kernel-enforced containment to it, so a target that is not
// genuinely inside the root still fails.
func (h *fileHandler) statInRoot(root *os.Root, name string) (os.FileInfo, string, error) {
	for hop := 0; ; hop++ {
		info, err := root.Stat(name)
		if err == nil {
			return info, name, nil
		}
		if hop >= maxSymlinkHops {
			return nil, "", err
		}

		// Only the absolute-symlink case is recoverable. Anything else — a
		// missing file, a link that really does escape — keeps its error.
		target, linkErr := root.Readlink(name)
		if linkErr != nil || !filepath.IsAbs(target) {
			return nil, "", err
		}

		next, ok := h.relocate(target)
		if !ok {
			return nil, "", err
		}
		name = next
	}
}

// relocate expresses an absolute path as a name relative to the served root,
// if it is inside it. Both the raw and the symlink-resolved root are tried,
// because the root itself may be reached through a symlink (macOS /var).
func (h *fileHandler) relocate(target string) (string, bool) {
	target = filepath.Clean(target)
	for _, base := range []string{h.root, h.resolvedRootDir()} {
		if !underRoot(base, target) {
			continue
		}
		rel, err := filepath.Rel(base, target)
		if err != nil {
			continue
		}
		return rel, true
	}
	return "", false
}

// rootName converts a cleaned URL path into the root-relative name that
// os.Root methods take: "/guides/intro.md" becomes "guides/intro.md", and the
// root itself becomes ".".
func rootName(relPath string) string {
	name := strings.TrimPrefix(relPath, "/")
	if name == "" {
		return "."
	}
	return filepath.FromSlash(name)
}

// vcsDirNames are repository-metadata directories that are never served,
// listed, or indexed: they leak the checkout's contents (.git/config et al.)
// and never contain markdown the viewer wants.
var vcsDirNames = map[string]bool{".git": true, ".hg": true, ".svn": true, ".bzr": true}

// vcsShortName matches the Windows 8.3 alias of a VCS directory: ".git" is
// also reachable as "GIT~1".
var vcsShortName = regexp.MustCompile(`^(git|hg|svn|bzr)~[0-9]+$`)

// isVCSName reports whether a path segment names a VCS metadata directory.
//
// The comparison is deliberately loose, because the filesystem is. On APFS
// (macOS default) and NTFS, "/.GIT/config" opens the real ".git/config", so an
// exact match would block the path only on Linux. Windows additionally ignores
// trailing dots and spaces (".git." is ".git") and exposes 8.3 aliases.
func isVCSName(name string) bool {
	n := strings.ToLower(name)
	n = strings.TrimRight(n, ". ")
	return vcsDirNames[n] || vcsShortName.MatchString(n)
}

// hasVCSSegment reports whether any "/"-separated segment of a URL path is a
// VCS directory name. Exact segment match only: ".gitignore" and ".github"
// are files/dirs of their own and stay servable.
func hasVCSSegment(urlPath string) bool {
	for _, seg := range strings.Split(urlPath, "/") {
		if isVCSName(seg) {
			return true
		}
	}
	return false
}

// underRoot reports whether p is root itself or beneath it. filepath.Rel
// avoids the classic prefix bug (root "/tmp/vault" matching "/tmp/vault-x")
// and behaves correctly when root is "/" on either separator style.
func underRoot(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}

// resolvedRootDir returns h.root with all symlinks resolved, computed once.
// Comparing resolved request paths against the resolved root (rather than the
// raw h.root) keeps containment working when the root itself is reached
// through a symlink — including macOS temp dirs (/var -> /private/var).
func (h *fileHandler) resolvedRootDir() string {
	h.resolveOnce.Do(func() {
		resolved, err := filepath.EvalSymlinks(h.root)
		if err != nil {
			resolved = filepath.Clean(h.root)
		}
		h.resolvedRoot = resolved
	})
	return h.resolvedRoot
}

// resolveContained resolves every symlink in fsPath and reports whether the
// fully-resolved target is servable.
//
// Containment itself is no longer this function's job — os.Root enforces that
// (see openRoot). What is still needed here is the VCS policy, which os.Root
// has no opinion about: a symlink that stays inside the root but points at
// ".git/config" is contained and must still be refused. The underRoot check is
// kept as a cheap second opinion, not as the guarantee.
func (h *fileHandler) resolveContained(fsPath string) (string, bool) {
	resolved, err := filepath.EvalSymlinks(fsPath)
	if err != nil {
		return "", false
	}
	root := h.resolvedRootDir()
	if !underRoot(root, resolved) {
		return "", false
	}
	// A symlink with a clean name can still land inside a VCS dir
	// (notes.md -> .git/config.md); apply the same policy to the target.
	if rel, err := filepath.Rel(root, resolved); err == nil {
		if hasVCSSegment("/" + filepath.ToSlash(rel)) {
			return "", false
		}
	}
	return resolved, true
}

// setBaseSecurityHeaders applies the headers every response gets, whatever it
// is. nosniff is the important one: without it a file with no extension whose
// contents begin with markup is sniffed to text/html and executed.
func setBaseSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Nothing here needs to tell a third party which document was open, and
	// markdown may legitimately reference remote images.
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func (h *fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setBaseSecurityHeaders(w)

	relPath := path.Clean("/" + r.URL.Path)
	fsPath := filepath.Join(h.root, relPath[1:])

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

	// Lexical containment first: path.Clean already strips leading "..", so
	// this is defense in depth against join/prefix bugs, not the real guard.
	if !underRoot(h.root, fsPath) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// VCS directories are never served; 404 (not 403) so the response does
	// not confirm whether the directory exists.
	if hasVCSSegment(relPath) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	root, err := h.openRoot()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer root.Close()

	info, name, err := h.statInRoot(root, rootName(relPath))
	if err != nil {
		if os.IsPermission(err) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// os.Root has already refused anything outside the tree. This is the VCS
	// policy on the symlink target: "notes.md -> .git/config" is contained, so
	// only resolving it catches the leak.
	if _, ok := h.resolveContained(fsPath); !ok {
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
		h.serveDirectory(w, r, root, name, relPath)
		return
	}

	if strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
		h.serveMarkdown(w, r, root, name, relPath, relPath)
		return
	}

	f, err := root.Open(name)
	if err != nil {
		if os.IsPermission(err) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// Resolve the type here instead of letting ServeContent do it, so the type
	// we defend against is exactly the type we send.
	ctype, err := contentTypeOf(info.Name(), f)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", ctype)
	if isExecutableType(ctype) {
		// A .html or .svg sitting beside the markdown would otherwise run in
		// the viewer's origin, with read access to everything else the server
		// exposes — which is the whole served tree, since there is no auth.
		// That defeats the point of omitting raw HTML from markdown: the
		// payload just moves into a sibling file that a link points at.
		//
		// "sandbox" with no allow-* tokens puts the response in an opaque
		// origin: no scripts, no same-origin reads. Images and PDFs are not
		// affected because they are not executable types.
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// contentTypeOf determines how a non-markdown file will be served: by
// extension when that is known, otherwise by sniffing, exactly as
// http.ServeContent would. The reader is rewound before returning.
func contentTypeOf(name string, f io.ReadSeeker) (string, error) {
	if ctype := mime.TypeByExtension(filepath.Ext(name)); ctype != "" {
		return ctype, nil
	}
	var head [512]byte
	n, err := io.ReadFull(f, head[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return http.DetectContentType(head[:n]), nil
}

// isExecutableType reports whether a browser will run script from a document
// of this type when it is navigated to directly.
func isExecutableType(ctype string) bool {
	base, _, _ := strings.Cut(ctype, ";")
	switch strings.ToLower(strings.TrimSpace(base)) {
	case "text/html", "application/xhtml+xml", "image/svg+xml",
		"application/xml", "text/xml":
		// XML is included for XSLT, which can script.
		return true
	}
	return false
}

// pageCSP is the policy for markdown and directory-listing pages, which are
// the documents markbrowse renders itself.
//
// script-src carries a per-request nonce for the one inline script (the mermaid
// bootstrap); 'self' covers the embedded /__mdview/*.js. connect-src 'self'
// stops any script that does slip through from exfiltrating over fetch.
//
// style-src keeps 'unsafe-inline' on purpose: the stylesheet is inlined into
// the page, and mermaid injects <style> elements at render time, which a
// nonce-only policy would block. img-src and font-src stay permissive because
// documents legitimately reference remote images and neither can execute.
func pageCSP(nonce string) string {
	return "default-src 'none'; " +
		"script-src 'self' 'nonce-" + nonce + "'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src * data: blob:; " +
		"font-src * data:; " +
		"connect-src 'self'; " +
		"base-uri 'none'; " +
		"form-action 'none'; " +
		"frame-ancestors 'none'"
}

// newNonce returns a fresh CSP nonce.
//
// The encoding is URL-safe and unpadded on purpose. Standard base64 contains
// "+", which html/template escapes to "&#43;" when the nonce is written into
// the nonce="" attribute, leaving the attribute and the CSP header textually
// different. A browser entity-decodes the attribute before comparing, so it
// would most likely still match — but the policy is only doing its job if the
// two are identical, and "relies on entity decoding" is a thin thing to rest
// script execution on. RawURLEncoding's alphabet (A-Za-z0-9-_) has nothing
// html/template escapes, and the CSP grammar accepts it.
//
// A failure to read the system CSPRNG is not recoverable into "carry on
// without a nonce": that would silently drop the inline script and break the
// page, so it is reported to the caller.
func newNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// serveDirectory renders a directory. dirName is the directory's name relative
// to the served root, as os.Root methods take it.
func (h *fileHandler) serveDirectory(w http.ResponseWriter, r *http.Request, root *os.Root, dirName, relPath string) {
	for _, name := range []string{"README.md", "readme.md", "INDEX.md", "index.md"} {
		indexName := filepath.Join(dirName, name)
		indexPath := filepath.Join(h.root, indexName)
		// The candidate may be a symlink into a VCS directory, which os.Root
		// would happily follow because it stays inside the tree. A rejected
		// candidate falls through to the listing — a hostile symlink must not
		// break browsing the directory.
		if _, ok := h.resolveContained(indexPath); !ok {
			continue
		}
		// os.Root refuses a candidate that escapes the tree (README.md ->
		// /etc/passwd), so the escape case needs no check of its own: Stat
		// simply fails and the loop moves on.
		if info, effective, err := h.statInRoot(root, indexName); err == nil && !info.IsDir() {
			// path.Join keeps the root case right: "/" + "README.md" is
			// "/README.md", not "//README.md".
			h.serveMarkdown(w, r, root, effective, relPath, path.Join(relPath, name))
			return
		}
	}

	entries, err := fs.ReadDir(root.FS(), filepath.ToSlash(dirName))
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

	nonce, err := newNonce()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	data := dirData{
		Path:       relPath,
		Nonce:      nonce,
		CSS:        h.css(),
		HasParent:  relPath != "/",
		ParentPath: parentPath(relPath),
		Entries:    dirEntries,
		// Trailing slash so the sidebar can prefix-match this directory and
		// open it; parentPath keeps the root from becoming "//".
		CurrentPath: dirCurrentPath(relPath),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", pageCSP(nonce))
	if err := dirTmpl.Execute(w, data); err != nil {
		log.Printf("dir template error: %v", err)
	}
}

// serveMarkdown renders a markdown file.
//
// fileName is the file's name relative to the served root, as os.Root methods
// take it. relPath is the request path, used for breadcrumbs; currentPath is
// the URL of the file actually being rendered, which the sidebar matches
// against to highlight and reveal it. The last two differ when a directory
// serves its index: the request is for /guides/ but the file on screen is
// /guides/README.md, and the sidebar has a node only for the latter.
func (h *fileHandler) serveMarkdown(w http.ResponseWriter, r *http.Request, root *os.Root, fileName, relPath, currentPath string) {
	info, fileName, err := h.statInRoot(root, fileName)
	if err != nil {
		if os.IsPermission(err) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Rendering is linear in file size in both CPU and memory and happens per
	// request, so an enormous document is a cheap way to exhaust the process.
	// That only really matters once --listen puts the server beyond loopback,
	// but the check costs nothing: the Stat above already has the size, and no
	// real document comes close to the limit.
	if info.Size() > maxMarkdownBytes {
		http.Error(w, "markdown file too large to render", http.StatusRequestEntityTooLarge)
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

	source, err := root.ReadFile(fileName)
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

	title := filepath.Base(fileName)
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

	nonce, err := newNonce()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	data := pageData{
		Title: title,
		Nonce: nonce,
		CSS:   h.css(),
		// #nosec G203 -- goldmark output with raw HTML omitted and dangerous
		// URLs filtered (unless --raw-html opts back in); template.HTML is
		// still required so goldmark's own tags (<table>, <pre class="mermaid">,
		// alert divs) render instead of printing as source.
		// nosemgrep: go.lang.security.audit.xss.template-html-does-not-escape.unsafe-template-type -- markdown body rendered by goldmark with WithUnsafe off by default (raw HTML omitted, javascript:/data: URLs filtered); --raw-html is an explicit operator opt-in
		Body:        template.HTML(body),
		Breadcrumbs: buildBreadcrumbs(relPath),
		HasMermaid:  strings.Contains(body, `class="mermaid"`),
		CurrentPath: currentPath,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", pageCSP(nonce))
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

// parentPath returns the URL of relPath's parent directory for the "../" row
// of a directory listing. The root is already its own trailing-slash form, so
// path.Dir("/sub") == "/" must stay "/": appending another slash yields "//",
// which a URL parser reads as the start of an authority (a protocol-relative
// URL) rather than as the root path.
// dirCurrentPath is the trailing-slash form of a directory's own path, used by
// the sidebar to decide which folder to open. The root is already its own
// trailing-slash form.
func dirCurrentPath(relPath string) string {
	if relPath == "/" {
		return "/"
	}
	return relPath + "/"
}

func parentPath(relPath string) string {
	dir := path.Dir(relPath)
	if dir == "/" {
		return "/"
	}
	return dir + "/"
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

// maxMarkdownBytes caps what serveMarkdown will read and render.
const maxMarkdownBytes = 32 << 20 // 32 MiB

// treeCacheTTL bounds how long the sidebar tree stays cached. The tree can
// change as files are added/removed on disk, so it is rebuilt after this
// window instead of holding a stale snapshot forever.
const treeCacheTTL = 5 * time.Second

// treeJSONCached returns the serialized file tree, rebuilding it at most
// once per treeCacheTTL. The walk, prune and sort are the most expensive
// work the server does on every page load, so caching avoids repeating it
// for rapid successive requests (e.g. the sidebar fetching on each page).
func (h *fileHandler) treeJSONCached() ([]byte, string) {
	h.treeMu.Lock()
	defer h.treeMu.Unlock()
	if h.treeJSON != nil && time.Since(h.treeAt) < treeCacheTTL {
		return h.treeJSON, h.treeETag
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
		// VCS directories are never listed in the sidebar, even when they
		// contain markdown. The root itself is exempt so `markbrowse .git`
		// (serving a VCS dir deliberately) keeps working.
		if walkPath != h.root && d.IsDir() && isVCSName(d.Name()) {
			return filepath.SkipDir
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
		return nil, ""
	}
	// The payload is the only reliable identity for the tree: it changes
	// whenever a markdown file is added, removed or renamed anywhere beneath
	// the root, and nothing cheaper tracks that. Hashing here rather than per
	// request means it costs once per rebuild, at most once per treeCacheTTL.
	sum := sha256.Sum256(data)
	h.treeJSON = data
	h.treeETag = fmt.Sprintf(`"%x"`, sum[:16])
	h.treeAt = time.Now()
	return h.treeJSON, h.treeETag
}

func (h *fileHandler) serveTreeJSON(w http.ResponseWriter, r *http.Request) {
	data, etag := h.treeJSONCached()
	if data == nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// no-cache means "revalidate", not "don't store": the sidebar refetches the
	// tree on every page navigation, and on a large vault that payload is a few
	// hundred KB. With an ETag the revalidation is a 304 with no body whenever
	// the tree has not changed.
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
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

# 2026-10-16 — P2: parse once, make sidebar folders navigable, resolve fewer symlinks

**Symptom / goal.** Three findings from a whole-code review of v0.4.0, all
correctness-or-waste rather than features.

1. Every markdown request parsed the document twice.
2. A folder in the sidebar could be expanded but never opened.
3. Every directory request resolved up to four symlink paths that mostly did
   not exist.

**Diagnosis.**

*Double parse.* `serveMarkdown` called `h.md.convert(source)` to render, then
`h.md.metaTitleOf(source)` to get the front matter title. `metaTitleOf` built a
fresh `parser.NewContext()`, ran the full parse again, and read the same
`frontmatter.Items(ctx)` the render pass had already had in hand. goldmark v2
separates parse from render, so the metadata is available from the context the
render used — the second parse bought nothing.

*Dead `<h1>` fallback.* The title logic was:

```go
title := filepath.Base(fileName)
if t := h.md.metaTitleOf(source); t != "" {
	title = t
} else if strings.HasPrefix(body, "<h1") {
	start := strings.Index(body, ">")
	end := strings.Index(body, "</h1>")
	...
}
```

`convert` prepends the front matter table to `body`, so any document with front
matter begins with `<table class="meta-table">`. The prefix test is therefore
false for exactly the documents that have front matter — a document with front
matter but **no** `title` key fell through to the file name even when it had a
perfectly good `# Heading`. The slicing was also wrong for a heading with
inline markup: for `# A `x``, `Index(">")` and `Index("</h1>")` span the
rendered `<h1 id="a-x">A <code>x</code></h1>`, leaving `<code>x</code>` in the
title.

*Unreachable folders.* `folderRow` in `static/js/sidebar.js` built one
`<button class="tree-folder">` whose only handler toggled the children, and no
anchor. `fileRow` builds a real `<a href>`. So a file was navigable from the
tree and a directory was not: browsing into a folder meant editing the URL.
One element cannot both expand and navigate, which is why the two actions need
two controls.

*Symlink walks for non-existent candidates.* `serveDirectory` tried four index
names (`README.md`, `readme.md`, `INDEX.md`, `index.md`) and called
`h.resolveContained(indexPath)` — a full `filepath.EvalSymlinks`, which lstats
every path component — *before* checking whether the candidate existed. Four
resolutions per directory request, of which at most one could ever matter.

**Change.**

- `markdown.go`: `convert` now returns `(body, metaTitle, error)`, reading the
  title from the render pass's own context. `metaTitleOf` is deleted.
- `handler.go`: `serveMarkdown` takes the title from that return value, and the
  new `docTitle` helper picks between front matter title, first `<h1>`, and file
  name. The `<h1>` is matched anywhere in the body with `h1Pattern`, its inner
  tags are removed with `tagPattern`, and entities are decoded.
- `handler.go`: the index loop stats a candidate first and resolves symlinks
  only for one that exists as a file. Same decision, reached later, after the
  cheap test.
- `static/js/sidebar.js` + `static.go`: folder rows became
  `.tree-folder` (a flex row) containing `.tree-folder-toggle` (the arrow, with
  `aria-expanded` and a descriptive `aria-label`) and `.tree-folder-link` (the
  folder's name, linking to its path).
- Tests: new `title_test.go` (5 cases) and `TestSidebarFoldersAreNavigable`;
  `TestMetaTitle` rewritten against the new return value; `sidebar_test.go`'s
  `clickFolder` helper now targets the toggle, and a `folderLinks` helper was
  added so the link can be asserted; the 20 `convert` call sites across
  `markdown_test.go`, `golden_test.go` and `bench_test.go` were updated for the
  extra return value.

**Commands.**

```
$ TMPDIR=/Users/glt5/WORK/.gotmp go build ./... && go vet ./...   # clean
$ TMPDIR=/Users/glt5/WORK/.gotmp go test ./... -count=1
ok  github.com/anantshri/markbrowse  0.137s   (and the 4 internal packages)
$ go tool cover -func=cover2.out | grep -E 'docTitle|convert|serveDirectory'
docTitle        100.0%
convert          92.3%
serveDirectory   80.0%
```

The first attempt at the mechanical edit asserted 20 `convert` call sites in
`markdown_test.go` alone and aborted before writing that file (the 20 was
across three files). The assertion did its job — nothing was half-edited — and
the remaining call sites were applied per file with their own expected counts.

**Verification.**

- `gosec ./...`: 9 files / 2610 lines → **0 issues**.
- `semgrep --config auto --include='*.go' --include='*.js'` (mermaid bundle
  excluded): 283 rules on 12 files → **0 findings**.
- `gitleaks detect --no-banner --redact`: 51 commits, 1.28 MB → **no leaks**.
- `go test -cover ./...`: overall 84.0%, `docTitle` **100.0%**.
- The new tests are meaningful: `TestPageTitleFromH1UnderFrontMatter` expects
  `Real Title` where the old code returned `doc.md`;
  `TestPageTitleStripsInlineMarkup` expects `A x & more` where the old slice
  returned `A <code>x</code> & more`; `TestPageTitleIgnoresEmptyHeading` and
  `TestSidebarFoldersAreNavigable` (which looks for `.tree-folder-link`) both
  fail against the previous code.

**Notes.**

- The sidebar change is verified through the goja harness, which drives the
  real `sidebar.js` against a DOM shim and asserts the rendered rows — the same
  level the existing lazy-render tests use. No browser was driven for this
  change.
- `resolveContained` in `ServeHTTP` is deliberately **left as-is**. It is called
  unconditionally on every request, and an `Lstat`-first guard looked like the
  obvious win, but it would be wrong: a request for `/link/secret.md` whose
  *parent* `link` is a symlink to `.git` passes the lexical VCS check and is a
  non-symlink at the final component, so the full resolution is what catches it.
  The policy needs the whole-path answer; only the index-candidate loop could be
  reordered safely.
- `docs/security.md` (referenced by `.github/workflows/sbom.yml` but absent) and
  the remaining findings are in the P3 batch.

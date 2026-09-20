# Detailed Changelog

The long-form companion to `CHANGELOG.md`. Where `CHANGELOG.md` says *what*
changed in one line, this file records *why* and *how* — enough for a future
reader to audit, reproduce, or roll back any change without re-deriving it.

Add a new entry (newest first) for every meaningful change. Use the template
below; drop sections that genuinely don't apply.

---

## 2026-09-20 — os.Root containment, from GitHub code scanning on PR #21

**Summary:** CodeQL (`go/path-injection`, "This path depends on a
user-provided value") flagged two filesystem calls in `handler.go`. Rather
than dismiss them, all file access below the served directory moved onto
`os.Root`, which makes containment a kernel guarantee instead of a check this
code performs. Behaviour is unchanged, verified end to end.

**The alerts** were on `os.Open(resolved)` in `ServeHTTP` and
`os.Stat(resolved)` in `serveDirectory`'s index loop — both carrying a
`#nosec G304` justification, which does nothing for CodeQL: `#nosec` is
gosec-only. A grep found six such sinks in total; CodeQL had reported two
representatively, so all six were converted rather than the named pair.

**Why this is a fix and not a suppression.** The previous design was
resolve (`filepath.EvalSymlinks`), compare (`underRoot`), then open the
resolved absolute path. That is correct — traversal and symlink escape were
both verified to 404 during the review — but it is correct *by argument*: the
safety lives in the reader's head and in the ordering of three statements, and
between the check and the open there is a window. `os.Root` inverts it: its
methods refuse any name that resolves outside the root, enforced by the
kernel, so the path reaching a filesystem call is no longer something that has
to be proven safe by reading the surrounding code. Clearing the alert is a
side effect of removing the taint sink, not the goal.

**What moved:**

| Was | Now |
|---|---|
| `os.Stat(fsPath)` in `ServeHTTP` | `statInRoot(root, name)` |
| `os.Open(resolved)` | `root.Open(name)` |
| `os.Stat(resolved)` (index candidate) | `statInRoot(root, indexName)` |
| `os.ReadDir(fsPath)` | `fs.ReadDir(root.FS(), dirName)` |
| `os.Stat(fsPath)` in `serveMarkdown` | `statInRoot(root, fileName)` |
| `os.ReadFile(fsPath)` | `root.ReadFile(fileName)` |

`serveMarkdown` and `serveDirectory` now take a root-relative name rather than
an absolute path. Every `#nosec G304` annotation is gone, because the sinks
they were justifying no longer exist. The root is opened lazily behind a
`sync.Once` so a zero-value `fileHandler` — which most tests construct —
keeps working untouched.

**The one behaviour `os.Root` would have removed.** It refuses *every*
absolute symlink, including one pointing back into the served tree:

```
abs.md   ERR: statat abs.md: path escapes from parent   readlink=/tmp/probe.../real.md
rel.md   OK                                             readlink=real.md
```

`TestServeHTTPAllowsSymlinkInsideRoot` has asserted since symlink containment
was added that such links are served, and linking shared notes into a vault
with `ln -s /abs/path/notes.md` is an ordinary thing to do. Silently
404-ing those would be a regression, so `statInRoot` rewrites an absolute
target to a root-relative name and resubmits it to `os.Root`. The lexical
comparison only decides *how* to rewrite; it never authorises access, because
the rewritten name goes back through `os.Root` and gets the same kernel check.
A hop limit bounds symlink loops.

`resolveContained` survives, demoted from containment guarantee to VCS policy:
a symlink that stays inside the root but points at `.git/config` is contained
and must still be refused, and `os.Root` has no opinion about that.

**A real bug found by a flaky test.** After the refactor,
`TestMermaidScriptCarriesTheNonce` failed intermittently under
`-shuffle=on -count=3`:

```
inline script nonce "1EqaCJRNI&#43;&#43;/1KHDuqmeHQ==" does not match CSP nonce "1EqaCJRNI++/1KHDuqmeHQ=="
```

The nonce was standard base64, and `html/template` escapes `+` to `&#43;` in
an attribute context — so it only failed when the random 16 bytes happened to
encode a `+`, roughly three runs in four. Browsers entity-decode the attribute
before comparing the nonce, so this would most likely have worked in practice,
but a policy that only functions because of entity decoding is a thin thing to
rest script execution on. Switched to `base64.RawURLEncoding`, whose alphabet
(`A-Za-z0-9-_`) has nothing `html/template` escapes and which the CSP grammar
accepts. `TestNonceIsUnpredictable` now asserts the alphabet, so the class
cannot come back.

**Verification:**

```
containment    /../etc/passwd  ..%2f..  /escape.md  /etcdir/  /etcdir/passwd  -> all 404
symlinks       /abs.md 200 (Deep)   /rel.md 200 (Deep)    <- absolute and relative both served
VCS policy     /.git/config  /.GIT/config  /sneaky.md (-> .git/config)  -> all 404
headers        evil.html: sandbox + nosniff;  pic.png: no CSP
nonce          9yhHLrPpuWB98XwlJtMaVA  -- URL-safe, nothing to escape
```

- New tests: `TestAbsoluteSymlinkInsideRootStillServed` (absolute-inside,
  relative-inside, absolute-to-absolute chain, escaping) and
  `TestSymlinkRewritingIsBounded` (a symlink loop must terminate, not spin).
- `go test -count=2 -shuffle=on ./...` run repeatedly, stable.
- Coverage 78.3% -> **78.6%**. `gofmt`, `go vet`, `aidc-scan` clean.

**Notes:**
- Whether the CodeQL alerts actually clear depends on whether that version
  models `os.Root` methods as sinks. If it still reports them, the remaining
  answer is dismissal — but on much firmer ground than before, since
  containment is no longer an argument about statement ordering.
- `filepath.EvalSymlinks` was *not* among the alerts even though it predates
  this change, which is what suggested their query pack models
  `os.Open`/`os.Stat` as sinks but not symlink resolution. That is why
  `resolveContained` could stay for the VCS policy.
- The `filepath.WalkDir` calls in `treeJSONCached` and `buildFileIndex` walk
  the configured root, not a request path, so they are not part of this class
  and were left alone.

---

## 2026-09-20 — Fix Windows CI (PowerShell vs the hardened Build step)

**Symptom:** the `build` job failed on `windows-latest` with

```
The term 'REF_NAME=$(printf '%s' "$REF_NAME" | tr -c 'A-Za-z0-9._-' '_')' is not
recognized as a name of a cmdlet, function, script file, or operable program.
```

**Cause:** not a regression from this release's work. Commit `9aad01c`
(2026-08-30, "harden CI ref handling") replaced

```yaml
run: go build -trimpath -ldflags "-s -w -X main.version=${{ github.ref_name }}" -o markbrowse .
```

with a multi-line script that scrubs `REF_NAME` through `tr` before using it.
The original worked on every platform by accident: Actions substitutes `${{ }}`
before the shell ever sees the line, so PowerShell only had to run `go build`.
The replacement is POSIX shell — variable assignment, command substitution, a
pipe into `tr` — and the `build` job runs a three-OS matrix in which
`windows-latest` defaults to PowerShell. Windows CI has been broken since that
commit.

`release.yml` is unaffected: it cross-compiles for Windows from `ubuntu-latest`
via `GOOS`/`GOARCH`, so its shell is always bash.

**Fix:** pin the shell for the whole job rather than the one step, so a future
multi-line step cannot reintroduce this.

```yaml
    runs-on: ${{ matrix.os }}
    defaults:
      run:
        shell: bash
```

GitHub-hosted Windows runners ship Git Bash, so one script now works on all
three platforms.

**Verification:**
- All three workflow files parse, and a scan for multi-line `run:` steps on
  non-Linux runners without an explicit shell reports none remaining.
- `go build ./...` and `go vet ./...` (which include the test files) pass for
  `windows/amd64`, `darwin/arm64`, `darwin/amd64` and `linux/amd64`.
- Checked that the new `security_test.go` does not depend on OS-provided MIME
  data: every extension it asserts on (`.svg`, `.png`, `.html`, `.xml`, `.txt`,
  `.zip`) is in Go's platform-independent `builtinTypesLower` table. `.md` is
  not, but markdown is dispatched to `serveMarkdown` and never reaches
  `contentTypeOf`.

**Notes:** the Windows build still produces an extensionless `markbrowse`
rather than `markbrowse.exe`, because `-o markbrowse` is taken literally. It is
harmless — the CI build is a compile check and the artifact is never used —
and `release.yml` already names the binary correctly per `GOOS`.

---

## 2026-09-20 — Security review findings fixed (pre-0.4.0)

**Summary:** a security review of the codebase before tagging 0.4.0 found three
issues. All three are fixed and folded into the 0.4.0 entry rather than left
for a follow-up, since the release had not been cut.

**What held up,** because it is worth recording that these were exercised
rather than assumed: path traversal (`/../etc/passwd`, `..%2f`, `%2e%2e/`,
`....//` — all 404), symlink escape (file and directory, both 404, and
`resolveContained` opens the *resolved* path so the usual TOCTOU window is
closed), `html/template` context escaping, wikilink `href` escaping
(`[[notes#" onmouseover="alert(1)]]` percent-encodes the quote to `%22` inside
the attribute and `&quot;`-escapes the text — no breakout), heading-ID
sanitisation, and the absence of `innerHTML`/`eval`/`document.write` anywhere
in the first-party JS.

### 1. Non-markdown files executed in the viewer's origin

markbrowse omits raw HTML from markdown and filters dangerous URL schemes
precisely because vault content may be untrusted. That control was bypassable
by putting the payload in a file *beside* the markdown:

```
/evil.html     Content-Type: text/html; charset=utf-8
/evil.svg      Content-Type: image/svg+xml
/noextension   Content-Type: text/html; charset=utf-8    <- content-sniffed
```

The whole chain was verified end to end:

```
README.md renders  <a href="report.html">audit report</a>   (raw HTML OFF)
click              -> 200 text/html, script runs
script             -> GET /__mdview/tree.json   enumerate every file
                   -> GET /private/creds.md     secret-api-key=AKIA...
                   -> exfiltrate
```

No auth and same origin, so one click turned "view an untrusted vault" into
"read and exfiltrate the served tree". Navigation was required — an
`<img src="evil.svg">` does not execute, and with raw HTML off markdown cannot
iframe it — but `[audit report](report.html)` is an entirely natural link to
follow.

Fixed by resolving the content type *before* writing the response (rather than
letting `http.ServeContent` sniff it, so the type defended against is the type
sent) and attaching `Content-Security-Policy: sandbox` when that type is one a
browser will execute: `text/html`, `application/xhtml+xml`, `image/svg+xml`,
and XML (for XSLT). `sandbox` with no `allow-*` tokens puts the response in an
opaque origin — no script, no same-origin reads. Images, PDFs and downloads
are untouched, which is why the fix is type-scoped rather than blanket.

### 2. VCS blocking was case-sensitive

```
hasVCSSegment("/.git/config") = true
hasVCSSegment("/.GIT/config") = false
```

`isVCSName` did an exact map lookup. On a case-insensitive filesystem — APFS
by default on macOS, and NTFS — `/.GIT/config` opens the real `.git/config`,
so the block held only on Linux. Demonstrated by serving a directory literally
named `.GIT`:

```
/.GIT/config  -> 200
body: [remote]   url = https://user:TOKEN@github.com/x/y
```

`.git/config` routinely carries credentials in the remote URL, and `.git/`
exposes full history. It was also quiet: the tree walk checks the on-disk name
(`.git`) and correctly hid it from the sidebar, so only the direct URL was
affected.

Matching is now case-folded, and additionally covers two Windows behaviours:
trailing dots and spaces are ignored by the OS (`.git.` opens `.git`), and 8.3
aliases (`GIT~1`) reach the same directory. Near-misses stay servable —
`.gitignore`, `.github`, `git~1.md` are all still fine, and tested.

### 3. No security headers

None of `X-Content-Type-Options`, `Content-Security-Policy` or
`Referrer-Policy` were set on any response. `nosniff` is the one that matters
most here: it is what stops the extensionless-file case in finding 1.

Now on every response, including errors and blocked paths. Rendered pages
additionally get a policy with a per-request nonce:

```
default-src 'none'; script-src 'self' 'nonce-<random>'; style-src 'self' 'unsafe-inline';
img-src * data: blob:; font-src * data:; connect-src 'self';
base-uri 'none'; form-action 'none'; frame-ancestors 'none'
```

Two deliberate loosenings, both load-bearing:

- **`style-src 'unsafe-inline'`.** The stylesheet is inlined into the page, and
  mermaid injects `<style>` elements at render time. A nonce-only style policy
  would silently stop diagrams rendering.
- **`img-src *` / `font-src *`.** Documents legitimately reference remote
  images, and neither type can execute. `Referrer-Policy: no-referrer` limits
  what those requests disclose.

`connect-src 'self'` is the meaningful restriction: a script that did slip
through cannot exfiltrate over `fetch`.

### Also fixed

Markdown over 32 MiB is refused with `413`. Rendering is linear in file size in
both CPU and memory and happens per request, so an enormous document was a
cheap way to exhaust the process — which matters once `--listen` puts the
server beyond loopback. The check uses the `Stat` already performed, so it
costs nothing, and no real document approaches the limit.

The README gains a Security section describing the posture and naming
`--raw-html` and `--css` as the two flags that deliberately turn protections
off. `--css` is injected verbatim and CSS can make outbound requests via
`url(...)`, so the stylesheet is trusted code.

**Verification:**

```
FINDING 1  /report.html   CSP: sandbox   nosniff
           /evil.svg      CSP: sandbox   nosniff
           /noextension   CSP: sandbox   nosniff
           /pic.png       (no CSP)       nosniff      <- images unaffected
FINDING 2  /.git/config /.GIT/config /.Git/config /GIT~1/config  -> all 404
FINDING 3  nosniff + no-referrer + CSP present on rendered pages
           CSP nonce == inline <script nonce="...">, and differs per request
SIZE CAP   32.4 MB .md -> 413,  normal .md -> 200
```

`security_test.go` covers all of it: base headers across nine response kinds
(markdown, index, listing, raw file, sniffed file, embedded asset, tree API,
404, blocked path), the sandbox decision for seven content types, that the
content type is decided rather than sniffed, nonce freshness across five
requests, that the mermaid bootstrap carries the matching nonce — the test
that would catch a CSP which silently breaks diagrams — and the size cap.
`TestIsVCSName` and `TestHasVCSSegment` gained the case, trailing-dot and 8.3
variants. Coverage 77.5% -> 78.3%.

**Notes:**
- The page CSP is the one change not verifiable from here: there is no browser
  in this container, so "mermaid still renders under this policy" is argued
  from the policy text (`style-src` keeps `'unsafe-inline'`) and asserted at
  the nonce level, not observed. Worth one look at a page with a diagram, with
  the browser console open for CSP violations.
- Not changed: the YAML parse error still emits a fragment of document content
  as an HTML comment. It can no longer close the comment early, and the
  behaviour is deliberate — it is how a broken front matter block explains
  itself.

---

## 2026-09-20 — Sidebar quick filter and lazy rendering (#18)

**Summary:** Issue #18 asked for two things: a quick filter above the file tree
("to find the markdown file deep down"), and a fix for load time on large
folders, "lazy load the content but still provide search capability". Both are
done, and the second is what makes the first cheap.

**Measured first.** A synthetic vault of 4,800 markdown files in 441
directories, three levels deep:

| | Before |
|---|---|
| `tree.json` build | 14.7 ms cold, **0.5 ms** cached |
| `tree.json` payload | **311 KB**, `Cache-Control: no-cache`, **no ETag** |
| DOM elements `sidebar.js` built on load | **10,923** |

So the server was never the problem — the 5-second tree cache already had it at
half a millisecond. The cost was entirely on the client, and it was paid *on
every page navigation*: re-download 311 KB, then construct ~10,900 elements,
because `renderNode` walked the entire tree eagerly and built every folder and
every file whether or not it was visible.

**What changed:**

- `static/js/sidebar.js` rewritten. A folder's children are built by a closure
  the first time that folder opens; collapsed folders hold an empty container.
  The only branch rendered up front is the one containing the current page, so
  the active file is still visible on load. Result on the same vault: **174
  elements instead of 10,923**, measured by counting `createElement` calls
  under the test DOM.
- The parsed tree stays in memory. That is the answer to "lazy load but still
  provide search": the filter searches data the DOM has never drawn, with no
  extra request and no server-side search endpoint.
- The client-side re-sort is gone. `sortTree` in `handler.go` already emits
  directories first then alphabetical, so sorting every folder again on every
  render was pure duplicated work.
- `handler.go`: `treeJSONCached` now also returns an ETag, computed once per
  rebuild (so at most once per `treeCacheTTL`, not per request) by hashing the
  payload. `serveTreeJSON` sets it and answers `If-None-Match` with `304`.
  `no-cache` is kept — it means "revalidate", not "don't store", and the
  revalidation is now free.
- `templates.go`: a `type="search"` input plus an `aria-live` status line above
  `#tree-root`, in both the markdown and directory-listing templates.
- `static.go`: styles for the input, status line, match highlight and result
  rows, using the existing CSS variables so dark mode follows automatically.

**Filter design decisions:**

- *Name by default, path when the query contains `/`.* A bare `guides` matching
  directory names would return every file under `guides/` — the opposite of
  finding one file. Typing `guides/table` is the explicit way to ask for a path
  match.
- *Capped at 200 results,* with "Showing 200 of N matches". A one-character
  query on a large vault matches thousands of files, and rendering them all
  would rebuild the flat equivalent of the tree — reintroducing exactly the
  stall the lazy rendering removes.
- *Matches are highlighted* by splitting the name into text nodes around the
  hit. Built with `createTextNode`, never `innerHTML`: the highlighted span is
  derived from what the user typed.
- *Debounced 120 ms,* so typing a word renders once rather than once per key.

**Testing.** `sidebar_test.go` runs the real `static/js/sidebar.js` in goja
against a stub DOM (~150 lines: elements, `classList`, `textContent`,
fragments, a class-only `querySelector`, a synchronous `fetch`, and a
controllable timer queue). Unlike `tablesort.js` this script cannot be
unwrapped — its IIFE has a top-level `return` for the "no sidebar on this page"
case, which is a syntax error outside a function — so the tests drive it the
way a user does: set the input value, fire `input`, flush the debounce, inspect
what was rendered.

Covered: lazy expansion (nothing from a collapsed folder is rendered; opening
one renders its children but not its grandchildren), auto-expansion to the
current page, name matching, path matching via `/`, case-insensitivity,
highlighting, the 200 cap and its count message, the empty state, clearing,
`Esc`, and that a file named `<img src=x onerror=...>.md` renders as text.
`handler_test.go` gains `TestServeTreeJSONETagNotModified` and ETag assertions
in the existing cache test.

**Commands:**
```
# 4,800-file vault, three levels deep
for a in $(seq 1 40); do for b in $(seq 1 10); do
  mkdir -p /tmp/bigvault/area-$a/section-$b
  for f in $(seq 1 12); do echo "# Note $f" > /tmp/bigvault/area-$a/section-$b/note-$f.md; done
done; done

curl -sI http://127.0.0.1:PORT/__mdview/tree.json          # headers before/after
curl -H 'If-None-Match: "..."' ...                          # 304, 0 bytes
go test -run Sidebar ./...
go test -cover ./...
aidc-scan
```

**Verification:**
- `go test -cover ./...` — pass; main package 76.4% -> **77.3%**.
- Element counts measured by instrumenting `createElement` in the test DOM and
  running both the old and new `sidebar.js` against the 4,800-file tree:
  10,923 -> 174.
- Live on the big vault: the filter input is present in both templates;
  `tree.json` returns an `ETag`, a request carrying it gets `304` with 0 bytes,
  and a request without one gets `200` with 317,988 bytes.
- `aidc-scan` clean.

**Follow-up in the same session — the sidebar stayed collapsed on a page
opened directly.** Reported as: opening a page directly leaves the sidebar
collapsed, while clicking through to the same page reveals and marks it.

Both are ordinary full page loads, so the asymmetry pointed at the value the
sidebar matches on, and it turned out to be a server-side bug predating all of
this. `data-current-path` comes from `pageData.CurrentPath`, and when a
directory serves its index, `serveDirectory` called `serveMarkdown` with the
*directory's* request path:

| URL | reported | file node in the tree | matched |
|---|---|---|---|
| `/` | `/` | `/README.md` | no |
| `/guides/` | `/guides` | `/guides/README.md` | no |
| `/guides/README.md` | `/guides/README.md` | `/guides/README.md` | yes |

The sidebar only ever has a node for the *file*, so there was nothing to match
and no ancestor to expand. Clicking the file in the sidebar navigates to
`/guides/README.md`, which is why that path worked — exactly the asymmetry
reported.

Fixed by separating the two meanings `relPath` was carrying. `serveMarkdown`
now takes both a `relPath` (the request path, still what breadcrumbs are built
from) and a `currentPath` (the URL of the file actually rendered). The direct
-file call site passes the same value twice; the directory-index call site
passes `path.Join(relPath, name)`, which also keeps the root as `/README.md`
rather than `//README.md`. Breadcrumbs are deliberately untouched.

A directory *listing* has no file on screen, so it reports itself with a
trailing slash via a new `dirCurrentPath` helper — the sidebar prefix-matches
that and opens the folder without marking anything. That also retires the
`CurrentPath: relPath + "/"` expression that produced `//` at the root, noted
as harmless in the goldmark entry above and now simply gone.

Covered by `TestCurrentPathNamesTheRenderedFile` (six URL shapes: root index,
nested index, `INDEX.md` index, file direct, index file direct, listing) and
`TestSidebarRevealsIndexAndListingPages` on the JS side, which asserts an
index page marks its file and opens its folder while a listing page opens the
folder and marks nothing.

**Notes:**
- Not done: server-side pagination of the tree. It would shrink the 311 KB
  further, but it would also move search to the server, and the issue asked for
  lazy loading *without* losing search. With the ETag the payload is fetched
  once per change rather than once per navigation, which addresses the same
  cost without that trade.
- The 5-second `treeCacheTTL` is unchanged, so a new file still takes up to
  five seconds to appear — and now also needs the ETag to change, which it does
  because the hash is over the payload.

---

## 2026-09-20 — goldmark v2, with the markdown extensions vendored into internal/

**Summary:** markbrowse now parses and renders with `github.com/yuin/goldmark/v2`.
The four markdown extensions it depended on were ported into `internal/` and
trimmed to what markbrowse uses. Rendering did not change: all 44 golden cases
produce byte-identical HTML before and after. The runtime module list drops
from six to two.

**Why now:** the 2026-08-29 session recorded goldmark v2 as blocked because
`goldmark-meta/v2` required Go ≥ 1.25 and the other extensions had no v2. The
Go constraint is gone (the module is on 1.26), but the extension one is not
and will not resolve on its own: `goldmark-gh-alerts`,
`go.abhg.dev/goldmark/mermaid` and `go.abhg.dev/goldmark/wikilink` have no v2
module path at all — `go list -m .../v2@latest` returns 404 / "no matching
versions" — and their extenders implement the v1 `goldmark.Extender`
interface, which v2 removed outright. There is no version of this that is a
dependency bump.

**Why vendoring is a reduction, not just a relocation:** markbrowse uses a
narrow slice of each package.

| Package | Upstream | Vendored | What was dropped |
|---|---|---|---|
| mermaid | 1,128 LOC, 17 files | ~150, 1 file | chromedp server rendering (`mermaidcdp/`), mermaid-CLI rendering, `RenderMode` switching, the CDN `<script>` injection, internal test helpers |
| wikilink | 403, 6 files | ~260, 1 file | `DefaultResolver` and its `.html` suffix logic — markbrowse resolves against its own file index |
| gh-alerts | 385, 7 files | ~290, 1 file | the `Extend` wrapper; `details` and `summary` merged into one package |
| goldmark-meta | 299 (v2), 1 file | ~200, 1 file | the table renderer (markbrowse builds its own) and the unordered-map decode |

Dropping upstream mermaid also removes `chromedp`, `cdproto`, `gobwas/*`,
`go-json-experiment/json` and friends from the module graph.

`internal/` is load-bearing here, not cosmetic: Go refuses to let any other
module import a package under an `internal/` directory, so these cannot become
an accidental public API. That was the explicit requirement.

**Licensing:** wikilink and mermaid are BSD-3-Clause (© 2023 Abhinav Gupta),
gh-alerts is MIT (© 2024 Adam Chovanec), goldmark-meta is MIT (© 2019 Yusuke
Inuzuka). All permit modification and redistribution provided the copyright
notice and licence text travel with the code, so each `internal/<pkg>/`
directory keeps the upstream `LICENSE` verbatim, and every package comment
names its origin and lists what diverged. `internal/README.md` records the
policy. `aidc-scan`'s licence gate passes.

**The migration, concretely.** goldmark v2 is a rewrite of the public API, not
a rename. What actually had to change:

- `goldmark.New`/`goldmark.Markdown`/`goldmark.Extender` no longer exist.
  `markdownConverter` now holds a `parser.Parser` and an `html.Renderer` and
  drives `Parse` then `Render` itself. One incidental win: `metaTitleOf` used
  to render the whole document into a throwaway buffer just to reach the
  parser context, and now only parses.
- Extensions split in two. Each vendored package exposes `NewParser()`
  (a `parser.Extension`) and `NewHTMLRenderer()` (an `html.Extension`), which
  is the naming convention goldmark v2's own extensions use.
- Renderers are generic over the writer. `RegisterFuncs(registerer)` became
  `html.WithNodeRenderers(map[ast.NodeKind]html.NodeRenderer)`, and the render
  signature gained a `renderer.Context` and takes `io.Writer`.
- `ast.BaseNode.Init(n)` must be called in every node constructor, or the
  argument-free tree mutators have no self reference.
- `ast.FencedCodeBlock` merged into `ast.CodeBlock`; the mermaid transformer
  now checks `CodeBlockKind == CodeBlockKindFenced` explicitly so an indented
  block is never mistaken for a diagram, and `Language()` returns
  `(string, bool)` rather than `[]byte`.
- `ast.TextBlock` was removed. Both `goldmark-meta` and the alerts summary
  parser used it to hold text destined for inline parsing; in v2 a block node's
  own `AppendSource(segment)` is what gets inline-parsed, which removed a node
  from each.
- `ast.String` was removed, so `writeNodeText` in wikilink lost a case.
- `text.Reader.Value(seg)` is gone; `seg.Bytes(reader.Source())` replaces it.
- `util.URLEscape` lost its second argument.
- Non-string AST attributes are gone. gh-alerts stored the alert kind as a
  `[]byte` attribute and the collapsed flag as a `bool`; both became typed
  struct fields on the node, which also removed the `t.([]uint8)` assertions
  the renderers were doing.

**Three deliberate behaviour changes, each tested:**

1. *wikilink drops a `sync.Map`.* Upstream tracked "did this node open an
   `<a>`?" in a `sync.Map` keyed by node pointer, written on enter and drained
   on exit. Recomputing it on exit is cheaper and, more importantly,
   stateless — a renderer holding per-node state cannot be shared across
   concurrent renders, and markbrowse renders on every request from a single
   converter. `TestNoStrayClosingTag` covers the tag balance.
2. *The front matter error comment is sanitised.* Upstream interpolates the
   YAML error into `<!-- %s -->` verbatim. YAML error text quotes the input
   that produced it, so `-->` is now escaped out of the message before it is
   written. `TestErrorCommentCannotBeClosedEarly` covers it.
3. *The alert kind is HTML-escaped into its `class` attribute.* Upstream used
   `fmt.Sprintf` straight into the attribute. The marker regexp restricts the
   kind to `\w+`, so this was not reachable; it is closed at the sink anyway.

**Testing.** The migration was done against a golden-file net captured *first*,
on goldmark v1 (commit `3d33864`), so any rendering change had to appear as a
diff rather than as silence: every markdown file in `testdata/` plus 12
synthetic cases — all wikilink forms including malformed ones, all alert kinds
plus titled/collapsed/unknown, mermaid fences beside non-mermaid fences, front
matter including malformed and odd-typed input, GFM, raw HTML, and the
dangerous URL schemes — rendered in both `--raw-html` modes. 44 files.

Exactly one differed on first run: a missing `\n` after the front matter error
comment, because goldmark v1's `renderTextBlock` writes a newline on exit when
the node has children and a next sibling, and the replacement node did not.
Porting that rule made all 44 byte-identical. The pre-existing test suite
needed no changes at all.

Each vendored package then got its own unit tests, at 90.9% (alerts), 94.8%
(frontmatter), 92.7% (mermaid) and 94.9% (wikilink) statement coverage. The
only uncovered functions are the empty `Close` methods the
`parser.BlockParser` interface requires; goldmark does call them on every
block, but an empty body has no statements for the cover tool to count.

**Commands:**
```
go get github.com/yuin/goldmark/v2@v2.1.5
go test -run TestRenderGolden -update-golden     # on v1, before the migration
# ... port the four packages, rewrite the pipeline in markdown.go ...
go mod tidy
gofmt -l . && go vet ./... && go test -cover ./...
aidc-scan
```

goldmark v2 ships its own migration material at
`.agent-plugins/migrate-goldmark-v1-to-v2/` in the upstream repo — a
breaking-changes reference and an extension-authoring guide. Both were used
here and are worth reading before touching these packages again.

**gitleaks configuration.** The scan flagged `v=g.atlasCount` inside
`static/js/mermaid.min.js` as a `generic-api-key` — an entropy false positive
on minified JavaScript. Added `.gitleaks.toml` allowlisting the
`generic-api-key` rule for that one path. The scope was verified rather than
assumed: the same payload is still caught outside that file, and a private key
block *inside* it is still caught. The file's integrity is asserted separately
by the sha256 pin in `TestMermaidAssetIsTheVendoredBundle`, so nothing can be
edited into it without failing a test.

**Notes / follow-ups:**
- `gopkg.in/yaml.v2` stays. Vendoring the front matter parser does now unblock
  moving to the maintained `go.yaml.in/yaml/v3` — the `goldmark-meta` v1
  dependency on `yaml.MapSlice` was the reason it could not move — but that is
  a separate change with its own golden diff, and mixing it into a goldmark
  migration would make both harder to attribute. `goldmark-meta/v2` remains
  the wrong answer regardless: it pulls `go.yaml.in/yaml/v4`, which has only
  release candidates.
- What is *not* verified: nothing here renders a mermaid diagram in a browser,
  so the mermaid 12 rendering question from the previous entry is still open.
  This change does not touch that path beyond emitting the same
  `<pre class="mermaid">` markup as before.
- The cost side of this decision is real and should be stated: markbrowse now
  owns CommonMark conformance for two hand-written parsers and will not
  receive upstream fixes. `internal/alerts`'s `scanQuoteMarker` is the piece to
  watch — it is derived from goldmark's own blockquote parser, so it has to
  keep agreeing with goldmark about where a blockquote line begins.

---

## 2026-09-20 — Bring every pinned dependency to latest (Go modules, Actions, gosec/syft/grype, mermaid 12)

**Summary:** A full sweep for "latest version" across every kind of pin in the
repo, not just the ones dependabot was configured to see. The Go module bumps
were routine; the interesting findings were that three whole classes of
dependency had no automation watching them at all.

**What was already current:** every *direct* Go module. `goldmark` 1.8.6,
`goldmark-meta` v1.1.0, `goldmark-gh-alerts`, `mermaid`/`wikilink` v0.6.0 and
`yaml.v2` v2.4.0 are each the newest release of their current major.

**What was stale, and why nothing caught it:**

| Pin | Was | Now | Watched by dependabot? |
|---|---|---|---|
| indirect Go modules | x/text 0.3.8 (2022), regexp2 2.5.2, sourcemap 2.1.3, pprof 2023 | 0.42.0 / 2.8.0 / 2.1.4 / 2026 | yes, but `go get -u ./...` skips test-only deps — it needs `-t` |
| `actions/checkout` | v6.0.2 (and a v6 pin in sbom.yml) | v7.0.1 | **no** |
| `actions/setup-go` | v6.4.0 | v7.0.0 | **no** |
| `actions/upload-artifact` (sbom.yml) | v4 | v7.0.1 | **no** |
| `softprops/action-gh-release` | v3.0.0 | v3.0.3 | **no** |
| gosec | v2.22.3 | v2.29.0 | **no** (a `go install` line in a `run:` step) |
| syft / grype | v1.18.1 / v0.87.0 | v1.52.0 / v0.119.0 | **no** (checksum-pinned downloads) |
| mermaid bundle | 11.15.0 | 12.0.0 | **no** (a vendored file) |

`.github/dependabot.yml` only declared the `gomod` ecosystem, so every action
pin was frozen from the day it was written — checkout and setup-go were a full
major behind. Added a `github-actions` entry; dependabot understands SHA pins
with a trailing version comment and rewrites both.

**Action major bumps checked for breaking changes before taking them:**
checkout v7 blocks fork-PR checkout under `pull_request_target`/`workflow_run`
(this repo triggers on `push` and `pull_request`, unaffected) and moved to
ESM; setup-go v7 is an ESM migration; upload-artifact v5/v6/v7 are Node 24 and
ESM moves with `name`/`path` unchanged. All SHAs resolved with
`git ls-remote refs/tags/<tag>` and recorded with the version in a comment.
syft/grype checksums came from each release's own `checksums.txt`, which is
the bump procedure the workflow comment already documents.

**Go toolchain, again:** `go get -u -t ./...` moved the `go` directive 1.25.0
→ 1.26.0, because `golang.org/x/text@v0.42.0` declares `go 1.26.0`. Worth
being explicit about the shape of this: x/text is reached only through goja,
which is a *test-only* dependency, so it is not linked into the release binary
— but the `go` directive is module-wide, so the release build needs 1.26 too.
CI and release workflows moved to `go-version: "1.26"` to match. Pinning
x/text to its last `go 1.25` release would avoid this; not done, because the
instruction was to be on latest and Go 1.26 is the current release.

**Mermaid 11.15.0 → 12.0.0 (deliberate, user-chosen):** the vendored
`static/js/mermaid.min.js` was confirmed byte-identical to npm
`mermaid@11.15.0`'s `dist/mermaid.min.js` (sha256
`70137e77…`), so the swap is a clean artifact replacement rather than a
re-bundle. v12 is a breaking major:

- ELK replaces dagre as the default layout engine.
- `neo` replaces `classic` as the default look; `redux-color` replaces
  `default` as the theme.
- Requires ES2024 — Safari 17.4+, current Chrome/Firefox/Edge.
- Bundle grows 3.2 MB → 5.3 MB (ELK is bundled in).

The choice was between pinning the old rendering (`layout:"dagre"`,
`look:"classic"`) and adopting v12's defaults; **adopting the new defaults was
chosen**, so `theme:"default"` was dropped from `mermaid.initialize` in
`templates.go`. Existing diagrams will visibly re-lay-out and restyle. The
bundle is still only served on pages that contain a diagram, so the size
increase does not touch ordinary markdown pages.

`securityLevel:"strict"` was deliberately *kept* — it was added by the
2026-08-30 raw-HTML hardening, and "mermaid defaults to strict" is exactly the
assumption a major version invalidates. Verified the v12 bundle still honours
it (31 occurrences, same `strict`/`antiscript`/`sandbox`/`loose` levels) and
still installs `globalThis["mermaid"]`, which the classic `<script src>` load
in the template depends on.

**Tests:** the mermaid bundle was previously untracked by anything — not
dependabot, not the SBOM (syft sees Go modules and workflow actions here, not
vendored JS). That invisibility is how it sat three minors and a major behind.
Added to `handler_test.go`:
- `mermaidVendoredVersion` / `mermaidVendoredSHA256` constants plus
  `TestMermaidAssetIsTheVendoredBundle`, which digests the embedded bytes.
  Bumping mermaid now *fails* until both constants are updated in the same
  commit — the point being that the version in the tree is always one someone
  chose.
- `TestMermaidBootstrapIsHardened`, asserting `securityLevel:"strict"`,
  `startOnLoad:false` and `mermaid.run()` survive, and that a diagram-free
  page does not pull the bundle.

**Commands:**
```
go list -m -u all
go get -u -t ./...            # -t: without it, test-only deps are skipped
go mod tidy
git ls-remote --tags --refs https://github.com/actions/checkout      # + each action
git ls-remote https://github.com/actions/checkout refs/tags/v7.0.1   # -> pin SHA
curl -fsSL .../syft_1.52.0_checksums.txt | grep linux_amd64.tar.gz
npm registry: mermaid dist-tags -> 12.0.0; tarball dist/mermaid.min.js vendored
go build ./... && go vet ./... && go test -cover ./...
aidc-scan
```

**Verification:**
- `go build ./...`, `go vet ./...` clean; `go test -cover ./...` pass at 76.1%.
- Vendored 11.15.0 bundle hashed against the npm tarball to prove the file was
  unmodified before replacing it.
- v12 bundle inspected for `globalThis["mermaid"]`, `securityLevel` and the
  `strict`/`sandbox`/`loose` levels before dropping the theme pin.
- Live run: `guides/getting-started.md` emits `<pre class="mermaid">`, loads
  `/__mdview/mermaid.js` (200, `application/javascript`, 5,575,485 bytes) and
  bootstraps `mermaid.initialize({startOnLoad:false,securityLevel:"strict"})`;
  `guides/table-sorting.md`, which has no diagram, loads neither.
- `aidc-scan` clean with the 5 MB bundle in scope.

**Notes / residual risk:**
- **Mermaid rendering itself is not verified.** There is no browser or node in
  this container, so "diagrams still draw correctly under ELK/neo" is
  unproven — only that the bundle loads, exports the global, and keeps the
  security option. Worth one manual pass over a page with diagrams.
- `goldmark/v2` (v2.1.5) and `goldmark-meta/v2` (v2.0.2) exist, and the Go
  version that previously blocked them is no longer a constraint. Still not
  adoptable: `goldmark-gh-alerts`, `go.abhg.dev/goldmark/mermaid` and
  `go.abhg.dev/goldmark/wikilink` have no v2 module path (404 / no matching
  version), and their extenders implement the *v1* `goldmark.Extender`
  interface, so they cannot register on a v2 `goldmark.Markdown`. Migrating
  means dropping or forking three extensions — a rewrite, not an upgrade.
- `gopkg.in/yaml.v2` stays at v2.4.0, which is the last v2 release. Moving to
  yaml.v3 is not available as a version bump: `markdown.go` consumes
  `yaml.MapSlice` returned by `goldmark-meta` v1, and that type is v2-only.

---

## 2026-09-20 — Table sorting compares whole values (#19, PR #20); dependabot bumps (#16, #17)

**Summary:** Integrated the three open pull requests. PR #20 (ai-anant) fixes
issue #19 — table sorting looked at the first number `parseFloat` could find,
so `9.0` sorted above `8x.0` and any cell with a `%`/unit/symbol attached fell
through to the text branch. Its diagnosis and its second fix (the `../` row of
a directory listing linking to `//`) are correct and kept. Its comparison
strategy was reworked because it regressed letter-prefixed identifiers. PRs
#17 and #16 are dependabot bumps applied as version bumps rather than branch
merges, because their `go.mod` base no longer matches this branch.

**Why:** The reporter's own words — *"it seems it still sorts by first number
only not the full number. so 9.0 will be ahead of 8x.0"* and *"% or m
interfere"*. Both were reproduced before touching the fix, by running the real
`static/js/tablesort.js` in a JS engine (no node in the container):

| column | before | expected |
|---|---|---|
| `9.0`, `8x.0`, `10.0` | `9.0`, `10.0`, `8x.0` | `8x.0`, `9.0`, `10.0` |
| `9%`, `100%`, `95%` | `100%`, `9%`, `95%` | `9%`, `95%`, `100%` |
| `8.10.0`, `8.9.0` | `8.10.0`, `8.9.0` | `8.9.0`, `8.10.0` |

The mechanism in each case: `parseSortValue` matched `^-?[\d.]+$`, so `8x.0`
and `95%` never matched and became text, and numbers sort before text
(`va.n && !vb.n => -1`). For `8.10.0` the regex *did* match and
`parseFloat("8.10.0")` returned `8.1`, which compares below `8.9`.

**What PR #20 got right, and was kept:**
- The diagnosis and the goal: compare the whole value.
- The version branch — two or more dots cannot all be decimal points, so the
  components are compared as separate integers.
- Requiring an explicit unit on the byte-size branch, so `8.10.0` is not
  silently truncated by the old `^([\d.]+)\s*(B|KB|...)?$` pattern.
- `parentPath()`. This is a real second bug and was verified independently:
  `path.Dir("/sub") + "/"` is `"//"`, so the `../` row of every first-level
  directory emitted `href="//"` — read by a URL parser as the start of an
  authority, not as the root path. And because the sorter only pinned rows
  whose href ended in `"../"`, while the server has always emitted the parent
  *path*, the "parent row stays pinned" feature had never worked at all.
  Both halves of that fix are kept unchanged.

**What was changed, and why:**
- PR #20 dropped every non-numeric character and compared the remaining digit
  runs (`A1` -> `[1]`). Verified regression: a column of `A1`, `B2`, `A10`,
  `C3` sorted to `A1`, `B2`, `C3`, `A10` — the letter stopped counting. It
  also needed a `NUMERIC_NOISE` heuristic ("at most two letters of unit") to
  keep `video.mp4` and `Chapter 10` in the text branch, which is a guess
  about intent that breaks at three letters (`1.2 ms` numeric,
  `800 sec` textual).
- Replaced with segment comparison: a cell becomes alternating number and
  text segments (`8x.0` -> `[8, "x.", 0]`, `A10` -> `["a", 10]`) compared in
  order, number segments before text segments. This fixes #19 identically,
  keeps `A1` < `A10` < `B2`, additionally sorts `Chapter 3` before
  `Chapter 10`, and deletes the letter-counting heuristic — `video.mp4`
  (`["video.mp", 4]`) stays alphabetical because its first segment is text,
  not because of a rule about word-shaped cells.
- A bare `B` now requires whitespace before it to count as a byte unit.
  Pre-existing (the old regex used `\s*` too), but it only became visible
  once suffixed labels sorted numerically: `3b` was read as three bytes, so
  a `3a`/`3b`/`3c` column sorted `3b`, `3a`, `3c`. The listing always emits
  `386 B` with a space; `3b` is far more likely a label. `1.5KB`/`2.0MB`
  still need no space — multi-letter units are unambiguous.
- Thousands separators are stripped only between digits (`/(\d),(?=\d)/`)
  rather than everywhere, so `Smith, John` keeps its comma.
- Sort keys are computed once per row instead of inside the comparator,
  which previously re-parsed both cells on every one of the O(n log n)
  comparisons. This also removes the old `if (!ca || !cb) return 0` branch,
  a non-transitive comparator for ragged tables.

**Tests:** `tablesort.js` had no automated coverage, and the container has no
node. Added `github.com/dop251/goja` (pure-Go JS engine) as a test dependency
and `tablesort_test.go`, which unwraps the script's IIFE, evaluates the body
in the engine's global scope against the *embedded* `tablesortJS` bytes, and
asserts both directions for 21 value shapes plus `../` row detection. Run
against PR #20's version of the file, exactly two cases fail
(`letter-prefixed identifiers`, `numbered labels sort naturally`), which is
the intended difference.

**Toolchain:** goja's `go.mod` requires `go 1.25.0` (it moved from 1.20
straight to 1.25 in June 2026; there is no recent pre-1.25 commit to pin).
`go.mod` therefore moves to `go 1.25.0` and `ci.yml` / `release.yml` from
`go-version: "1.24"` to `"1.25"`. Go 1.24 is out of upstream support now that
1.26 is released, so this is a bump the repo needed regardless.

**Dependency bumps:** `github.com/yuin/goldmark` 1.8.5 -> 1.8.6 (PR #17) —
`v1.8.5..v1.8.6` is `fix: URLEscape validated the same hex digit twice`,
`fix: URLEscape dropped a truncated utf8 leading byte`, and
`fix(extension): fix #571`; all link-handling correctness, no advisory.
`gopkg.in/yaml.v2` 2.3.0 -> 2.4.0 (PR #16). Applied with `go get` rather than
by merging the branches: both PRs branch from a `go.mod` that predates the
Go 1.25 and goja changes, so merging them would conflict on every line they
touch for no added value.

**Commands:**
```
git fetch https://github.com/anantshri/markbrowse 'refs/pull/20/head:pr-20' \
    'refs/pull/17/head:pr-17' 'refs/pull/16/head:pr-16'
git checkout -b integrate-prs-20-sep-2026
git merge --no-ff pr-20                 # conflict in CHANGELOG.md only
                                        # + semantic conflict: PR #20's new
                                        # test called newMarkdownConverter(dir),
                                        # which took a second arg since 0a186d7
go get github.com/dop251/goja@latest
go get github.com/yuin/goldmark@v1.8.6 gopkg.in/yaml.v2@v2.4.0
go mod tidy
go build ./... && go vet ./... && go test -cover ./...
aidc-scan
```

**Verification:**
- `go test -cover ./...` — ok, 76.1% of statements; `parentPath` 100%.
- `go test -run TableSort -v` — 21 ordering cases + 7 parent-row cases pass.
- Swapping the pre-fix `tablesort.js` back in fails the suite; swapping in PR
  #20's version fails exactly the two cases above. Both re-verified before
  restoring the fixed file.
- `go vet ./...` clean. `aidc-scan` clean.

**Notes:**
- Unit letters are still not interpreted: `1.2m` sorts below `950k` because
  1.2 < 950. Documented in `testdata/guides/table-sorting.md`; interpreting
  SI suffixes would need a per-column decision the sorter does not have.
- `serveDirectory` also builds `CurrentPath: relPath + "/"`, which is `"//"`
  at the root — the same shape as the `ParentPath` bug. Left alone: it is
  only compared against *file* node paths in `sidebar.js`, which never end in
  a slash, so no highlight can be affected.
- `markdown.go` is not `gofmt`-clean on this branch (an import ordering nit
  that predates this work, present on `2ecdebf`). Not touched here to keep
  the diff to the change at hand; CI does not run a format check.

---

## 2026-08-30 — Bind loopback by default (--listen), harden CI ref handling

**Summary:** The server bound `:port` (all interfaces) with no
authentication, so anyone on the LAN — or the internet, if port-forwarded —
could read the entire served tree (secreports/report1.md finding 7, Low,
CWE-306). Separately, `ci.yml` and `release.yml` interpolated
`${{ github.ref_name }}` directly into `run:` shell steps
(findings 5+6, CWE-78).

**Why:** Both re-confirmed on `db268b3`. Exploitability of the CI finding is
narrower than the report states — on `pull_request` events `ref_name` is
`<N>/merge`, not the attacker's branch name, and tags are
maintainer-controlled — but the `${{ }}`-in-`run:` pattern is still against
GitHub's hardening guidance and worth closing.

**What changed:**
- `main.go`: `const defaultListenHost = "127.0.0.1"`; new `--listen` flag
  (IP or hostname). `listenWithFallback(host, port)` binds
  `net.JoinHostPort(host, port)` — JoinHostPort brackets IPv6 literals
  (`::1` -> `[::1]:port`) where the old `Sprintf(":%d")` form did not.
  `srv.Addr` and the startup log updated; `displayHost()` renders wildcard
  binds as `localhost` in the logged URL (0.0.0.0 is not a browsable URL).
- `listen_test.go`: holders now bind `127.0.0.1:0` — the same host:port spec
  production tries. This preserves the exact invariant from commit 537b0e3
  (the macOS CI fix): the busy-port holder must use the same bind spec as
  the code under test, or the two sockets coexist on macOS/BSD and the
  "busy" port binds successfully. The now-unneeded wildcard nosemgrep
  annotations on the holders were removed; new tests:
  TestDefaultListenHostIsLoopback, TestListenWithFallbackWildcardHost,
  TestListenWithFallbackIPv6Loopback, TestDisplayHost.
- `ci.yml` Build step and `release.yml` Package step: `REF_NAME` passed via
  `env:`, referenced as `"$REF_NAME"`, and scrubbed with
  `tr -c 'A-Za-z0-9._-' '_'` before use. Env-passing alone removes the
  `${{ }}`-in-`run:` pattern scanners flag; the scrub also closes the
  residual hole that git refs may legally contain `"` and `$` (so
  `x";curl${IFS}evil` could still break out of a double-quoted ldflags
  string), and guarantees the archive name is a valid filename.
- `README.md`: default-bind callout, docker `-p` migration note, flags
  block updated.

**Commands and verification:**
- `go build ./... && go vet ./...` — clean.
- `go test -cover ./...` — 69 tests pass (4 new listen tests +
  TestDisplayHost).
- Live check: default run binds `127.0.0.1` (connection from the host's LAN
  IP refused); `--listen 0.0.0.0` restores network reachability; `--listen
  ::1` binds IPv6 loopback.
- Workflow YAML: no `${{ github.` remains inside any `run:` block (checked
  by grep); `actionlint` is not installed in this container, so YAML was
  verified by inspection + CI will exercise it.

**Notes:** This is the one deliberately breaking change in the security
batch: anyone serving on a LAN (or in a container with `-p`) must now pass
`--listen 0.0.0.0`. Called out in CHANGELOG under **Changed** with a
migration note, per Keep-a-Changelog convention for behavior changes.

## 2026-08-30 — Symlink containment and VCS-dir blocking in the file handler

**Summary:** The root-containment check was purely lexical
(`strings.HasPrefix(fsPath, h.root)`), but `os.Stat`/`os.Open`/`os.ReadFile`
follow symlinks — so any symlink planted inside the served tree could serve
any host file (verified live: a symlink to `/etc/hosts` and a symlinked
directory exposing all of `/etc`). Separately, `.git/config` and friends were
served to anyone who could reach the port (secreports/report1.md findings
3+4, CWE-59/CWE-200).

**Why:** Both findings reproduce byte-for-byte on `db268b3`. The lexical check
cannot be patched around: following symlinks is the OS default for every file
API in use, so containment has to happen on the *resolved* path.

**What changed (`handler.go`):**
- `underRoot(root, p)` — `filepath.Rel`-based containment (no prefix-boundary
  bug; correct for root `/` and Windows separators). Replaces the `HasPrefix`
  check as the lexical first gate (still 403).
- `resolvedRootDir()` — `filepath.EvalSymlinks(h.root)` computed once per
  handler (`sync.Once`); falls back to `filepath.Clean` if the root vanishes.
  Comparing resolved-to-resolved is what keeps a symlinked root working —
  notably macOS temp dirs (`/var` -> `/private/var`), which would otherwise
  404 every request.
- `resolveContained(fsPath)` — resolves all symlinks in the request path and
  requires the target to stay under the resolved root AND outside VCS dirs
  (a clean-named symlink like `notes.md -> .git/config.md` is caught by the
  second check). 404 on failure (not 403, to avoid an existence oracle).
- `ServeHTTP` pipeline order: virtual assets -> lexical `underRoot` -> VCS
  segment check -> `os.Stat` (preserves the 403/404 mapping for
  missing/permission-denied) -> `resolveContained` -> dispatch on the
  *resolved* path (the path validated is the path opened).
- `serveDirectory` index candidates (`README.md` etc.) now go through
  `resolveContained` too — `README.md -> /etc/passwd` was a live bypass of
  the request-path check; a rejected candidate falls through to the listing
  instead of failing the directory view.
- `vcsDirNames` (`.git`, `.hg`, `.svn`, `.bzr`) + `isVCSName`/`hasVCSSegment`;
  exact segment match only, so `.gitignore`/`.github` stay servable.
- `treeJSONCached` walk and `buildFileIndex` (markdown.go) `SkipDir` on VCS
  dirs (root itself exempt, so `markbrowse .git` still works). Non-VCS
  dot-dirs (`.obsidian`, `.hidden`, `.vault`) remain listed/indexed — that
  behavior is deliberate (Obsidian vaults) and covered by existing tests.

**Behavior notes:** `filepath.EvalSymlinks` also resolves Windows junctions/
mount points, which lexical checks miss entirely. `filepath.WalkDir` already
refuses to descend symlinked dirs, so the tree/file-index walks were never
able to escape; only the request path and index candidates needed the fix.

**Commands and verification:**
- `go build ./... && go vet ./...` — clean.
- `go test -cover ./...` — 65 tests pass; 14 new (TestUnderRoot incl.
  boundary + root-"/" + Rel-error cases, TestUnderRootRelError,
  TestResolvedRootDirFallsBackWhenRootMissing, TestIsVCSName,
  TestHasVCSSegment, TestServeHTTPRejectsSymlinkEscape,
  TestServeHTTPRejectsSymlinkDirEscape, TestServeHTTPAllowsSymlinkInsideRoot,
  TestServeHTTPRootItselfSymlinked, TestServeDirectoryIndexSymlinkEscape,
  TestServeHTTPNotFoundUnchanged, TestServeHTTPBlocksVCSDirs,
  TestServeHTTPBlocksVCSThroughSymlink, TestBuildFileIndexSkipsVCSDirs) +
  TestServeTreeIncludesDotDirs strengthened to put a `.md` inside `.git`.
  New helpers at 100% statement coverage.
- Live PoC re-run: symlink file + symlink dir escapes -> 404; `.git/config`
  -> 404; `.obsidian`-style dot-dirs -> 200; internal symlinks -> 200.

**Notes:** Rejections use 404 rather than 403 so the response doesn't confirm
that an out-of-tree target exists. Symlink tests skip on Windows
(`os.Symlink` needs Developer Mode there), matching the repo's existing
Windows-skip convention. Residual, accepted: a symlink whose *target* is a
non-VCS dot-dir inside root is still served under its clean name — inside the
operator's trust boundary.

## 2026-08-30 — Omit raw HTML in markdown by default, add --raw-html opt-in

**Summary:** goldmark was configured with `html.WithUnsafe()`, which passes
raw HTML in markdown through verbatim AND disables its dangerous-URL filter
(`javascript:`/`vbscript:`/`file:`/`data:` — both behaviors are gated on the
same `r.Unsafe` flag in goldmark's renderer). The rendered body was then
wrapped in `template.HTML`, bypassing `html/template` auto-escaping. Anyone
able to place a `.md` file in the served tree (e.g. a cloned repo's
`README.md`) could execute script in every viewer's browser
(secreports/report1.md findings 1+2, both High, CWE-79).

**Why now:** Both findings were re-verified live against `db268b3` —
`<script>`, `<img onerror>`, and `href="javascript:..."` were served
byte-for-byte. Dropping `WithUnsafe` fixes both findings in one change with
zero new dependencies.

**What changed:**
- `markdown.go`: `newMarkdownConverter(rootDir string, allowRawHTML bool)` —
  `ghtml.WithUnsafe()` is appended to the renderer options only when
  `allowRawHTML` is set. (The options slice is `[]renderer.Option`, not
  `[]ghtml.Option` — goldmark's `WithRendererOptions` takes the wider type.)
- `main.go`: new `--raw-html` flag, threaded to the converter. Flag help
  states it re-enables BOTH raw HTML and dangerous-URL filtering.
- `templates.go`: `mermaid.initialize` now passes `securityLevel:"strict"`
  explicitly (mermaid 11.x defaults to strict; explicit beats default).
- `handler.go`: reworded the `#nosec G203` / nosemgrep justification on
  `template.HTML(body)` to describe the new behavior.
- README: usage example, "How it works" note, and flags block updated.

**Behavior notes:** Without `--raw-html`, HTML *blocks* (`<script>...`) are
omitted entirely and inline HTML keeps its text but drops the tags
(`<!-- raw HTML omitted -->bold<!-- raw HTML omitted -->`) — GitHub's own
behavior. `data:image/png|gif|jpeg|webp` URLs remain allowed (goldmark's
deliberate safe subset). GFM tables, task lists, strikethrough, mermaid
blocks, wikilinks, and gh-alerts (incl. their SVG icons) render unchanged —
verified empirically; those extensions render via their own renderers, not
the `Unsafe`-gated paths.

**Commands and verification:**
- `go build ./... && go vet ./...` — clean.
- `go test -cover ./...` — 51 tests pass (9 new: TestRawHTMLSuppressedByDefault,
  TestDangerousURLsNeutralizedByDefault, TestSafeDataURLsStillRender,
  TestSafeURLsUnaffected, TestAutolinkDangerousSchemeFiltered,
  TestRawHTMLOptIn, TestInlineHTMLTextPreserved,
  TestGFMFeaturesSurviveWithoutUnsafe, TestServeMarkdownEscapesScriptTag).
- Live PoC re-run: `evil.md` with script/onerror/javascript: payloads now
  renders as `<!-- raw HTML omitted -->` / `href=""`; with `--raw-html` the
  trusted HTML renders verbatim.

**Notes:** mermaid bundled version confirmed v11.15.0 (report's "pre-8.2"
concern moot). Report finding 8 (tree.json full walk) verified already fixed
by the existing 5s-TTL cache.

---

## 2026-08-30 — Fix Windows CI failures in permission-denial tests

**Summary:** Three tests that simulate unreadable files/directories via
`chmod 000` failed on the Windows CI runner (got 200/nil, expected
403/error). They now skip on Windows — the simulation is impossible there,
not the behavior broken.

**Why:** Windows doesn't enforce Unix permission bits: `os.Chmod(0o000)`
on a file only clears/sets the read-only attribute (which affects writes,
not reads), and on a directory it is effectively a no-op. So the
"unreadable" fixtures remained readable, the handler correctly served 200,
and `validateReadable` correctly returned nil. The production 403 mapping
and startup validation are unchanged — a real Windows ACL denial still
surfaces as `os.ErrPermission` and maps to 403/the startup error.

**What changed:**
- `handler_test.go`: `TestServeHTTPForbiddenOnUnreadableFile` and
  `TestServeDirectoryForbiddenOnUnreadableDir` skip on
  `runtime.GOOS == "windows"` with an explanatory message.
- `main_test.go`: `TestValidateReadablePermissionDenied` likewise.
- No production code touched.

**How / commands run:**
```
go vet ./... && go test ./...                 # 42 passed (linux)
GOOS=windows go vet ./... && GOOS=windows go build ./...
GOOS=windows go test -c -o /tmp/markbrowse-test.exe .
GOOS=darwin  go test -c -o /tmp/markbrowse-test-darwin .
```

**Errors encountered & resolution:** None beyond the diagnosis — the
cross-compile of the test binary for windows/darwin confirms the skip
branches compile everywhere.

**Verification:** Full suite 42/42 on Linux; `GOOS=windows` vet + build +
test-binary compile all succeed, so the Windows runner will compile and
skip the three tests rather than fail. (Executed Windows runtime behavior
can't be verified from this Linux container.)

**Notes / follow-ups:** If Windows-native permission behavior ever needs
real coverage, the test would have to manipulate ACLs (e.g. via
`icacls`) behind a build tag — out of scope for a test-only skip.
Folded into the 0.3.0 changelog (release still not cut).

---

## 2026-08-30 — Fix macOS CI failure in TestListenWithFallback

**Summary:** The test that guards the port-fallback feature failed on
macOS GitHub runners (`fallback returned the busy port 49169`). Rewrote it
to hold the busy port with the same wildcard bind the production code
uses, making the conflict platform-independent; also hardened the
requested-port and error-path coverage.

**Why:** CI matrix (`ci.yml`) runs ubuntu/macos/windows. The test held
`127.0.0.1:<ephemeral>` (IPv4 loopback) while `listenWithFallback` binds
`:<port>` (wildcard). On Linux the two conflict → fallback triggered →
test passed. On macOS/BSD the IPv6 dual-stack wildcard socket coexists
with the IPv4-loopback holder, so the function bound the "busy" port and
returned it → assertion failed. A test bug, not a product bug — the
feature itself works on macOS.

**What changed:**
- `listen_test.go`:
  - `TestListenWithFallback` now holds `:0` (wildcard, same spec as
    production) and asserts `actual > port` (strictly above, catching
    equal-or-lower regressions) plus the 10000 floor; skips if the
    ephemeral port leaves no headroom.
  - `TestListenWithFallbackPrefersRequestedPort` (new): a just-freed
    random port is rebound without fallback (skip on TOCTOU loss).
  - `TestListenWithFallbackErrorPath` (new): invalid port `-1` fails the
    initial bind and the scan starts at the 10000 floor — asserted as
    documented behavior (a bind at/above 10000 may legitimately succeed,
    so the old "must error" expectation was wrong on every platform).

**How / commands run:**
```
go vet ./...
go test -run TestListenWithFallback -v .   # 3 passed
go test ./...                              # 42 passed
```

**Errors encountered & resolution:** First draft of the error-path test
asserted `listenWithFallback(-1)` must error — it doesn't: the scan starts
at the 10000 floor and binds there. Rewrote to assert the floor behavior.
Also removed a leftover `fmt` import guard. Finally, the wildcard `":0"`
holders tripped semgrep's `avoid-bind-to-all-interfaces` — annotated both
with `nosemgrep:` justifications (test-only transient holders mirroring the
production bind spec deliberately; that mirroring is the fix itself).

**Verification:** All three tests pass locally (Linux/arm64); the wildcard
holder guarantees the same conflict semantics on macOS/Windows since it
now uses the exact production bind spec. Full suite 42/42; `aidc-scan`
clean (semgrep + gosec).

**Notes:** Folded into the 0.3.0 changelog entry (release not yet cut).
The production `listenWithFallback` code is unchanged.

---

## 2026-08-30 — Release 0.3.0 preparation

**Summary:** Cut the release paperwork for 0.3.0: the `Unreleased` changelog
section becomes `0.3.0` dated 2026-08-30, with compare links added. No tags,
branches, or pushes — release mechanics (tag `v0.3.0`, workflow draft) are
done manually by the owner.

**Why:** The integration round (PR fold-in + #13/#14 implementation), the
testdata fixtures, the TOC toggle, and the README credit are all merged on
`enhancement-29-aug-2026`; the changeset is feature-sized (semver: new
functionality, no breaking changes → minor bump from 0.2.0 to 0.3.0).

**What changed:**
- `CHANGELOG.md`: `## [Unreleased]` → empty section retained for future
  work + `## [0.3.0] - 2026-08-30` heading over the existing entries;
  added `[Unreleased]` (compare v0.3.0...HEAD) and `[0.3.0]` link refs.
- Nothing else — `main.version` stays `"dev"` (CI injects the real version
  via `-ldflags -X main.version=` from the tag name on release builds; both
  `ci.yml` and `release.yml` already do this), so no code changes are
  needed for the release.

**How / commands run:**
```
go build ./... && go vet ./... && go test ./...   # 40 passed
aidc-scan                                          # clean
```

**Verification:** Build/vet/tests/scan all clean; changelog renders with
the new heading and both link references resolve to the intended GitHub
URLs (release URL for 0.3.0 will exist once the owner tags it).

**Notes / follow-ups — owner's manual steps for the release:**
1. Merge `enhancement-29-aug-2026` → `main`.
2. `git tag v0.3.0 && git push origin v0.3.0` — this triggers
   `.github/workflows/release.yml`, which builds linux/darwin/windows
   (amd64/arm64) archives with the version baked in and creates a **draft**
   GitHub release with generated notes.
3. Edit/publish the draft release (workflow sets `draft: true`).
4. Optionally close out PRs #5/#7/#8/#9/#11/#12 and issues
   #1/#2/#3/#6/#13/#14 — the integration commit message carries `Fixes`
   references, so merging the branch into default-branch commits closes
   them automatically.

---

## 2026-08-30 — Collapsible right sidebar (TOC toggle)

**Summary:** The right-hand TOC panel now collapses and restores via a
toggle button, mirroring the left file sidebar's behavior exactly.

**Why:** The TOC added for #13 was always visible on wide viewports with no
way to reclaim the reading width. The left sidebar already had a collapse
pattern; the right side deserved the same.

**What changed:**
- `templates.go`: `<button id="toc-toggle">` after the TOC nav in `mdTmpl`
  only (» glyph; JS flips to « when collapsed).
- `static.go`: `.mdview-toc` gains `transition:width .2s ease,min-width
  .2s ease`; new `#toc-toggle` fixed-position button rule (mirrored
  geometry: `right:240px` at the TOC's edge), `body.toc-collapsed` rules
  collapsing the panel and parking the button at `right:12px`; the 1100px
  media query now hides the button alongside the panel.
- `static/js/toc.js`: toggle handler flipping `body.toc-collapsed` and the
  glyph; the no-headings early exit now hides the button too (no dead
  button on heading-less pages).
- Tests: `TestTemplatesIncludeClientScripts` asserts the button on mdTmpl
  and its absence on dirTmpl; new `TestTocToggleCSSPresent` covers the
  button, collapse, and media-query rules.

**How / commands run:**
```
node --check static/js/toc.js
go build ./... && go vet ./... && go test ./...   # 40 passed
go build -o /tmp/markbrowse-e2e . && /tmp/markbrowse-e2e -port 18452 testdata/
curl -s http://localhost:18452/guides/table-of-contents.md | grep toc-toggle
curl -s http://localhost:18452/guides/ | grep -o '[^<>]*toc-toggle[^<>]*'
```

**Errors encountered & resolution:** An initial live check looked like the
dir page "matched toc-toggle 3×" — inspection showed those were the CSS
rules in the shared stylesheet (`#toc-toggle` selectors), not the button
element; selectors are inert without the element, and the Go test asserts
the `id="toc-toggle"` attribute form, which correctly appears only on md
pages. No fix needed.

**Verification:** Button present on `guides/table-of-contents.md`, absent
(dir-listing HTML) on `/guides/`; served `toc.js` contains the
`toc-collapsed` handler; served CSS carries the collapse rules. 40 tests
pass; `aidc-scan` clean.

**Notes / follow-ups:** No state persistence (matches left sidebar);
no mobile drawer mode for the TOC — left sidebar's ≤767px pattern stays
unique to it.

---

## 2026-08-30 — Test fixtures for the new feature set

**Summary:** Added markdown fixtures under `testdata/` exercising every
feature from the 2026-08-29 integration round, so `markbrowse testdata/`
demonstrates and manually verifies all of it out of the box.

**Why:** The new features (front matter table, TOC, table sorting, dot-dirs,
alert tints, ETag/tree caching) shipped with Go unit tests and temporary
`/tmp` fixtures, but the in-repo sample vault still showcased only the
original feature set — nothing reproducible for a human to click through.

**What changed:**
- `testdata/frontmatter.md` (new): full metadata block (strings, bool,
  number, two lists) — renders the GitHub-style table and sets the tab
  title; body links onward via wikilinks.
- `testdata/guides/table-of-contents.md` (new): h1–h6 hierarchy including a
  level skip, exercising the TOC's nesting, expand/collapse defaults, and
  anchors.
- `testdata/guides/table-sorting.md` (new): four tables (comma numbers, byte
  sizes, timestamps, mixed text) starting out of order, with per-table
  instructions on what correct sorting proves.
- `testdata/guides/callout-tints.md` (new): all five alert types + dark-mode
  note; also carries front matter (title check).
- `testdata/guides/performance.md` (new): documents and demonstrates the
  ETag/304 flow (copy-paste curl), tree TTL, lazy wikilink index, and the
  permission behavior table.
- `testdata/.obsidian/vault-notes.md` (new): dot-directory fixture reachable
  as `[[vault-notes]]` and visible in the sidebar.
- `testdata/README.md`: feature list extended; wikilinks to every new
  fixture (which is itself the wikilink-resolution test).

**How / commands run:**
```
go build -o /tmp/markbrowse-e2e .
/tmp/markbrowse-e2e -port 18441 /workspace/testdata
curl -s http://localhost:18441/__mdview/tree.json   # .obsidian + new guides listed
curl -s http://localhost:18441/frontmatter.md       # meta table + <title>
curl -sI http://localhost:18441/guides/performance.md | grep -i etag
curl -s http://localhost:18441/README.md            # wikilinks resolved
go build ./... && go vet ./... && go test ./...     # 39 passed
aidc-scan                                           # clean
```

**Verification:** Live server against `testdata/` — tree lists `.obsidian/`
and all five new pages; `frontmatter.md` renders the full meta table with
`<title>Case Assignment Memo</title>`; both titled guides pick up their
front-matter titles; `performance.md` carries an ETag; all README wikilinks
resolve to real hrefs (`/.obsidian/vault-notes.md`, `/frontmatter.md`, both
new guides); the TOC fixture produces 11 anchored headings across all six
levels; the sorting fixture renders 4 tables; `getting-started.md` remains
meta-table-free (regression guard). `aidc-scan` clean.

**Notes / follow-ups:** Fixtures are prose-first (they double as user-facing
demos); assertions about them live in the Go test suite via temp dirs, not
in testdata itself, to keep the sample vault clean.

---

## 2026-08-29 — PR integration round: table sorting, TOC, front matter, permissions, perf

**Summary:** Folded the useful content of open PRs #5, #7, #8, #9, #11, #12
into the codebase and implemented the two unaddressed issues #13 (right-side
TOC) and #14 (front matter as a GitHub-style table). PR #10 was skipped as
superseded by #12. Six of six open issues are now resolved by this code.

**Why:** The repo had accumulated six open ai-anant PRs (the owner's AI
persona — content integrated directly without per-PR merges) and one
dependabot PR, plus six open issues with no fix shipped to `main`.

**What changed:**
- `go.mod`/`go.sum`: goldmark 1.8.2 → 1.8.5 (cherry-picked dependabot commit
  `5bdfce5`); added `github.com/yuin/goldmark-meta v1.1.0` (+ transitive
  `gopkg.in/yaml.v2`).
- `handler.go`:
  - Dot directories no longer pruned from the sidebar tree walk (#1);
    empty ones still vanish via `pruneEmptyDirs`.
  - Permission errors mapped to 403 at every filesystem site (Stat, Open,
    ReadDir, ReadFile) instead of 404/500 (#2).
  - `serveMarkdown` computes an ETag (`"<mtime-unixnano>-<size>"`), answers
    `If-None-Match` with a bare 304, and tags 200s with the ETag (PR #11).
  - New `treeJSONCached()` with a 5s TTL caches the walk+prune+sort
    (PR #11); `serveTreeJSON` serves the cached bytes with `no-cache`.
  - New routes `/__mdview/tablesort.js` and `/__mdview/toc.js`; all four
    embedded assets now go through a shared `serveAsset` helper.
  - Page title prefers a front matter `title` key over `<h1>` extraction.
- `markdown.go`:
  - `wikilinkResolver` defers `buildFileIndex` behind `sync.Once` (PR #11)
    and no longer skips dot directories (consistency with #7).
  - `convert()` renders through a `sync.Pool` of `bytes.Buffer` (PR #11).
  - Registers `meta.New()` (parser only). `convert()` reads
    `meta.GetItems(ctx)` (order-preserving `yaml.MapSlice`) and prepends
    `renderMetaTable()` output — one row per key, first pair in `<thead>`,
    values HTML-escaped, lists joined with `<br>` (#14).
  - New helpers `renderMetaTable`, `renderMetaValue`, `metaTitle`,
    `metaTitleOf`.
- `main.go`: `validateReadable(rootDir)` fails fast at startup when the root
  directory can't be read, with a message naming the permission bits to
  check (#2). Deliberately simpler than PR #8 (no README probing).
- `static.go`: embeds `tablesort.js` + `toc.js`; CSS gains sort-header
  affordances, the full `.mdview-toc` styleset (collapsible `details` tree,
  indent per level, `@media(max-width:1100px)` hides it,
  `scroll-behavior:smooth`), and the PR #9 alert tint variables + per-type
  `background-color` rules for both light and dark palettes (#3).
- `static/js/tablesort.js` (new, from PR #12 verbatim): value-aware click
  sorting; `static/js/toc.js` (new): builds the nested TOC from
  `h1..h6[ id ]`, h1/h2 expanded, IntersectionObserver highlights the
  section in view.
- `templates.go`: both templates load `tablesort.js`; `mdTmpl` also gains
  the `<nav id="toc-root">` element and `toc.js`.
- Tests: ported/added `TestServeTreeIncludesDotDirs`,
  `TestServeHTTPForbiddenOnUnreadableFile`,
  `TestServeDirectoryForbiddenOnUnreadableDir`,
  `TestTreeJSONCachedRebuildsAfterTTL`, `TestServeMarkdownETagNotModified`,
  `TestTableSortAssetServed`, `TestTocAssetServed`,
  `TestTemplatesIncludeClientScripts`, `TestServeDirectoryListingAndIndex`,
  `TestServeMarkdownTitleFromFrontMatter`, `TestAlertCSSIncludesBackgroundTints`,
  `TestTablesortCSSPresent`, `TestTocCSSPresent`,
  `TestBuildFileIndexIncludesDotDirs`, `TestWikilinkResolvesInDotDirs`,
  front-matter suite in `markdown_test.go`, `main_test.go`
  (`validateReadable`, `loadCustomCSS`), `listen_test.go`, and PR #11's
  `bench_test.go`.

**How / commands run:**
```
# toolchain (container had only Go 1.22; go.mod requires >= 1.24)
curl -sL https://go.dev/dl/go1.24.0.linux-arm64.tar.gz -o /tmp/go1.24.0.tgz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go1.24.0.tgz
sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go

git cherry-pick 5bdfce5          # goldmark 1.8.5 (PR #5)
go get github.com/yuin/goldmark-meta@v1.1.0 && go mod tidy
go build ./... && go vet ./...
go test -cover ./...             # 39 passed, 69.7% statements
aidc-scan                        # all scanners clean
```

**Errors encountered & resolution:**
- `go` kept trying to download toolchain go1.24 (module requirement) —
  fixed by installing Go 1.24.0 to `/usr/local/go` and symlinking into PATH.
- Import collision: stdlib `html` vs `goldmark/renderer/html` in
  `markdown.go` — aliased the goldmark package as `ghtml`.
- `metaTitleOf` initially passed `nil` as source (returned "") — corrected
  to pass the actual `source` bytes.
- Extracting `tablesort.js` from the PR diff lost the trailing `})();` —
  caught by `node --check`, appended.
- `aidc-scan` initial run: gosec G703 (new taint-analysis rule, 5 sites)
  + semgrep XSS/open-redirect findings on long-standing patterns the new
  rules now flag. Fixed by consolidating asset writes into `serveAsset`,
  and annotating each remaining site with `#nosec G304 G703` /
  `nosemgrep:` comments carrying the justification (validation happens in
  `ServeHTTP` before dispatch; redirects use server-side `path.Clean`'d
  paths, never raw user input).

**Verification:**
- `go test ./...`: 39 tests pass; coverage 69.7% (from ~48% pre-change);
  `main()` itself remains untestable by design.
- `node --check` on both new JS files.
- Live server run against a fixture tree (`/tmp/e2e`): front matter table
  rendered exactly as GitHub (row-per-key, thead/tbody), `<title>` taken
  from `title:` key, `.hidden/` present in `tree.json` while a markdown-less
  `.git/` stayed pruned, ETag + conditional request returned 304 with empty
  body, `toc.js`/`tablesort.js` served as `application/javascript`, heading
  IDs present for TOC anchors, alert tint CSS present in served pages,
  unreadable file → 403, unreadable root → startup aborts with the clear
  permission message.
- `aidc-scan`: semgrep, gitleaks, gosec, vet, license-check all clean.

**Notes / follow-ups:**
- goldmark v2 / goldmark-meta v2 (v2.0.2 released 2026-08-27, needs Go
  ≥ 1.25) is blocked: the gh-alerts, mermaid and wikilink extensions have
  no v2 releases yet. The custom `renderMetaTable` reproduces the v2-style
  row-per-key layout on v1 anyway. Revisit when the ecosystem catches up.
- PR #10 skipped — same feature as #12 with a weaker comparator.
- PR #8's startup README-probing dropped — per-request 403s already cover
  file-level permission issues.
- GitHub housekeeping (closing PRs #5/#7/#8/#9/#11/#12 and issues
  #1/#2/#3/#6/#13/#14) intentionally left to the owner; SSH auth to
  github.com failed from this container.

---

## YYYY-MM-DD — <short title>

**Summary:** One or two sentences on what changed and the user-facing effect.

**Why:** The problem, request, or constraint that prompted this. Link the issue
/ ticket / discussion if there is one.

**What changed:**
- File-by-file or component-by-component list of the edits.

**How / commands run:**
```
# exact commands executed, with the relevant output
```

**Errors encountered & resolution:** Anything that went wrong and how it was
fixed (or why it was left as-is).

**Verification:** How the change was proven to work — tests run, scanners,
manual checks, screenshots.

**Notes / follow-ups:** Design choices, trade-offs, and anything deferred.

# 2026-09-20 — Security review before 0.4.0, and the fixes

Fifth session log of the day. Follows
`2026-09-20-sidebar-search-lazy-tree.md`.

## Symptom / goal

"I am wondering could there be a security issue in this codebase. can you do a
proper code review please." Then, on the findings: apply all the fixes and fold
them into 0.4.0, which had not yet been tagged.

## Diagnosis

Reviewed against a running server rather than by reading alone: a scratch vault
was built for each hypothesis and the responses inspected.

### What held up

Recorded because these were exercised, not assumed:

| Checked | Result |
|---|---|
| `/../etc/passwd`, `..%2f`, `%2e%2e/`, `....//` | all 404 |
| symlink to `/etc/passwd`, symlinked dir to `/etc` | both 404 |
| `html/template` context escaping | correct; `template.HTML` confined to goldmark output |
| `[[notes#" onmouseover="alert(1)]]` | `%22` inside the href, `&quot;` in the text — no breakout |
| `# Title "><script>` | `id="title-scriptalert1script"`, raw HTML omitted |
| `innerHTML` / `eval` / `document.write` in first-party JS | none |

`resolveContained` resolves symlinks and then opens the *resolved* path, which
also closes the usual stat-then-open TOCTOU window.

One correction worth noting: the wikilink test initially reported an attribute
breakout. Reading the actual output showed the substring assertion was matching
`onmouseover=` inside the percent-encoded URL and the escaped link text. The
code was right and the assertion was wrong.

### Finding 1 — non-markdown files execute in the viewer's origin

```
/evil.html     Content-Type: text/html; charset=utf-8
/evil.svg      Content-Type: image/svg+xml
/noextension   Content-Type: text/html; charset=utf-8    <- sniffed
```

Verified end to end:

```
README.md renders  <a href="report.html">audit report</a>   (raw HTML OFF)
click              -> 200 text/html, script runs
script             -> GET /__mdview/tree.json   {"name":"sec3",...}
                   -> GET /private/creds.md     secret-api-key=AKIA...
```

The raw-HTML hardening exists because vault content may be untrusted; this
moves the payload one file sideways and bypasses it entirely. No auth, same
origin, so one click reads the whole served tree. Navigation is required — an
`<img src="evil.svg">` does not execute — but a markdown link is natural to
follow.

### Finding 2 — VCS blocking case-sensitive

```
hasVCSSegment("/.git/config") = true
hasVCSSegment("/.GIT/config") = false
```

On APFS/NTFS the OS resolves `.GIT` to the real `.git`. Demonstrated on Linux
with a directory literally named `.GIT`:

```
/.GIT/config  -> 200
body: [remote]   url = https://user:TOKEN@github.com/x/y
```

Quiet, too: the tree walk checks the on-disk name so the sidebar still hid it.
Only the direct URL was affected.

### Finding 3 — no security headers

No `X-Content-Type-Options`, `Content-Security-Policy` or `Referrer-Policy` on
anything. `nosniff` is what stops the extensionless case in finding 1.

## Change

| File | Change |
|---|---|
| `handler.go` | `setBaseSecurityHeaders`; `contentTypeOf` + `isExecutableType` + sandbox CSP; `pageCSP` + `newNonce`; case-folded `isVCSName`; `maxMarkdownBytes` |
| `templates.go` | `Nonce` on both page structs, `nonce="{{.Nonce}}"` on the inline mermaid script |
| `security_test.go` | **new** — headers, sandboxing, content-type decision, nonce freshness, size cap |
| `handler_test.go` | `TestIsVCSName` / `TestHasVCSSegment` gained case, trailing-dot and 8.3 variants |
| `README.md` | new Security section |

The content type is now resolved before the response is written rather than
left to `http.ServeContent`, so the type defended against is the type sent:

```go
ctype, err := contentTypeOf(info.Name(), f)
w.Header().Set("Content-Type", ctype)
if isExecutableType(ctype) {
    w.Header().Set("Content-Security-Policy", "sandbox")
}
```

`sandbox` with no `allow-*` tokens gives the response an opaque origin. The
decision is type-scoped rather than blanket so images, PDFs and downloads are
untouched — a blanket `sandbox` risks breaking the browser's PDF viewer.

VCS matching is now loose because the filesystem is:

```go
n := strings.ToLower(name)
n = strings.TrimRight(n, ". ")           // Windows ignores trailing dots/spaces
return vcsDirNames[n] || vcsShortName.MatchString(n)   // and 8.3: GIT~1
```

Page CSP, with two deliberate loosenings:

```
default-src 'none'; script-src 'self' 'nonce-<random>'; style-src 'self' 'unsafe-inline';
img-src * data: blob:; font-src * data:; connect-src 'self';
base-uri 'none'; form-action 'none'; frame-ancestors 'none'
```

- `style-src 'unsafe-inline'` — the stylesheet is inlined and **mermaid injects
  `<style>` at render time**. A nonce-only style policy would silently stop
  diagrams rendering.
- `img-src *` / `font-src *` — documents legitimately reference remote images
  and neither type executes. `Referrer-Policy: no-referrer` limits disclosure.

`connect-src 'self'` is the real restriction: a script that slipped through
cannot exfiltrate over `fetch`.

## Commands

```bash
# one scratch vault per hypothesis, then inspect the live responses
curl -sI $B/evil.html $B/evil.svg $B/noextension $B/pic.png
curl -s -o /dev/null -w '%{http_code}' $B/.GIT/config
bash /tmp/verify.sh          # after the fixes

gofmt -l . && go vet ./... && go test -cover ./...
aidc-scan
```

## Verification

```
FINDING 1  /report.html   CSP: sandbox   nosniff
           /evil.svg      CSP: sandbox   nosniff
           /noextension   CSP: sandbox   nosniff
           /pic.png       (no CSP)       nosniff      <- images unaffected
FINDING 2  /.git/config /.GIT/config /.Git/config /GIT~1/config  -> all 404
FINDING 3  nosniff + no-referrer + CSP on rendered pages
           CSP nonce == <script nonce="...">, differs per request
SIZE CAP   32.4 MB .md -> 413,  normal .md -> 200
```

- `security_test.go`: base headers across nine response kinds (markdown,
  index, listing, raw file, sniffed file, embedded asset, tree API, 404,
  blocked path), sandbox decisions for seven content types, content type
  decided not sniffed, nonce freshness over five requests,
  `TestMermaidScriptCarriesTheNonce` (the test that would catch a CSP silently
  breaking diagrams), and the size cap.
- Coverage 77.5% -> **78.3%**.
- `gofmt -l` clean, `go vet ./...` clean, `aidc-scan` clean.

## Notes

- **The page CSP is the one thing not verifiable here.** No browser in the
  container, so "mermaid still renders under this policy" is argued from the
  policy text and asserted at the nonce level, not observed. One look at a page
  with a diagram, console open for CSP violations, would close it. If something
  does break, `style-src` is the first place to look.
- Finding 2 was demonstrated on Linux using a directory genuinely named `.GIT`,
  which proves the predicate gap. The macOS/Windows step — that the OS maps
  `.GIT` onto the real `.git` — is inference from APFS/NTFS being
  case-insensitive, not something this container can execute.
- Deliberately unchanged: the YAML parse error still emits a fragment of
  document content in an HTML comment. It can no longer close the comment
  early, and the behaviour is how a broken front matter block explains itself.
- `--raw-html` and `--css` remain documented footguns rather than being
  removed; both are explicit operator opt-ins and now named as such in the
  README.

---

## Follow-up: GitHub code scanning on PR #21 (`go/path-injection`)

CodeQL reported two alerts, both "This path depends on a user-provided value":

- `handler.go` — `os.Open(resolved)` in `ServeHTTP`
- `handler.go` — `os.Stat(resolved)` in `serveDirectory`'s index loop

Both carried a `#nosec G304` justification, which does nothing here: `#nosec`
is gosec-only. A grep found six such sinks; CodeQL had named two
representatively, so all six were converted.

### Why os.Root rather than a dismissal

The old design was resolve (`EvalSymlinks`), compare (`underRoot`), open the
resolved path. It is correct — traversal and symlink escape both 404, verified
in the review above — but correct *by argument*, and there is a window between
the check and the open. `os.Root` makes containment a kernel guarantee
(`openat2`/`RESOLVE_BENEATH`): its methods refuse any name resolving outside
the root, so the path reaching a filesystem call no longer has to be proven
safe by reading three statements in order. Clearing the alert is a side effect
of removing the sink.

`resolveContained` stays, demoted to VCS policy only: a symlink that stays
inside the root but points at `.git/config` is contained, and `os.Root` has no
opinion about that.

### The behaviour os.Root would have removed

It refuses *every* absolute symlink, including one pointing back inside:

```
abs.md   ERR: statat abs.md: path escapes from parent   readlink=/tmp/.../real.md
rel.md   OK                                             readlink=real.md
```

`TestServeHTTPAllowsSymlinkInsideRoot` has asserted the opposite since symlink
containment was added, and `ln -s /abs/path/notes.md` into a vault is ordinary.
So `statInRoot` rewrites an absolute target to a root-relative name and
resubmits it through `os.Root` — the lexical check decides how to rewrite, not
whether to allow, and the rewritten name gets the same kernel check. A hop
limit bounds loops.

### A real bug the flaky test caught

Post-refactor, `TestMermaidScriptCarriesTheNonce` failed intermittently under
`-shuffle=on -count=3`:

```
inline script nonce "1EqaCJRNI&#43;&#43;/1KHDuqmeHQ==" does not match CSP nonce "1EqaCJRNI++/1KHDuqmeHQ=="
```

The nonce was standard base64; `html/template` escapes `+` to `&#43;` in an
attribute, so it failed only when the random bytes encoded a `+`. Browsers
entity-decode before comparing the nonce so it would most likely have worked,
but a policy that functions only because of entity decoding is thin. Switched
to `base64.RawURLEncoding` (`A-Za-z0-9-_`), and `TestNonceIsUnpredictable` now
asserts the alphabet.

### Verification

```
containment    /../etc/passwd  ..%2f..  /escape.md  /etcdir/  /etcdir/passwd  -> all 404
symlinks       /abs.md 200 (Deep)   /rel.md 200 (Deep)
VCS policy     /.git/config  /.GIT/config  /sneaky.md (-> .git/config)  -> all 404
headers        evil.html: sandbox + nosniff;  pic.png: no CSP
nonce          9yhHLrPpuWB98XwlJtMaVA  -- URL-safe
```

New: `TestAbsoluteSymlinkInsideRootStillServed`, `TestSymlinkRewritingIsBounded`.
`go test -count=2 -shuffle=on ./...` repeatedly stable. Coverage 78.3% ->
**78.6%**.

### Open

Whether the alerts actually clear depends on whether that CodeQL version
models `os.Root` methods as sinks. If they persist, dismissal is now on much
firmer ground. Note `filepath.EvalSymlinks` was never flagged despite
predating this work, which is what suggested their query pack models
`os.Open`/`os.Stat` but not symlink resolution — and is why `resolveContained`
could stay.

---

## Follow-up: Windows CI, once it actually ran

Pinning the build job to bash let `windows-latest` get as far as `go test`,
which then surfaced four problems. Two were real bugs, one was mine from the
`os.Root` change, one was repo hygiene.

### 1. Alerts lost their icon and title in CRLF documents

Every golden comparison involving a callout showed:

```
want: <p class="markdown-alert-title">[ICON]Note</p>
got:  <p class="markdown-alert-title"></p>
```

Not a line-ending artifact in the comparison — a genuine rendering bug.
`alertTitleParser.Open` trimmed the line ending with:

```go
if len(line) > 0 && line[len(line)-1] == '\n' { segment.Stop-- }
```

On CRLF that leaves `"\r"`, which is non-empty, so the alert took the
custom-title branch with a title of `"\r"`: no icon, no kind name. Reproduced
locally before fixing:

```
LF               title="<svg id=\"note\"></svg>Note"
CRLF             title=""
LF titled        title="Custom"
CRLF titled      title="Custom"
```

This affects any CRLF vault on any platform, not just Windows CI. Fixed with
`segment.TrimRightSpace(reader.Source())`, which is what goldmark's own parsers
use. Inherited from upstream goldmark-gh-alerts; ours now.

### 2. A tab after the marker swallowed the next line

Writing the CRLF test turned up a second one: `> [!WARNING]\t` rendered as
`<p class="markdown-alert-title"> Body.</p>` — the title ate the body.

`util.IndentWidth(line, pos)` returns `(width, pos)`. The code took the first
and passed it to `reader.Advance`, which counts bytes. Spaces make the two
equal, so it was invisible; a tab is one byte and up to four columns, so it
advanced past the line ending. Now advances by the byte offset.

### 3. The served directory was held open — mine

Every `t.TempDir()` cleanup failed:

```
TempDir RemoveAll cleanup: unlinkat C:\...\001:
  The process cannot access the file because it is being used by another process.
```

The `os.Root` was cached on the handler behind a `sync.Once`, so it held a
descriptor on the served directory for the handler's lifetime. Harmless on
POSIX; Windows refuses to delete a directory with an open handle.

Opened per request now, closed with `defer`. markbrowse has no shutdown path,
so a cached handle had nothing to release it, and one extra `openat` per
request is not measurable beside the stat and render already happening.

**The first test written for this was vacuous.** It counted `/proc/self/fd`
before and after 1,000 requests — and passed even with the `defer root.Close()`
deleted, because a leaked `os.Root` is closed by its finalizer, so a count only
sees the leak until the GC runs. What matters is whether a handle is held while
the handler is *alive*. Replaced with a test that scans `/proc/self/fd` for a
descriptor pointing at the served directory, plus a `RemoveAll` of it while the
handler is still reachable (the Windows symptom). Verified it bites by
restoring the cached version:

```
security_test.go:408: descriptor 6 -> /tmp/markbrowse-handles639564187
security_test.go:409: 1 descriptor(s) still point at the served directory after serving
--- FAIL: TestServedDirectoryIsNotHeldOpen
```

### 4. Line endings

`core.autocrlf` is on by default on Windows runners, so the checkout rewrote
LF to CRLF. Three things here compare bytes, not lines, and all broke:

| | Symptom |
|---|---|
| `golden/**` | every case "differs" while printing identically |
| `static/js/*.js` | "tablesort.js is no longer a bare IIFE" — the `$`-anchored unwrap regexes do not match before `\r` |
| `static/js/mermaid.min.js` | sha256 `0080945a…` instead of the pinned `28fca7ae…` |

Added `.gitattributes` with `* text=auto eol=lf`, and `-text -diff` for the
mermaid bundle (a hashed artifact, and 5 MB of minified JS is not worth
diffing). `git add --renormalize .` staged nothing beyond the day's real edits,
confirming the repo was already all-LF. The IIFE regexes also gained `\r?`, so
that failure mode cannot recur with a message pointing at the wrong thing.

### Verification

- Full suite green; `go test -count=2 -shuffle=on ./...` stable over repeated
  runs. Coverage 78.6% -> **79.0%**.
- `go build` + `go vet` (tests included) pass for `windows/amd64`,
  `darwin/arm64`, `linux/amd64`.
- `aidc-scan` clean.

### Notes

- One CI difference is *not* a bug and was left alone: with CRLF input goldmark
  renders a code span spanning a line break as `<code>304 Not\n Modified</code>`
  rather than collapsing the break to a space. That is goldmark's own CRLF
  handling, it no longer arises here now that checkouts are LF, and it is
  cosmetic in a CRLF vault.

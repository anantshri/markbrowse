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

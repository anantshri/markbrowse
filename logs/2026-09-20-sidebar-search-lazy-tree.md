# 2026-09-20 — Sidebar quick filter and lazy tree rendering (#18)

Fourth session log of the day. Follows
`2026-09-20-goldmark-v2-vendored-extensions.md`.

## Symptom / goal

Issue #18, verbatim:

> The left side folder explorer needs a quick search to find the markdown file
> deep down. quick search should be placed right above the listing and should
> act as a quick filter.

> It seems on a large enough folder the loading time gets too big. we need to
> find a way to lazy load the content but still provide search capability.

Two asks, and the second constrains the first: whatever makes loading lazy must
not take search away.

## Diagnosis

Built a synthetic vault — 4,800 markdown files across 441 directories, three
levels deep — and measured before designing anything.

| | Before |
|---|---|
| `tree.json` build | 14.7 ms cold, **0.5 ms** cached |
| `tree.json` payload | **311 KB** |
| `tree.json` caching | `Cache-Control: no-cache`, **no ETag** |
| DOM elements built on load | **10,923** |

The server was never the bottleneck: the existing 5-second tree cache already
served it in half a millisecond. Two client-side costs, both paid **on every
page navigation**:

1. The full 311 KB was re-downloaded every time, because `no-cache` without a
   validator means "revalidate" but there is nothing to revalidate against.
2. `renderNode` walked the whole tree and built an element for every folder and
   every file regardless of whether it was visible — 10,923 of them. Markdown
   pages already had ETag/304 (`serveMarkdown`), so the tree was the odd one
   out.

Also noticed: `sidebar.js` re-sorted every folder's children on each render,
while `sortTree` in `handler.go` already emits directories first then
alphabetical. Pure duplicated work.

## Change

| File | Change |
|---|---|
| `static/js/sidebar.js` | rewritten: lazy folder rendering + quick filter |
| `handler.go` | `treeJSONCached` returns an ETag; `serveTreeJSON` honours `If-None-Match` |
| `templates.go` | filter input + `aria-live` status line, both templates |
| `static.go` | styles for input, status, match highlight, result rows |
| `sidebar_test.go` | **new** — runs the real script in goja against a stub DOM |
| `handler_test.go` | `TestServeTreeJSONETagNotModified` + ETag assertions |
| `testdata/guides/sidebar-search.md` | **new** demo page |
| `testdata/guides/performance.md`, `README.md` | document both halves |

### Lazy rendering

A folder's children are built by a closure the first time it opens; until then
its container is empty. The one branch rendered eagerly is the one containing
the current page, so the active file is still visible and scrolled into view on
load.

```js
function folderRow(node, depth, open, build) {
  var built = false;
  function populate() { if (built) return; built = true; build(kids); }
  if (open) populate();            // only the current page's ancestors
  btn.addEventListener("click", function () {
    if (btn.classList.contains("collapsed")) populate();
    ...
  });
}
```

The parsed tree stays in memory. That is the whole answer to "lazy load but
still provide search": the filter searches data the DOM has never drawn, with
no extra request and no server-side search endpoint.

### The filter

- **Name by default; path when the query contains `/`.** A bare `guides`
  matching directory names would return every file under `guides/`, which is
  the opposite of finding one file. `guides/table` is the explicit path query.
- **Capped at 200** with "Showing 200 of N matches". A one-character query on a
  large vault matches thousands; rendering them all rebuilds the flat
  equivalent of the whole tree and undoes the lazy rendering.
- **Highlighted** by splitting the name into text nodes around the hit — built
  with `createTextNode`, never `innerHTML`, since the span is derived from what
  the user typed.
- **Debounced 120 ms**, so typing a word renders once, not once per keystroke.
- `Esc` clears.

### ETag

```go
sum := sha256.Sum256(data)
h.treeETag = fmt.Sprintf(`"%x"`, sum[:16])
```

Computed when the tree is rebuilt — at most once per `treeCacheTTL` — not per
request. `no-cache` stays: it means "revalidate", and the revalidation is now
free.

## Commands

```bash
# the measuring rig
for a in $(seq 1 40); do for b in $(seq 1 10); do
  mkdir -p /tmp/bigvault/area-$a/section-$b
  for f in $(seq 1 12); do echo "# Note $f" > /tmp/bigvault/area-$a/section-$b/note-$f.md; done
done; done

curl -sI http://127.0.0.1:18510/__mdview/tree.json
curl -H 'If-None-Match: "<etag>"' -o /dev/null -w '%{http_code} %{size_download}\n' ...

go test -run Sidebar ./...
go test -run TestRenderGolden -update-golden ./...   # new testdata page
gofmt -l . && go vet ./... && go test -cover ./...
aidc-scan
```

## Verification

- **10,923 -> 174 DOM elements** on initial render for the 4,800-file vault.
  Measured by instrumenting `createElement` in the test DOM and running the
  old and new `sidebar.js` against the same tree — not estimated.
- Live: `tree.json` returns an `ETag`; a request carrying it gets `304` with
  **0 bytes**; a request without one gets `200` with 317,988 bytes. The filter
  input is present in both the markdown and directory-listing templates.
- `go test -cover ./...` — pass; main package 76.4% -> **77.3%**.
- Golden files regenerated for the new `sidebar-search.md` and the edited
  `performance.md`; the `[[sidebar-search]]` wikilink resolves to
  `/guides/sidebar-search.md`.
- `gofmt -l` clean, `go vet ./...` clean, `aidc-scan` clean.

## Notes

- `sidebar.js` could not be tested the way `tablesort.js` is. That test
  unwraps the IIFE and calls the helpers directly; `sidebar.js` has a top-level
  `return` for the "no sidebar on this page" case, which is a syntax error
  outside a function. So the test builds a stub DOM (~150 lines) and drives the
  script the way a user does — set the input, fire `input`, flush the debounce,
  inspect what was rendered. Slower to write, but it covers the wiring
  (listeners, debounce, lazy expansion) and not just the logic.
- **Not done: server-side tree pagination.** It would shrink the 311 KB
  further, but it moves search to the server, and the issue explicitly asked
  for lazy loading *without* losing search. The ETag addresses the same cost —
  the payload is now fetched once per change rather than once per navigation —
  without that trade.
- `treeCacheTTL` is unchanged at five seconds, so a new file still takes up to
  five seconds to appear. It also needs the ETag to change, which it does:
  the hash is over the payload.
- Not verified in a real browser — no browser in this container. The script is
  exercised end-to-end under a stub DOM, which covers logic and wiring but not
  layout, focus behaviour or scroll position.

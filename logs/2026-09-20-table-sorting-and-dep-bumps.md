# 2026-09-20 — Table sorting whole-value comparison (#19/#20) and dependency bumps (#16, #17)

## Symptom / goal

Three open PRs and two open issues on `anantshri/markbrowse`:

| # | Kind | Title |
|---|---|---|
| 20 | PR (ai-anant) | fix: compare whole numeric values when sorting tables — closes #19 |
| 17 | PR (dependabot) | Bump `github.com/yuin/goldmark` 1.8.5 → 1.8.6 |
| 16 | PR (dependabot) | Bump `gopkg.in/yaml.v2` 2.3.0 → 2.4.0 |
| 19 | Issue | "The table sorting is still not good." |
| 18 | Issue | "Left sidebar improvements" (quick search + lazy loading) |

Goal: integrate the PRs, but verify issue #19 independently before accepting
PR #20 — the fix was produced by another model, not reviewed by a human.
Issue #18 has no PR attached and is a feature, not a fix; the user chose to
leave it for a separate pass.

## Diagnosis

### Reproducing issue #19

The reporter's words: *"it seems it still sorts by first number only not the
full number. so 9.0 will be ahead of 8x.0"*, and that `%` or `m` interfere.

`static/js/tablesort.js` is a browser IIFE with no test coverage and the
container has no JS runtime (`node`, `deno`, `bun` all absent; `npm` resolves
to a `pmg` shim that reports `PackageManagerNotFound`). Built a throwaway
harness in `/tmp/jsverify` instead: a Go program using `github.com/dop251/goja`
that strips the IIFE wrapper, evaluates the body with a stub `document`, and
sorts sample columns with the file's own comparator. Ran it against the
pre-fix file, PR #20's file, and the final file.

Pre-fix output, confirming both halves of the report:

```
issue #19 example: [9.0 10.0 8x.0]      <- 8x.0 should lead
percent:           [100% 9% 95%]        <- string order
versions:          [1.2.3 8.10.0 8.9.0 10.0.1]
--- parseSortValue ---
  "9.0"          {"v":9,"n":true}
  "8x.0"         {"v":"8x.0","n":false}   <- text branch
  "95%"          {"v":"95%","n":false}    <- text branch
  "8.10.0"       {"v":8.1,"n":true}       <- parseFloat truncation
```

Mechanism: `parseSortValue` only accepted `^-?[\d.]+$` (and a byte-size
pattern), so anything with a letter or symbol became text, and
`va.n && !vb.n => -1` puts every number above every text cell. `8.10.0` *did*
match the digits-and-dots pattern, and `parseFloat("8.10.0") === 8.1`.

### The second bug PR #20 found

`handler.go` built `ParentPath: path.Dir(relPath) + "/"`. For `relPath ==
"/sub"`, `path.Dir` returns `"/"`, so the template emitted
`<a href="//">../</a>` — `//` starts an authority component in a URL, so it is
not the root path. Independently confirmed by reading `templates.go:98` and
`serveDirectory`. And `tablesort.js` pinned rows via
`/\.\.\/$/.test(href)` while the server has *always* emitted the parent path,
never a literal `../` — so "parent row stays pinned" had never worked, at any
version. Both halves of PR #20's fix here are correct.

### Regression in PR #20's comparison strategy

PR #20 follows the issue's literal suggestion ("removing non number characters
from consideration") — it collects every digit run and compares those alone,
guarded by a `NUMERIC_NOISE` heuristic that rejects cells whose leftover
characters contain more than two letters (so `video.mp4` stays textual).

Harness comparison, pre-fix vs PR #20 vs final:

| column | pre-fix | PR #20 | final |
|---|---|---|---|
| `9.0`,`8x.0`,`10.0` | `9.0 10.0 8x.0` | `8x.0 9.0 10.0` ✓ | `8x.0 9.0 10.0` ✓ |
| `9%`,`100%`,`95%` | `100% 9% 95%` | `9% 95% 100%` ✓ | `9% 95% 100%` ✓ |
| `8.10.0`,`8.9.0` | `8.10.0 8.9.0` | `8.9.0 8.10.0` ✓ | `8.9.0 8.10.0` ✓ |
| `A1`,`B2`,`A10`,`C3` | `A1 A10 B2 C3` | `A1 B2 C3 A10` ✗ | `A1 A10 B2 C3` ✓ |
| `Chapter 10/3/1` | `1 10 3` | `1 10 3` | `1 3 10` ✓ |
| `3b`,`3a`,`3c` | `3b 3a 3c` | `3a 3b 3c` ✓ | `3a 3b 3c` ✓ |

So PR #20 fixes the reported cases but makes letter-prefixed identifiers sort
by their number with the letter ignored. The `NUMERIC_NOISE` guard is also a
guess about intent that breaks at the third letter: `1.2 ms` is numeric,
`800 sec` is textual, in the same column.

## Change

| File | Change |
|---|---|
| `static/js/tablesort.js` | Segment-based comparison (below); size unit and comma handling tightened; sort keys computed once per row |
| `handler.go` | `parentPath()` helper from PR #20 (comment reworded) |
| `handler_test.go` | PR #20's `TestServeDirectoryParentRowLink`, fixed for the current `newMarkdownConverter` signature |
| `tablesort_test.go` | **new** — runs the embedded JS in goja, 21 ordering cases + 7 parent-row cases |
| `go.mod`, `go.sum` | +goja (test only); goldmark 1.8.6; yaml.v2 2.4.0; `go 1.24` → `1.25.0` |
| `.github/workflows/{ci,release}.yml` | `go-version: "1.24"` → `"1.25"` (3 sites) |
| `README.md`, `testdata/guides/table-sorting.md` | Describe the actual behavior; demo tables re-verified against the harness |
| `CHANGELOG.md`, `DETAILED_CHANGELOG.md` | Entries |

The comparison change, in place of PR #20's digits-only approach:

```js
// Alternating runs of digits and non-digits. A dot between digits belongs to
// the number ("1.2m" -> 1.2, "m"); anything else is text ("8x.0" -> 8, "x.", 0).
var SEGMENTS = /\d+(?:\.\d+)?|\D+/g;
```

`compareValues` walks the two segment arrays in step: numbers compare
numerically, text compares case-insensitively, a number segment sorts before a
text segment, and the shorter array wins a prefix tie (`1` before `1a`). The
timestamp, byte-size and version branches still short-circuit ahead of it.

`9.0` is `[9]` and `8x.0` is `[8, "x.", 0]`, so 8 < 9 decides it — the same
result the issue asks for, reached without discarding the letters, which is
why `A1` (`["a", 1]`) still sorts below `B2` (`["b", 2]`).

Three smaller corrections on the way:

- `SIZE_WITH_UNIT` now requires whitespace before a bare `B`
  (`/^(\d+(?:\.\d+)?)(?:\s*(KB|MB|GB|TB)|\s+(B))$/i`). The old pattern used
  `\s*` for every unit, so the label `3b` was read as three bytes — invisible
  while such cells were text, visible once they sorted numerically
  (`3b`, `3a`, `3c`).
- Thousands separators are stripped only between digits
  (`/(\d),(?=\d)/g`), so `Smith, John` keeps its comma.
- Keys are parsed once per row rather than inside the comparator, which
  re-parsed both cells on every one of the O(n log n) comparisons. This also
  drops `if (!ca || !cb) return 0`, a non-transitive comparator for ragged
  tables.

## Commands

```bash
# PRs and issues (gh is not installed; git + WebFetch against github.com)
git ls-remote https://github.com/anantshri/markbrowse 'refs/pull/*/head'
git fetch https://github.com/anantshri/markbrowse \
    'refs/pull/20/head:pr-20' 'refs/pull/17/head:pr-17' 'refs/pull/16/head:pr-16'

# investigation harness (outside the repo)
mkdir -p /tmp/jsverify && cd /tmp/jsverify && go mod init jsverify
go get github.com/dop251/goja@latest
go run . tablesort.old.js tablesort.new.js      # pre-fix vs PR #20

# integration
git checkout -b integrate-prs-20-sep-2026
git merge --no-commit --no-ff pr-20
go get github.com/dop251/goja@latest
go get github.com/yuin/goldmark@v1.8.6 gopkg.in/yaml.v2@v2.4.0
go mod tidy
gofmt -l . && go vet ./... && go test -cover ./...
aidc-scan
```

Two problems hit during the merge:

1. `CHANGELOG.md` conflicted (both sides added to `[Unreleased]`) — resolved
   by keeping this branch's `Changed`/`Security` sections and adding `Fixed`
   after them, per Keep a Changelog ordering.
2. A *semantic* conflict git could not see: PR #20's new test calls
   `newMarkdownConverter(dir)`, but commit `0a186d7` on this branch gave that
   function a second `allowRawHTML` parameter. Build failure
   (`handler_test.go:364: not enough arguments`), fixed by passing `false`
   like the other 10 call sites.

## Verification

- `go test -cover ./...` — pass, 76.1% of statements. `go tool cover -func`
  shows `parentPath` at 100%.
- `go test -run TableSort -v` — 21 ordering subtests (each asserting the
  descending order is the exact mirror of ascending) and 7 parent-row
  subtests pass.
- **Tests were checked against the code they are meant to catch.** Swapping
  the pre-fix `tablesort.js` back in fails the suite; swapping in PR #20's
  version fails exactly two cases and nothing else:

  ```
  --- FAIL: TestTableSortOrders/letter-prefixed_identifiers
      ascending = [A1 B2 C3 A10], want [A1 A10 B2 C3]
  --- FAIL: TestTableSortOrders/numbered_labels_sort_naturally
      ascending = [Chapter 1 Chapter 10 Chapter 3], want [Chapter 1 Chapter 3 Chapter 10]
  ```

  The fixed file was restored after each swap and the suite re-run.
- Every ordering claimed in `testdata/guides/table-sorting.md` was run through
  the harness before being written down, including the rewritten
  "Text and Numbers Together" table.
- `go build ./...`, `go vet ./...` clean after the dependency bumps.
- `aidc-scan` clean.

## Notes

- **Toolchain bump was forced, not chosen.** goja's `go.mod` requires
  `go 1.25.0`; its history goes from `go 1.20` (Aug 2024) straight to
  `go 1.25` (Jun 2026), so pinning a pre-1.25 goja would mean a two-year-old
  JS engine that dependabot would immediately try to bump back. Go 1.24 is
  out of upstream support now that 1.26 is out, so `go.mod` and the two
  workflows move to 1.25.
- **Dependabot PRs applied as `go get`, not branch merges.** Both branch from
  a `go.mod` predating the Go 1.25 line and the goja require block, so
  merging them would conflict on every line they touch to reach the same
  two-version result.
- Unit letters are still not interpreted — `1.2m` sorts below `950k` because
  1.2 < 950. Kept and documented; interpreting SI suffixes needs a per-column
  decision the sorter does not have.
- `serveDirectory` also sets `CurrentPath: relPath + "/"`, which is `"//"` at
  the root — the same shape as the `ParentPath` bug. Left alone deliberately:
  `sidebar.js` only compares it against *file* node paths, which never end in
  a slash, so no highlight is affected.
- `markdown.go` is not `gofmt`-clean, and was not before this session
  (verified against `2ecdebf`): `goldmark-meta` sorts before `goldmark/`
  extension imports. Left untouched to keep this diff to the change at hand;
  CI runs `go vet`, not a format check.
- Pushing the branch and closing #19 / PRs #20, #17, #16 on GitHub is left to
  the owner — SSH to github.com fails from this container, and the write
  actions need explicit approval regardless.
- Issue #18 (sidebar quick search + lazy loading) is untouched by design.

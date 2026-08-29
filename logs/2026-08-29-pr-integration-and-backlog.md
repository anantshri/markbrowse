# 2026-08-29 — PR integration and issue backlog clear

## Symptom / goal

The repository had six open PRs from the owner's AI persona (`ai-anant`),
one dependabot PR (#5), and six open issues — four of them covered by PRs
that were never merged, two (#13 TOC, #14 front matter) with no PR at all.
Goal: fold the useful PR content directly into the code (no attribution
required — the persona is the owner's), implement the two missing features,
and leave the issue backlog fully addressed.

## Diagnosis

- Reviewed every PR diff (`/tmp/prs/pr*.diff` fetched from
  `github.com/anantshri/markbrowse/pull/N.diff`).
  - **#7** dot-dirs in sidebar: 1-line fix, but missed the parallel walk in
    `buildFileIndex` (markdown.go) — wikilinks would disagree with the tree.
  - **#8** permission validation: good 403 mapping; startup probing of
    README/index files redundant once per-request 403s exist.
  - **#9** alert tints: pure CSS var + rule additions, exactly GitHub's palette.
  - **#10 vs #12**: both add click-to-sort tables; #12's comparator
    understands byte sizes, timestamps, and comma-numbers → took #12 only.
  - **#11** perf: tree cache (5s TTL), lazy wikilink index, buffer pool,
    ETag/304 — all four taken; its 403 hunks overlap #8 and were merged
    with it.
  - **#5** goldmark 1.8.5: dependabot commit `5bdfce5` already fetched
    locally — cherry-picked rather than re-done.
- Issues: #1→#7, #2→#8, #3→#9, #6→#12 all resolved by the above; #13 and
  #14 needed fresh implementation (user confirmed scope).
- goldmark-meta v2.0.2 exists (2026-08-27) but requires goldmark/v2 + Go
  ≥ 1.25, and gh-alerts/mermaid/wikilink have no v2 releases — adopted
  v1.1.0 parser-only with a custom renderer instead.
- Container had only Go 1.22 (`go.mod` requires ≥ 1.24) — installed Go
  1.24.0 to `/usr/local/go`.

## Change

| File | Change |
|---|---|
| `go.mod`, `go.sum` | goldmark 1.8.2→1.8.5 (cherry-pick `5bdfce5`); +goldmark-meta v1.1.0, +yaml.v2 |
| `handler.go` | dot-dirs in tree walk; 403 mapping at all fs sites; ETag/304 in `serveMarkdown`; `treeJSONCached()` 5s TTL; `tablesort.js`/`toc.js` routes; shared `serveAsset`; title prefers front matter `title` |
| `markdown.go` | lazy wikilink index (`sync.Once`) incl. dot-dirs; buffer pool; `meta.New()` parser; `renderMetaTable`/`renderMetaValue`/`metaTitle`/`metaTitleOf` |
| `main.go` | `validateReadable()` startup check |
| `static.go` | embed tablesort/toc JS; sort-header + TOC CSS; alert tint vars & rules (light+dark) |
| `static/js/tablesort.js` | new (PR #12 verbatim) |
| `static/js/toc.js` | new — nested `<details>` TOC, h1/h2 open, IntersectionObserver highlight |
| `templates.go` | `<nav id="toc-root">` + toc.js in `mdTmpl`; tablesort.js in both templates |
| tests | `handler_test.go` (14 tests), `markdown_test.go` (+7), `static_test.go` (new, 3), `main_test.go` (new, 4), `listen_test.go` (new), `bench_test.go` (new, 3 benchmarks) |

Full narrative in `DETAILED_CHANGELOG.md` entry for 2026-08-29.

## Commands

```bash
# toolchain
curl -sL https://go.dev/dl/go1.24.0.linux-arm64.tar.gz -o /tmp/go1.24.0.tgz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go1.24.0.tgz
sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go

git cherry-pick 5bdfce5                       # PR #5
go get github.com/yuin/goldmark-meta@v1.1.0   # issue #14
go mod tidy
go build ./... && go vet ./...
node --check static/js/tablesort.js static/js/toc.js
go test -cover ./...                          # 39 passed
aidc-scan                                     # clean after annotations
```

## Verification

- `go test -cover ./...` — 39 tests pass, 69.7% statement coverage
  (pre-change ~48%).
- `aidc-scan` — semgrep, gitleaks, gosec, shellcheck, vet, license-check
  all clean. Initial run flagged gosec G703 (5 sites, taint analysis) and
  semgrep XSS/open-redirect: consolidated asset writes into `serveAsset`
  and annotated the genuinely-validated sites with justification comments.
- Live run (`/tmp/markbrowse-e2e -port 18432 /tmp/e2e`):
  - front matter → GitHub-style table (`<thead>` first pair, `<tbody>` rows),
    `<title>` from `title:` key;
  - `tree.json` contains `.hidden/secret.md`, prunes markdown-less `.git/`;
  - `ETag` on 200, `If-None-Match` → 304 empty body;
  - `toc.js`/`tablesort.js` → 200 `application/javascript`; heading IDs present;
  - alert tint CSS in served HTML; chmod-000 file → 403; chmod-000 root →
    startup aborts with permission message.

## Notes

- PR #10 deliberately skipped (superseded by #12).
- goldmark v2 migration blocked on extension ecosystem (no v2 of
  gh-alerts/mermaid/wikilink); custom renderer gives the v2 layout on v1.
- Closing the GitHub PRs/issues left to the owner (SSH to github.com fails
  from this container; and write actions need explicit approval anyway).

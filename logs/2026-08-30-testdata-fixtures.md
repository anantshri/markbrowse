# 2026-08-30 — Test fixtures for the new feature set

## Symptom / goal

The 2026-08-29 integration round shipped front matter tables, the right-side
TOC, table sorting, dot-directory support, alert tints, and caching — all
covered by Go unit tests against temp directories, but the in-repo sample
vault (`testdata/`) still demonstrated only the original features. Goal:
fixtures a human can serve and click through, one per new feature.

## Diagnosis

Mapped each new feature to the fixture that would exercise it:

| Feature | Needs |
|---|---|
| Front matter table + title | metadata block with strings/bool/number/lists |
| TOC | headings across all six levels + a level skip |
| Table sorting | out-of-order tables per value type (numbers, sizes, dates, text) |
| Dot dirs | a `.name/` directory containing markdown, linked via `[[wikilink]]` |
| Alert tints | all five callout types on one page |
| ETag/tree cache/permissions | a page documenting + demonstrating the curl flow |

## Change

- `testdata/frontmatter.md` — new
- `testdata/guides/table-of-contents.md` — new
- `testdata/guides/table-sorting.md` — new (4 tables)
- `testdata/guides/callout-tints.md` — new (also has front matter title)
- `testdata/guides/performance.md` — new (also has front matter title)
- `testdata/.obsidian/vault-notes.md` — new dot-dir fixture
- `testdata/README.md` — extended feature table + wikilinks to all fixtures
  (the links themselves exercise dot-dir + regular wikilink resolution)

No Go code touched.

## Commands

```bash
go build -o /tmp/markbrowse-e2e .
/tmp/markbrowse-e2e -port 18441 /workspace/testdata
curl -s http://localhost:18441/__mdview/tree.json | python3 -m json.tool
curl -s http://localhost:18441/frontmatter.md | grep -o '<table class="meta-table">.*</table>'
curl -s http://localhost:18441/frontmatter.md | grep -o '<title>.*</title>'
curl -sI http://localhost:18441/guides/performance.md | grep -i etag
curl -s http://localhost:18441/README.md | grep -o 'href="[^"]*"'
go test ./...      # 39 passed
aidc-scan          # clean
```

## Verification

- Tree lists `.obsidian/` and all five new pages.
- `frontmatter.md`: full meta table (10 keys incl. bool, number, two
  `<br>`-joined lists) and `<title>Case Assignment Memo</title>`.
- Both guides with `title:` keys render their front-matter titles.
- `performance.md` carries an ETag header.
- All README wikilinks resolve (`/.obsidian/vault-notes.md`, `/frontmatter.md`,
  `/guides/table-sorting.md`, `/guides/table-of-contents.md`).
- TOC fixture: 11 anchored headings spanning h1–h6 with a skip.
- Sorting fixture: 4 tables render.
- `getting-started.md` still has no meta table (no-front-matter regression
  guard).
- `go test ./...` 39 passed; `aidc-scan` clean.

## Notes

Fixtures are prose-first so the sample vault doubles as user-facing
documentation; behavioral assertions stay in the Go test suite (temp dirs),
keeping testdata free of test scaffolding.

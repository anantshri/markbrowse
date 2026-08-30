# 2026-08-30 — Collapsible right sidebar (TOC toggle)

## Symptom / goal

The right-hand TOC (added for #13) was permanently visible on wide
viewports — no way to hide it and reclaim reading width. The left file
sidebar already had a collapse toggle; goal: mirror it for the right side.

## Diagnosis

Left-sidebar pattern already in the codebase:
- button `#sidebar-toggle` in the template,
- handler in `sidebar.js` toggling `body.sidebar-collapsed`,
- CSS: fixed-position button at the sidebar's edge (`left:260px`),
  collapse via `width:0;min-width:0;overflow:hidden;border-…:none` with
  `transition:width .2s ease,min-width .2s ease`, button parks at
  `left:12px` when collapsed.

TOC had none of this — static 240px panel, only a <1100px display cutoff.

## Change

| File | Edit |
|---|---|
| `templates.go` | `<button id="toc-toggle" title="Toggle table of contents">»</button>` after the TOC nav, `mdTmpl` only |
| `static.go` | `transition` on `.mdview-toc`; `#toc-toggle{position:fixed;top:12px;right:240px;…;transition:right .2s ease}`; `body.toc-collapsed .mdview-toc{width:0;…}` + `body.toc-collapsed #toc-toggle{right:12px}`; 1100px media query hides button too |
| `static/js/toc.js` | toggle handler (flips `toc-collapsed` class + »/« glyph); no-headings path hides the button as well |
| `handler_test.go` | mdTmpl has `id="toc-toggle"`, dirTmpl doesn't |
| `static_test.go` | new `TestTocToggleCSSPresent` (button rule, collapse rules, media query, transition) |

## Commands

```bash
node --check static/js/toc.js
go build ./... && go vet ./... && go test ./...          # 40 passed
go build -o /tmp/markbrowse-e2e . && /tmp/markbrowse-e2e -port 18452 testdata/
curl -s http://localhost:18452/guides/table-of-contents.md | grep -o 'id="toc-toggle"[^>]*'
curl -s http://localhost:18452/guides/ | grep -o '[^<>]*toc-toggle[^<>]*'
aidc-scan
```

## Verification

- Markdown page serves the button; directory listing serves only the CSS
  rules (no button element) — the Go test asserts the `id="toc-toggle"`
  attribute form so this stays honest.
- Served `toc.js` contains the `toc-collapsed` handler; served CSS carries
  all collapse rules including the media-query hide.
- 40 tests pass, `aidc-scan` clean.

## Notes

Mirrors the left sidebar exactly (0.2s width animation, button parks 12px
from the edge, arrow indicates collapse direction). No persistence across
pages and no mobile drawer mode — matching the left sidebar's semantics.
Initial "3 matches on dir page" scare was the shared stylesheet's own
selectors, not a leaked button.

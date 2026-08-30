# 2026-08-30 — Omit raw HTML in markdown by default (stored XSS fix)

## Symptom / goal

`secreports/report1.md` findings 1+2 (both High, CWE-79): goldmark was built
with `html.WithUnsafe()` and the rendered body was wrapped in
`template.HTML`, so any `.md` file in the served tree could execute script in
every viewer's browser. Re-verified live against `db268b3`:

```
$ curl -s http://127.0.0.1:18777/evil.md | grep -oE '<script>[^<]*</script>|<img src=x onerror="[^"]*"'
<script>alert(document.cookie)</script>
<img src=x onerror="alert(1)"
$ curl -s http://127.0.0.1:18777/evil.md | grep -oE 'href="javascript:[^"]*"'
href="javascript:alert(document.domain)"
```

Goal: neutralize both findings without breaking GFM/mermaid/alerts/wikilink
rendering, and give operators an explicit opt-back-in.

## Diagnosis

- Root cause is a single flag: goldmark v1.8.5's renderer gates raw HTML
  (`renderer/html/html.go:383,394,647`) AND dangerous URLs
  (`:518,:591,:620` — `if r.Unsafe || !IsDangerousURL(url)`) on the same
  `r.Unsafe` value. `WithUnsafe()` disabled both protections.
- Empirical check (scratch module, same extensions, no `WithUnsafe`):
  `<script>` → `<!-- raw HTML omitted -->`, dangerous hrefs → `href=""`,
  while tables / mermaid / gh-alerts / wikilinks render unchanged — those
  extensions write their output via their own renderers, never the
  `Unsafe`-gated paths (verified in dependency source).
- No existing test asserted raw-HTML pass-through, so flipping the default
  breaks nothing.

## Change

- `markdown.go` — `newMarkdownConverter(rootDir string, allowRawHTML bool)`;
  `ghtml.WithUnsafe()` appended only when `allowRawHTML`. Options slice is
  `[]renderer.Option` (a `[]ghtml.Option` slice won't convert).
- `main.go` — `--raw-html` flag (default false) threaded to the converter.
  Help text says it re-enables both raw HTML and dangerous-URL targets.
- `templates.go` — `securityLevel:"strict"` added to `mermaid.initialize`
  (explicit hardening; 11.x already defaults to strict).
- `handler.go` — reworded the G203/nosemgrep justification on
  `template.HTML(body)`.
- `README.md` — usage example + "How it works" note + flags block.
- ~15 mechanical test call-site updates (`, false`).

## Verification

- `go build ./...`, `go vet ./...` — clean.
- `go test -cover ./...` — 51/51 pass; 9 new tests cover suppressed blocks,
  inline-text preservation, dangerous URL neutralization (links, images,
  autolinks), the safe `data:image/*` subset, unaffected safe schemes, the
  `--raw-html` opt-in, GFM feature survival, and an end-to-end
  ServeHTTP check.
- Live PoC re-run on the rebuilt binary: all three payload classes now
  render as `<!-- raw HTML omitted -->` / `href=""`; `--raw-html` restores
  verbatim rendering for trusted content.

## Notes

- HTML *blocks* are omitted entirely (GitHub behavior); inline HTML keeps
  its text (`<b>bold</b>` → `bold` between omission markers). First draft
  of the handler test wrongly asserted the script's *text* stays visible —
  blocks take their content with them; fixed the assertion to check the
  omission marker instead.
- `data:image/png|gif|jpeg|webp` stays allowed by design (goldmark's
  deliberate safe subset) — pinned by TestSafeDataURLsStillRender.
- Report finding 8 (tree.json walk) verified stale during this session:
  the 5s-TTL cache at handler.go already exists; no change made.

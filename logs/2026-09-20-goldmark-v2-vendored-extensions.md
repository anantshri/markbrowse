# 2026-09-20 — goldmark v2 migration with the extensions vendored into internal/

Third session log of the day, following
`2026-09-20-table-sorting-and-dep-bumps.md` and
`2026-09-20-dependency-refresh.md`.

## Symptom / goal

The dependency refresh reported goldmark v2 as unreachable. The user asked
whether the blocking extensions could instead be subsumed into markbrowse —
kept and maintained in-tree, not published — and whether doing so would reduce
their complexity. Both answers were yes, so the work is: move to
`github.com/yuin/goldmark/v2` and vendor the extensions under `internal/`.

## Diagnosis

### Why this could not be a dependency bump

`github.com/yuin/goldmark/v2` v2.1.5 and `goldmark-meta/v2` v2.0.2 exist. The
other three do not:

```
go list -m go.abhg.dev/goldmark/mermaid/v2@latest    -> 404 Not Found
go list -m go.abhg.dev/goldmark/wikilink/v2@latest   -> 404 Not Found
go list -m github.com/thiagokokada/goldmark-gh-alerts/v2@latest
                                                     -> no matching versions
```

and their extenders implement the v1 `goldmark.Extender` interface, which v2
removed. v1 and v2 can coexist in a module graph, but a v1 extender cannot be
registered on a v2 pipeline, so there is no incremental path.

### Why vendoring shrinks them

markbrowse uses a narrow slice of each:

| Package | Upstream | Vendored | Dropped |
|---|---|---|---|
| mermaid | 1,128 LOC / 17 files | ~150 / 1 | chromedp + mermaid-CLI server rendering, `RenderMode`, CDN script injection, test helpers |
| wikilink | 403 / 6 | ~260 / 1 | `DefaultResolver` (markbrowse has its own) |
| gh-alerts | 385 / 7 | ~290 / 1 | the `Extend` wrapper; two packages merged |
| goldmark-meta | 299 / 1 | ~200 / 1 | table renderer, unordered-map decode |

`internal/` was chosen over a plain package because Go's internal rule makes
the packages un-importable from outside the module — "not available for
others" enforced by the compiler.

### Licences

BSD-3-Clause (wikilink, mermaid — © 2023 Abhinav Gupta) and MIT (gh-alerts —
© 2024 Adam Chovanec; goldmark-meta — © 2019 Yusuke Inuzuka). Both permit
modification and redistribution with the notice retained. Each
`internal/<pkg>/LICENSE` is the upstream file verbatim.

### The v2 API delta

goldmark v2 ships its own migration material at
`.agent-plugins/migrate-goldmark-v1-to-v2/` — a breaking-changes reference and
an extension-authoring guide. Read both first; they cover the pipeline change,
the extension split, the `text.Value`/decoder model, and `Init(n)`.

## Change

Order mattered here: **the golden net was captured on v1 before anything else
moved.**

| Step | Commit |
|---|---|
| 1. Golden baseline on goldmark v1 | `3d33864` |
| 2. Port + migrate | this change |

| File | Change |
|---|---|
| `golden_test.go`, `golden/` | 44 golden files: every `testdata/` document + 12 synthetic cases, both `--raw-html` modes |
| `internal/{wikilink,mermaid,alerts,frontmatter}/` | vendored, ported, trimmed; each with `LICENSE` and a package comment listing divergences |
| `internal/README.md` | why these are here, and the rules for working on them |
| `markdown.go` | v2 pipeline: `parser.New` + `html.New`, extensions split into parser/renderer halves |
| `go.mod` | six runtime modules -> two (`goldmark/v2`, `yaml.v2`) |
| `.gitleaks.toml` | narrow allowlist for the minified mermaid bundle |
| `README.md` | dependency section rewritten with the vendoring table |

### What the port actually required

- `goldmark.New`/`Markdown`/`Extender` are gone — `markdownConverter` holds a
  `parser.Parser` and an `html.Renderer` and drives them. `metaTitleOf` no
  longer renders a throwaway document just to read the parser context.
- Extensions split: each package exposes `NewParser()` and `NewHTMLRenderer()`.
- `RegisterFuncs` -> `html.WithNodeRenderers(map[ast.NodeKind]html.NodeRenderer)`;
  render signature is now `(io.Writer, []byte, ast.Node, bool, renderer.Context)`.
- `Init(n)` in every node constructor.
- `ast.FencedCodeBlock` merged into `ast.CodeBlock` — the mermaid transformer
  checks `CodeBlockKindFenced` explicitly, and `Language()` returns
  `(string, bool)`.
- `ast.TextBlock` removed — both goldmark-meta and the alerts title parser used
  it to stage text for inline parsing; v2 inline-parses a block node's own
  `AppendSource(segment)`, so a node disappeared from each.
- `ast.String` removed; `text.Reader.Value(seg)` -> `seg.Bytes(reader.Source())`;
  `util.URLEscape` lost its second argument.
- Non-string attributes are gone: the alert kind (`[]byte`) and collapsed flag
  (`bool`) became typed struct fields, removing the `t.([]uint8)` assertions.

### Three intentional behaviour changes

```go
// 1. wikilink: upstream kept a sync.Map of "did this node open an <a>".
//    Recomputing on exit is stateless, so one converter can serve concurrent
//    requests. Covered by TestNoStrayClosingTag.
func (r *nodeRenderer) opensAnchor(n *Node) bool { ... }

// 2. frontmatter: YAML error text quotes its input, so it must not be able to
//    close the comment it is written into.
msg := strings.ReplaceAll(n.Message, "-->", "--&gt;")

// 3. alerts: the kind is escaped at the sink instead of fmt.Sprintf'd into
//    the class attribute.
_, _ = bw.Write(util.EscapeHTML([]byte(n.AlertKind)))
```

## Commands

```bash
git checkout -b goldmark-v2-vendor-20-sep-2026
go test -run TestRenderGolden -update-golden     # baseline, still on v1
git commit                                       # 3d33864

go get github.com/yuin/goldmark/v2@v2.1.5
cp "$(go env GOMODCACHE)/<each upstream>/LICENSE" internal/<pkg>/LICENSE
# port the four packages, rewrite markdown.go
go mod tidy
gofmt -l . && go vet ./... && go test -cover ./...
aidc-scan
```

## Verification

- **44 of 44 golden cases byte-identical.** First run differed in exactly one:

  ```
  --- want ---
  <!-- yaml: line 1: did not find expected ',' or ']' -->
  <p>Body after broken front matter.</p>
  --- got ---
  <!-- yaml: line 1: did not find expected ',' or ']' --><p>Body after broken front matter.</p>
  ```

  goldmark v1's `renderTextBlock` writes `\n` on exit when the node has
  children and a next sibling. Porting that rule to the replacement node's
  renderer closed it.
- The pre-existing test suite passed **without a single change**.
- New package tests: alerts 90.9%, frontmatter 94.8%, mermaid 92.7%,
  wikilink 94.9% statement coverage. Main package 76.4%.
- `gofmt -l` clean for everything touched; `go vet ./...` clean.
- Live run on `testdata`: alerts, mermaid containers, wikilinks and the front
  matter table all render; `/guides/getting-started.md` still emits
  `<pre class="mermaid">` and loads the bundle.
- `aidc-scan` clean.

## Notes

- **gitleaks needed a config.** It flagged `v=g.atlasCount` in
  `static/js/mermaid.min.js` as `generic-api-key` — entropy noise on minified
  JS. Added `.gitleaks.toml` allowlisting that rule for that path only, and
  verified the scope rather than assuming it:

  | payload | location | result |
  |---|---|---|
  | random `api_key = "..."` | outside the file | caught |
  | same payload | inside the file | suppressed |
  | RSA private key block | inside the file | still caught |

  A first attempt used the AWS documentation example key, which gitleaks
  allowlists as a known fake, and a 16-byte private key stub too short to trip
  the rule — both produced misleading "not detected" results before the test
  was corrected.
- `gopkg.in/yaml.v2` stays. Vendoring the front matter parser unblocks moving
  to the maintained `go.yaml.in/yaml/v3`, but that is its own change with its
  own golden diff. `goldmark-meta/v2` is still the wrong answer: it depends on
  `go.yaml.in/yaml/v4`, which has only release candidates.
- **The cost is real and should not be soft-pedalled.** markbrowse now owns
  CommonMark conformance for two hand-written parsers and gets no upstream
  fixes. `internal/alerts`'s `scanQuoteMarker` is the piece to watch: it is
  derived from goldmark's own blockquote parser, so it has to keep agreeing
  with goldmark about where a blockquote line starts. If a future goldmark
  release changes blockquote handling, that function is where it will hurt.
- Mermaid *rendering* under v12 is still unverified — no browser here. This
  change does not touch that path; it emits the same markup as before.

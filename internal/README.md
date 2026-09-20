# internal/

Markdown extensions vendored into markbrowse.

These began as third-party goldmark extensions. They were brought in-tree
because markbrowse moved to goldmark v2, whose extension API is not
source-compatible with v1, and none of the three upstreams has a v2 release —
`goldmark-gh-alerts`, `go.abhg.dev/goldmark/mermaid` and
`go.abhg.dev/goldmark/wikilink` have no v2 module path at all, and their
extenders implement the v1 `goldmark.Extender` interface, which v2 removed.

They sit under `internal/` deliberately: Go refuses to let any other module
import a package beneath an `internal/` directory, so these cannot become an
accidental public API. They are markbrowse's code now, not a fork anyone is
expected to consume.

## What is here

| Package | Ported from | License |
|---|---|---|
| `wikilink` | go.abhg.dev/goldmark/wikilink v0.6.0 | BSD-3-Clause, © 2023 Abhinav Gupta |
| `mermaid` | go.abhg.dev/goldmark/mermaid v0.6.0 | BSD-3-Clause, © 2023 Abhinav Gupta |
| `alerts` | github.com/thiagokokada/goldmark-gh-alerts | MIT, © 2024 Adam Chovanec |
| `frontmatter` | github.com/yuin/goldmark-meta v1.1.0 | MIT, © 2019 Yusuke Inuzuka |

Each directory keeps the upstream `LICENSE` verbatim. Both licenses permit
modification and redistribution as long as the copyright notice and licence
text travel with the code, which is what those files are for — do not remove
them.

## Working on these

Every package documents its divergence from upstream in its package comment.
Keep doing that: a reader needs to know which behaviour is inherited and which
is ours, especially when comparing against upstream to pick up a bug fix.

Each was trimmed to what markbrowse actually uses, which is most of the value
of vendoring them. `mermaid` is the clearest case — upstream is ~1,100 lines
because it can also rasterize diagrams at build time through the mermaid CLI or
a headless Chrome; markbrowse only ever renders in the browser, so what remains
is about 150 lines and the chromedp dependency tree is gone. Resist adding the
generality back unless markbrowse needs it.

The rendering contract is covered by the golden files in `golden/`, which are
compared against every markdown document in `testdata/` plus a set of synthetic
cases. A change here that alters output will show up there as a diff. Read it
before regenerating:

    go test -run TestRenderGolden -update-golden

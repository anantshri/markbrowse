package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The markdown pipeline is assembled from a parser, a renderer and four
// extensions, and most of its behaviour is emergent: no single unit test says
// what the HTML for a given document should be. These golden files do. They
// exist so that a change to the pipeline — a goldmark upgrade, an extension
// swap, an option flipped — has to show its effect as a reviewable diff
// instead of passing silently.
//
// Regenerate with:
//
//	go test -run TestRenderGolden -update-golden
//
// and read the resulting diff before committing it.
var updateGolden = flag.Bool("update-golden", false, "rewrite the golden files in golden/")

// syntheticCases cover constructs the demo vault does not, or does not cover
// in every variation: each extension's edge cases and the security-relevant
// raw-HTML and URL-scheme handling.
var syntheticCases = map[string]string{
	"wikilink-forms": `- plain: [[notes]]
- labelled: [[notes|see the notes]]
- fragment: [[notes#heading]]
- labelled fragment: [[notes#heading|jump]]
- embed: ![[notes]]
- unresolved: [[no-such-page]]
- unresolved embed: ![[no-such-image.png]]
- empty target: [[]]
- unterminated: [[notes
`,

	"alert-kinds": `> [!NOTE]
> Highlights information.

> [!TIP]
> Optional extra.

> [!IMPORTANT]
> Crucial.

> [!WARNING]
> Needs attention.

> [!CAUTION]
> Risky.

> [!NOTE] With a custom title
> Body under a titled alert.

> [!NOTE]-
> Collapsed variant.

> Plain blockquote, not an alert.

> [!NOPE]
> Unknown kind falls back to a blockquote.
`,

	"mermaid-block": "Before.\n\n```mermaid\ngraph TD;\n  A-->B;\n```\n\nAfter.\n\n```go\nfmt.Println(\"not mermaid\")\n```\n",

	"frontmatter-table": `---
title: A Title
tags:
  - one
  - two
count: 3
---

Body after front matter.
`,

	"gfm": `| Col | Num |
|---|---:|
| a | 1 |

~~struck~~ and **bold**

- [ ] todo
- [x] done

https://example.com/autolink
`,

	"raw-html": `<div class="trusted">block html</div>

Inline <b>bold</b> and <script>alert(1)</script>.

<img src=x onerror="alert(1)">
`,

	"url-schemes": `- [javascript](javascript:alert(1))
- [vbscript](vbscript:msgbox(1))
- [file](file:///etc/passwd)
- [data html](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)
- [data png](data:image/png;base64,iVBORw0KGgo=)
- [https](https://example.com)
- ![data png img](data:image/png;base64,iVBORw0KGgo=)
`,

	"headings-and-anchors": `# Top Level

## Section One

### Nested & Punctuated!

## Section One
`,
}

func TestRenderGolden(t *testing.T) {
	// Raw HTML is gated by a flag, and the whole point of the safe default is
	// that it changes the output — so both modes get golden files.
	for _, mode := range []struct {
		name         string
		allowRawHTML bool
	}{
		{"safe", false},
		{"rawhtml", true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			md := newMarkdownConverter("testdata", mode.allowRawHTML)

			for name, src := range syntheticCases {
				t.Run(name, func(t *testing.T) {
					got, err := md.convert([]byte(src))
					if err != nil {
						t.Fatalf("convert: %v", err)
					}
					compareGolden(t, filepath.Join("golden", mode.name, "synthetic", name+".html"), got)
				})
			}

			for _, path := range vaultFiles(t) {
				t.Run(path, func(t *testing.T) {
					src, err := os.ReadFile(path) // #nosec G304 -- fixed testdata corpus
					if err != nil {
						t.Fatal(err)
					}
					got, err := md.convert(src)
					if err != nil {
						t.Fatalf("convert: %v", err)
					}
					rel := strings.TrimPrefix(filepath.ToSlash(path), "testdata/")
					compareGolden(t, filepath.Join("golden", mode.name, "vault", rel+".html"), got)
				})
			}
		})
	}
}

// vaultFiles lists every markdown file in the demo vault, so a new fixture is
// picked up by the golden suite without being wired in by hand.
func vaultFiles(t *testing.T) []string {
	t.Helper()

	var out []string
	err := filepath.WalkDir("testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking testdata: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("no markdown found in testdata; the golden suite would pass vacuously")
	}
	return out
}

func compareGolden(t *testing.T, path, got string) {
	t.Helper()

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(path) // #nosec G304 -- path built from the fixture name
	if err != nil {
		t.Fatalf("reading golden (run: go test -run TestRenderGolden -update-golden): %v", err)
	}
	if got != string(want) {
		t.Errorf("rendered output differs from %s\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

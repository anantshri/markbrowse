# Table of Contents Demo

This page exercises the right-hand collapsible TOC. Every heading level from
h1 to h6 appears below; h1/h2 sections start expanded, deeper levels start
collapsed. Clicking an entry scrolls to the section, and the section currently
in view is highlighted as you scroll. Below 1100px viewport width the TOC
hides entirely.

## Level Two Sections

### First h3

Content under the first third-level heading.

#### Nested h4

Deeper nesting, collapsed by default in the TOC.

##### An h5

Fifth level.

###### An h6

Sixth level — the deepest heading goldmark anchors.

### Second h3

Sibling third-level section.

#### Another h4

Another collapsed-by-default entry.

## Another h2 Section

The TOC tree mirrors the document's heading hierarchy exactly; heading levels
that skip (e.g. an h4 directly after an h2) nest under the nearest shallower
ancestor.

### Skipping demo h3

### One more h3

The end of the TOC fixture.

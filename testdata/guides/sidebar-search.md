---
title: Sidebar Search
---

# Sidebar Search

The box above the file tree filters the whole vault, not just the folders that
happen to be open. Type part of a file name and every match appears, however
deep it is buried, with the folders it lives under shown above it.

## What it matches

| Query | Matched against | Example |
|---|---|---|
| no slash | the file name only | `perf` finds `performance.md` |
| contains `/` | the whole path | `guides/table` finds both table guides |

Matching is case-insensitive either way, and the matched characters are
highlighted in the result.

Leaving the slash out on purpose is what keeps short queries useful: if a bare
`guides` matched directory names too, it would return every file in the folder
rather than the one you are looking for.

## Keys

| Key | Does |
|---|---|
| `Esc` | clears the filter and restores the tree |

## Limits

At most 200 matches are rendered. Past that the count line reads *"Showing 200
of 1,432 matches"* — narrow the query rather than scrolling. The cap exists so
a one-character query on a large vault cannot rebuild the entire tree as a flat
list, which is the stall the lazy rendering below is there to avoid.

## Why the tree loads quickly now

The sidebar builds DOM for a folder the first time that folder is opened, not
when the page loads. Only the branch containing the page you are reading is
expanded up front.

On a 4,800-file vault this is the difference between building about 10,900
elements on every page navigation and building about 170:

| | Elements built on load |
|---|---|
| every folder rendered up front | 10,923 |
| rendered on demand | 174 |

The full tree is still fetched and kept in memory — that is what lets the
filter search files it has never drawn. The tree response carries an `ETag`, so
after the first page load the browser revalidates it with a `304` and no body
instead of re-downloading a few hundred kilobytes of JSON on every navigation.

> [!TIP]
> Sorting, callouts and the filter all coexist on this page — a quick check
> that the sidebar scripts and the page scripts do not interfere.

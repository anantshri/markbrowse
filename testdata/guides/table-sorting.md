# Table Sorting Demo

Click any column header to sort the table — first click ascending, second
click descending. The active header shows a ▲/▼ indicator. Sorting is
value-aware: it understands plain numbers, comma-separated numbers, byte
sizes with units, and timestamps. Try each column in the tables below; the
directory listing at the root of this vault sorts the same way.

## Plain and Comma-Separated Numbers

Rows start deliberately out of order so sorting is visible.

| Rank | Population | Score |
|---|---|---|
| 9 | 1,250 | 3.5 |
| 1 | 980,000 | 97.2 |
| 42 | 47 | 100 |
| 3 | 12,400 | 0 |
| 17 | 305,000 | 12.8 |

Click **Population** — `980,000` should sort above `12,400`, proving the
comma separators are understood, not compared as strings.

## Byte Sizes

Sizes with units sort by actual magnitude, so `900 B` < `1.5 KB` < `2 MB`.

| File | Size | Unit-free |
|---|---|---|
| notes.md | 386 B | 100 |
| mermaid.min.js | 3.2 MB | 3 |
| config.yaml | 1.5 KB | 20 |
| archive.zip | 42 B | 400 |
| video.mp4 | 2.0 MB | 2 |

## Timestamps

Directory-listing style timestamps sort chronologically.

| Commit | When | Files changed |
|---|---|---|
| ee9fd42 | 2026-05-28 09:15 | 8 |
| ff61404 | 2026-08-29 11:02 | 2 |
| b7c4437 | 2026-05-30 16:44 | 5 |
| 39ac813 | 2026-06-14 08:30 | 1 |

## Parent Rows Stay Pinned

In the directory listing (navigate up to `/`), the `../` row always stays at
the top regardless of sort column — try sorting by Size or Modified there.

## Mixed Text

Non-numeric cells fall back to case-insensitive string comparison.

| Name | Type |
|---|---|
| zebra | animal |
| Apple | fruit |
| mango | fruit |
| Banana | fruit |
| ant | animal |

> [!TIP]
> A quick sanity check that sorting and alert callouts coexist on the same
> page without interfering.

# Table Sorting Demo

Click any column header to sort the table — first click ascending, second
click descending. The active header shows a ▲/▼ indicator. Sorting is
value-aware: it understands plain numbers, comma-separated numbers, numbers
with `%`/unit/symbol attachments, byte sizes with units, version-like values,
and timestamps. Try each column in the tables below; the
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

## Percent and Attached Units

A cell is sorted by its numbers even when the digits carry `%`, a unit letter,
or a currency symbol, so a column like this no longer falls back to text
ordering (`9%`, `100%`, `95%` as strings) and instead reads
`9%` < `95%` < `100%`.

| Quarter | Coverage | Traffic | Budget |
|---|---|---|---|
| Q1 | 9% | 1.2m | $1,250 |
| Q2 | 100% | 950k | $42 |
| Q3 | 95% | 3.4m | $999 |

Only the digits are compared — the unit is *not* interpreted, so keep units
consistent within a column (`1.2m` sorts below `950k` because 1.2 < 950).

## Version-Like Values

Values with two or more dots are compared component by component, so the whole
version decides the order — `8.10.0` is read as 8, 10, 0 rather than 8.1.

| Release | Version |
|---|---|
| toolkit | 9.0 |
| driver | 8x.0 |
| firmware | 8.10.0m |

Sorting **Version** ascending gives `8x.0`, then `8.10.0m`, then `9.0`. A
single dot is still a decimal point, so `1.10` sorts as 1.1.

## What Stays Textual

Cells whose letters spell a word keep the case-insensitive text sort, so file
names and labels behave like names and not like numbers.

| File | Chapter |
|---|---|
| video.mp4 | Chapter 10 |
| notes.md | Chapter 3 |
| archive.zip | Chapter 1 |

**File** sorts alphabetically (`archive.zip`, `notes.md`, `video.mp4`) — the
`4` in `mp4` does not make it a number, and neither do the digits in
`Chapter 10`.

## Parent Rows Stay Pinned

In a directory listing below the vault root, the `../` row always stays at the
top regardless of sort column — try sorting by Size in `guides/`.

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

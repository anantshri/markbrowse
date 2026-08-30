---
title: Case Assignment Memo
document_id: E001-01
classification: Internal Use Only
prepared_by: Eric Thompson
designation: Chief Information Security Officer
company: ByteBrew Technologies
case_id: BBT-2026-001
draft: false
revision: 3
reviewers:
  - Angela Ruiz
  - Marcus Webb
tags:
  - forensics
  - memo
---

# Case Assignment Memo

This file demonstrates YAML front matter rendering. The metadata block above
renders as a GitHub-style key/value table (one row per key, first pair in the
table header), and the `title` key becomes the browser tab title instead of
the first `<h1>`.

## Scalar Values

String, boolean (`draft: false`), and numeric (`revision: 3`) values render
as plain escaped text.

## List Values

`reviewers` and `tags` are YAML lists; each entry renders on its own line
within the value cell, joined by line breaks.

## No Front Matter

Compare with the other pages in this directory (e.g. [[notes]]), which have
no metadata block and render exactly as before.

> [!NOTE]
> Front matter is optional. Files without a leading `---` block are
> unaffected.

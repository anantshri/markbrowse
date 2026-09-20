---
title: Performance Notes
---

# Performance Notes

Explains the caching behavior added for markdown pages and the sidebar tree,
and how to observe it.

## ETag / Conditional Requests

Every markdown response carries an `ETag` built from the file's modification
time and size. A repeat request with `If-None-Match` returns `304 Not
Modified` with an empty body — the file is neither re-read nor re-rendered.

```console
$ curl -sI http://localhost:8080/guides/performance.md | grep -i etag
etag: "18d047149d4b2d15-fb"

$ curl -s -o /dev/null -w '%{http_code}\n' \
    -H 'If-None-Match: "18d047149d4b2d15-fb"' \
    http://localhost:8080/guides/performance.md
304
```

Touch the file (`touch guides/performance.md`) and the same request returns
`200` again — the tag tracks the file's mtime and size.

## Sidebar Tree Cache

`/__mdview/tree.json` is cached for five seconds. Rapid successive page loads
reuse the cached walk instead of re-scanning the whole tree, so newly created
files can take up to five seconds to appear in the sidebar.

The response also carries an `ETag` derived from the payload, so a browser
revalidating it gets `304` and an empty body whenever the tree has not
changed. The sidebar refetches the tree on every page navigation, and on a
large vault that payload is a few hundred kilobytes.

## Lazy Sidebar Rendering

The sidebar builds DOM for a folder's contents the first time that folder is
opened, rather than for the whole tree on load. Only the branch containing the
page being viewed is expanded up front. On a 4,800-file vault that is about
170 elements per page load instead of about 10,900. See
[[sidebar-search]] for the search that rides on top of it.

## Lazy Wikilink Index

The file index behind `[[wiki links]]` is built once, on the first link that
needs it, rather than at server startup — starting `markbrowse` against a
large tree no longer pays the full walk up front.

## Permission Behavior

Unreadable paths now fail honestly:

| Situation | Before | Now |
|---|---|---|
| Unreadable file | 500 | 403 Forbidden |
| Unreadable directory | 500 | 403 Forbidden |
| Unreadable serve root | server starts, every request fails | startup aborts with a clear message |

> [!WARNING]
> The startup check covers the root directory itself, not every file below
> it; a single unreadable file still serves 403 per-request rather than
> blocking startup.

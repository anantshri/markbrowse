# 2026-08-30 — Symlink containment + VCS-dir blocking (findings 3+4)

## Symptom / goal

`secreports/report1.md` finding 3 (symlink arbitrary read, Medium) and
finding 4 (`.git` exposure, Medium). Both re-verified live against
`db268b3`:

```
$ ln -s /etc/hosts /tmp/mbvault/etc_hosts.txt
$ curl -s http://127.0.0.1:18777/etc_hosts.txt      # → /etc/hosts contents
$ ln -sfn /etc /tmp/mbvault/etcdir
$ curl -s http://127.0.0.1:18777/etcdir/hostname    # → hostname (whole /etc browsable)
$ curl -s http://127.0.0.1:18777/.git/config       # → [core] ... secret=abc123
```

Plain `..` traversal was already blocked (404) — the gap was symlinks and
VCS dirs, not lexical traversal.

## Diagnosis

- Containment was `strings.HasPrefix(fsPath, h.root)` — lexical only. Every
  file API in use (`os.Stat`, `os.Open`, `os.ReadFile`, `http.ServeContent`)
  follows symlinks, so a symlink inside the tree defeats the check.
- The `HasPrefix` form also lacks a separator boundary (`/tmp/vault` matches
  `/tmp/vault-x`), though `path.Clean("/"+path)` makes that unreachable via
  URLs today — hardened anyway.
- `.git` was served because `ServeHTTP` serves every path under root; only
  the sidebar skipped dot-entries (and recent changes re-added dot-dirs
  deliberately for Obsidian vaults, so a blanket dotfile ban would break the
  product — tests assert `.hidden`/`.vault` visibility).
- Hidden bypass found during planning: `serveDirectory`'s index-candidate
  loop joined `README.md` onto the already-checked `fsPath`, but the
  candidate itself could be a symlink (`README.md -> /etc/passwd`) — lexical
  join looks clean, target is not.

## Change

`handler.go`:
- `underRoot(root, p)` — `filepath.Rel`-based containment; replaces
  `HasPrefix` as the lexical gate (still 403).
- `resolvedRootDir()` — `EvalSymlinks(h.root)` once per handler via
  `sync.Once` (fallback `filepath.Clean`). Resolved-to-resolved comparison is
  what keeps a symlinked root working — critical on macOS where
  `t.TempDir()` sits behind `/var -> /private/var`; comparing against the
  unresolved root would 404 every request there.
- `resolveContained(fsPath)` — `EvalSymlinks` the full request path;
  require under resolved root AND no VCS segment in the resolved-relative
  path (catches `notes.md -> .git/config.md`). 404 on failure — not 403, to
  avoid an existence oracle.
- `ServeHTTP` order: virtual assets → lexical `underRoot` → VCS segment →
  `os.Stat` (preserves 403/404 mapping) → `resolveContained` → dispatch on
  the resolved path.
- `serveDirectory` index candidates go through `resolveContained`; rejected
  candidates fall through to the listing (a hostile symlink must not break
  browsing).
- `vcsDirNames` = `.git/.hg/.svn/.bzr`; exact segment match only
  (`.gitignore`, `.github` unaffected). `treeJSONCached` walk and
  `buildFileIndex` (markdown.go) `SkipDir` on them; root itself exempt so
  `markbrowse .git` keeps working.

## Verification

- `go build ./...`, `go vet ./...` clean; `go test -cover ./...` 65/65.
- 14 new tests + strengthened `TestServeTreeIncludesDotDirs` (a `.md` inside
  `.git` now exercises the SkipDir branch). New helpers at 100% statement
  coverage, including the `filepath.Rel` error branch and the
  missing-root `EvalSymlinks` fallback.
- Symlink tests skip on Windows (`os.Symlink` needs Developer Mode), matching
  the repo convention; Windows backslash cases in `TestUnderRoot` are
  GOOS-guarded (they'd false-fail on POSIX where `\` is not a separator).
- Live re-run: symlink file/dir escapes → 404; `/etc` via symlinked dir →
  404; `.git/config` → 404; `.obsidian/x.md` → 200; internal symlink → 200;
  missing file → 404 unchanged.

## Notes

- `filepath.EvalSymlinks` also resolves Windows junctions/mount points —
  lexical checks miss those entirely.
- `filepath.WalkDir` never descended symlinked dirs, so the tree/index walks
  were never the escape vector; only request paths and index candidates
  needed the guard.
- Mid-implementation misstep worth recording: the first cut of this branch
  was created off `main` instead of the Fix A branch, so it missed the
  `newMarkdownConverter(dir, bool)` signature and failed to build. Re-cut
  the branch stacked on `security-raw-html-30-aug-2026`.
- Residual, accepted: a symlink whose target is a non-VCS dot-dir inside
  root still serves under its clean name — within the operator's trust
  boundary, consistent with the deliberate dot-dir indexing decision.

# Security

How markbrowse is meant to be exposed, what it defends against, and what CI
checks. `.github/workflows/sbom.yml` refers here for the SBOM and license
pipeline's contract.

## Threat model

markbrowse renders a directory of markdown over HTTP **without authentication**.
That is the point of the tool, and every other decision follows from it: the
server is only safe to reach from a place you already trust.

- The default bind is `127.0.0.1` (`defaultListenHost` in `main.go`). `--listen`
  is the explicit opt-in to a wider audience, and passing `--listen 0.0.0.0`
  means anyone who can reach the port can read every file the server exposes.
- There is no user model, no session, and no CSRF token, because there is no
  identity to protect. Do not put this behind a reverse proxy and assume it is
  now multi-user.
- The content being served is assumed to be yours. `--raw-html` exists for
  documents you trust and re-enables raw HTML plus `javascript:`/`data:` URLs;
  leaving it off is the default for a reason.

## Containment

Every file access below the served directory goes through `os.Root`
(`openRoot` in `handler.go`), so containment is enforced by the kernel —
`openat2`/`RESOLVE_BENEATH` on Linux — rather than by a check the code performs
and then hopes still holds. That closes the stat-then-open race structurally.
`os.Root` refuses absolute symlinks outright, so `statInRoot` translates a link
whose target is inside the tree back into a root-relative name and resubmits it;
the rewritten name still goes through `os.Root`, so a target that is not
genuinely inside still fails.

Repository metadata directories (`.git`, `.hg`, `.svn`, `.bzr`) are never
served, listed, or indexed, and the check is deliberately loose
(`isVCSName`) because filesystems are: APFS and NTFS are case-insensitive, so
`/.GIT/config` opens the real `.git/config`, Windows ignores trailing dots and
spaces, and 8.3 aliases expose `GIT~1`. The policy is applied to the *resolved*
target as well (`resolveContained`), so a clean-looking symlink that lands in
`.git` is still refused.

## Rendering

- Raw HTML in markdown is omitted by default, and `javascript:`, `data:`,
  `file:` and `vbscript:` URLs are filtered. `--raw-html` opts back in.
- Rendered pages carry a Content-Security-Policy with a fresh per-request nonce
  (`pageCSP`, `newNonce`). `style-src` keeps `'unsafe-inline'` deliberately,
  because the stylesheet is inlined and mermaid injects `<style>` at render time;
  `connect-src 'self'` limits exfiltration if a script ever does slip through.
- Non-markdown files whose type a browser would execute (`text/html`,
  `image/svg+xml`, XML/XSLT) are served with `Content-Security-Policy: sandbox`,
  which puts them in an opaque origin with no scripts and no same-origin reads.
  A `.html` next to the markdown would otherwise run in the viewer's origin with
  read access to the whole served tree.
- `X-Content-Type-Options: nosniff` and `Referrer-Policy: no-referrer` are set
  on every response.
- Markdown rendering is capped at 32 MiB (`maxMarkdownBytes`) because rendering
  is linear in file size and happens per request.

## What CI checks

`.github/workflows/ci.yml`

- Build, `go vet` and `go test` on Linux, macOS and Windows.
- `gosec ./...` in a separate job. The project policy is zero findings across
  all targets, fixed in code — not suppressed. The `#nosec` and `nosemgrep`
  markers in the source each carry a written justification and are reviewed, not
  blanket-applied.

`.github/workflows/sbom.yml`

- `scripts/ci/aidc-sbom-all.sh` — code-level SBOM, optional build-time SBOM,
  code-vs-build diff, and a license-conflict check. The same scripts run
  unchanged under Jenkins or GitLab CI, so the workflow is a thin wrapper.
- syft and grype are installed from pinned release artifacts verified against
  pinned sha256 sums, not from an unpinned installer script.

Not yet wired into CI: secret scanning. `gitleaks` is the intended tool; until
it runs as a job, treat a leaked credential as something CI would not have
caught.

## SBOM and license pipeline contract

All configuration is by environment variable, and the exit code is the only
result CI needs to read.

| Variable | Meaning |
|---|---|
| `AIDC_SBOM_DIR` | Output directory for the SBOM artifacts. |
| `AIDC_SBOM_SRC` | Source tree to catalog (code-level SBOM). |
| `AIDC_IMAGE_REF` | Image reference for the build-time SBOM. Empty skips that stage. |
| `AIDC_PROJECT_LICENSE` | The license the project itself is under. |
| `AIDC_LICENSE_MODE` | `warn` (default) or `fail`. `fail` makes a conflict non-zero. |
| `AIDC_LICENSE_SBOM` | SPDX SBOM to read licenses from; defaults to the code SPDX output. |
| `AIDC_LICENSE_MATRIX` | Path to the allow/deny license matrix. |
| `AIDC_LICENSE_USE_VET` | Use `vet` for license data where available. |

Exit codes: `0` success, `1` policy violation (license conflict in `fail` mode),
`2` missing tool or usage error. Requirements are only `bash` and `jq`, plus
`syft`/`vet` for the stages that use them. The scripts are Bash 3.2-safe so they
run under macOS's system bash.

Entry point: `scripts/ci/aidc-sbom-all.sh`. Shared helpers and the exact
semantics live in `scripts/ci/aidc-lib-common.sh`.

## Reporting a problem

Open an issue at https://github.com/anantshri/markbrowse/issues. For anything
that looks exploitable rather than merely wrong, say so in the issue title and
avoid posting a working exploit until there has been a chance to respond.

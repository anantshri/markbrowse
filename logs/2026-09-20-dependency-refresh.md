# 2026-09-20 — Bring every pinned dependency to latest

Follows `logs/2026-09-20-table-sorting-and-dep-bumps.md` in the same session.

## Symptom / goal

"Ensure we are on the latest versions of all packages." Taken to mean every
kind of pin in the repo, not only the ones `go.mod` holds: Go modules (direct
and indirect), the SHA-pinned GitHub Actions, the tool versions the workflows
install (gosec, syft, grype), and the vendored `static/js/mermaid.min.js`.

## Diagnosis

`go list -m -u all` over the build list showed all **direct** Go modules
already at the newest release of their current major. The stale things were
everywhere dependabot was not looking:

| Pin | Was | Latest | Why it drifted |
|---|---|---|---|
| `golang.org/x/text` | 0.3.8 (2022) | 0.42.0 | indirect, test-only path |
| `dlclark/regexp2/v2` | 2.5.2 | 2.8.0 | same |
| `go-sourcemap/sourcemap` | 2.1.3 | 2.1.4 | same |
| `google/pprof` | 2023-02 | 2026-09 | same |
| `actions/checkout` | v6.0.2 / v6 | v7.0.1 | no `github-actions` ecosystem in dependabot.yml |
| `actions/setup-go` | v6.4.0 | v7.0.0 | same |
| `actions/upload-artifact` (sbom.yml) | v4 | v7.0.1 | same |
| `softprops/action-gh-release` | v3.0.0 | v3.0.3 | same |
| gosec | v2.22.3 | v2.29.0 | a `go install` line inside a `run:` step |
| syft / grype | v1.18.1 / v0.87.0 | v1.52.0 / v0.119.0 | checksum-pinned curl downloads |
| mermaid bundle | 11.15.0 | 12.0.0 | a vendored file, invisible to dependabot *and* to the SBOM |

Two root causes worth naming:

1. **`.github/dependabot.yml` only declared `gomod`.** Every action pin was
   frozen at whatever it was written as. Fixed by adding a `github-actions`
   entry — dependabot handles SHA pins with a trailing version comment.
2. **`go get -u ./...` silently did nothing.** It only walks non-test imports,
   and the stale indirect modules are reached through `goja`, which is a
   test-only dependency. `go get -u -t ./...` is what moved them. Noted because
   the first command *succeeded with no output*, which reads as "already
   current".

### Checks made before accepting the major bumps

- **checkout v7**: blocks fork-PR checkout under `pull_request_target` /
  `workflow_run`; this repo triggers on `push` and `pull_request`, so
  unaffected. Rest is an ESM migration.
- **setup-go v7**: ESM migration.
- **upload-artifact v5/v6/v7**: Node 24 runtime and ESM; `name` / `path`
  inputs unchanged, which is all sbom.yml uses.
- **mermaid 12**: genuinely breaking — see below.

### goldmark v2: still blocked

`github.com/yuin/goldmark/v2` v2.1.5 and `goldmark-meta/v2` v2.0.2 exist, and
the Go version that blocked them on 2026-08-29 no longer does. But
`goldmark-gh-alerts`, `go.abhg.dev/goldmark/mermaid` and
`.../wikilink` have no v2 module path at all (404 / "no matching versions"),
and their extenders implement the **v1** `goldmark.Extender` interface, so
they cannot be registered on a v2 `goldmark.Markdown`. Migrating means
dropping or forking three extensions. Left on v1 deliberately.

Likewise `gopkg.in/yaml.v2` v2.4.0 is the last v2 release, and moving to v3 is
not a version bump: `markdown.go` handles the `yaml.MapSlice` that
`goldmark-meta` v1 returns, and that type only exists in v2.

## Change

| File | Change |
|---|---|
| `go.mod`, `go.sum` | `go get -u -t ./...`; `go` directive 1.25.0 → 1.26.0 |
| `.github/workflows/{ci,release,sbom}.yml` | action SHAs re-pinned; `go-version` → `"1.26"`; gosec → v2.29.0; syft/grype versions + sha256 |
| `.github/dependabot.yml` | + `github-actions` ecosystem |
| `static/js/mermaid.min.js` | 11.15.0 → 12.0.0 (3.2 MB → 5.3 MB) |
| `templates.go` | `theme:"default"` dropped from `mermaid.initialize` |
| `handler_test.go` | + `TestMermaidBootstrapIsHardened`, + `TestMermaidAssetIsTheVendoredBundle` with version/sha256 constants |
| `README.md` | mermaid line notes v12 and the ES2024 browser floor |

### Go toolchain 1.25 → 1.26

`golang.org/x/text@v0.42.0` declares `go 1.26.0`, which raised the module's
own directive. The shape of this is worth stating plainly: x/text is reached
only through goja, goja is test-only, so x/text is **not** linked into the
release binary — but the `go` directive is module-wide, so the release build
needs 1.26 anyway. Pinning x/text to its last `go 1.25` release would avoid
it; not done, because the instruction was "latest" and 1.26 is current.

### Mermaid 11.15.0 → 12.0.0

First established the vendored file was an unmodified artifact, so the swap
would be a clean replacement rather than a re-bundle:

```
npm mermaid@11.15.0 dist/mermaid.min.js  sha256 70137e77bb273bb2…
static/js/mermaid.min.js                 sha256 70137e77bb273bb2…   identical
```

v12 breaking changes: ELK replaces dagre as the default layout engine, `neo`
replaces `classic` as the look, `redux-color` replaces `default` as the theme,
ES2024 is required (Safari 17.4+), and the bundle grows by ~2 MB because ELK
is bundled in. Offered as a choice between pinning the old rendering
(`layout:"dagre", look:"classic"`) and adopting the new defaults; **new
defaults chosen**, so `theme:"default"` came out of `mermaid.initialize`:

```diff
-<script>mermaid.initialize({startOnLoad:false,theme:"default",securityLevel:"strict"});mermaid.run();</script>
+<script>mermaid.initialize({startOnLoad:false,securityLevel:"strict"});mermaid.run();</script>
```

`securityLevel:"strict"` deliberately stayed. It was added by the 2026-08-30
raw-HTML hardening on the reasoning "mermaid 11.x defaults to strict; explicit
beats default" — and a major version is exactly what invalidates that kind of
assumption. Confirmed v12 still honours it before touching the line:

```
securityLevel occurrences in v12 bundle: 31
levels present: "strict" "antiscript" "sandbox" "loose"
global export:  globalThis["mermaid"] = globalThis.__esbuild_esm_mermaid_nm["mermaid"].default;
```

That last line matters because the template loads the bundle as a classic
`<script src>` and calls into a global.

## Commands

```bash
go list -m -u all
go get -u ./...          # no-op: skips test-only deps
go get -u -t ./...       # -t is what moved x/text, regexp2, sourcemap, pprof
go mod tidy

# action SHAs
git ls-remote --tags --refs https://github.com/actions/checkout | ... | sort -V | tail
git ls-remote https://github.com/actions/checkout refs/tags/v7.0.1

# tool checksums, per the procedure documented in sbom.yml itself
curl -fsSL https://github.com/anchore/syft/releases/download/v1.52.0/syft_1.52.0_checksums.txt \
  | grep linux_amd64.tar.gz
curl -fsSL https://github.com/anchore/grype/releases/download/v0.119.0/grype_0.119.0_checksums.txt \
  | grep linux_amd64.tar.gz

# mermaid
curl -s https://registry.npmjs.org/mermaid | python3 -c '...dist-tags...'   # 12.0.0
# pull the tarball, extract package/dist/mermaid.min.js, sha256sum, copy in

gofmt -l . && go build ./... && go vet ./... && go test -cover ./...
aidc-scan
```

Note the `go get -u ./...` line: it printed nothing and changed nothing, which
looks like "already up to date". It is not — it is the missing `-t`.

## Verification

- `go build ./...`, `go vet ./...` clean. `go test -cover ./...` pass, 76.1%.
- `go test -run Mermaid -v` — both new tests pass.
- Old bundle hashed against the npm 11.15.0 tarball before replacing it
  (identical), so nothing local had been patched into it.
- New bundle checked for `globalThis["mermaid"]`, `securityLevel` and the four
  security levels *before* the theme pin was dropped.
- Live run on `testdata`:
  - `guides/getting-started.md` → `<pre class="mermaid">`, loads
    `/__mdview/mermaid.js`, bootstraps
    `mermaid.initialize({startOnLoad:false,securityLevel:"strict"})`;
  - `guides/table-sorting.md` (no diagram) loads neither;
  - `GET /__mdview/mermaid.js` → 200 `application/javascript` 5,575,485 bytes.
- `aidc-scan` clean with the 5 MB bundle in scope (semgrep, gitleaks, gosec,
  dependency vet, license gate).

## Notes

- **Mermaid rendering is not verified.** No browser and no node in this
  container, so what is proven is that the bundle loads, exports its global,
  and keeps `securityLevel`. That diagrams *draw correctly* under ELK + neo is
  not proven and needs one manual pass over a page with diagrams. This is the
  one residual risk in the whole refresh.
- The mermaid bundle is now pinned by sha256 in `handler_test.go`. It was the
  only dependency in the repo with no automation whatsoever watching it, which
  is precisely how it fell three minors and a major behind. Bumping it now
  fails a test until the recorded version and digest move with the file.
- `markdown.go` remains not `gofmt`-clean (pre-existing, see the previous log).

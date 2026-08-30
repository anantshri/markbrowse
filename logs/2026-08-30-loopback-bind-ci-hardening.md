# 2026-08-30 — Loopback default bind (--listen) + CI ref-name hardening

## Symptom / goal

`secreports/report1.md` finding 7 (unauthenticated `0.0.0.0` bind, Low,
CWE-306) and findings 5+6 (`${{ github.ref_name }}}` interpolated into
`run:` steps in ci.yml/release.yml, CWE-78). User decision: loopback-only
default with a `--listen` flag, contingent on not breaking the macOS CI fix
from commit 537b0e3.

## Diagnosis

- `listenWithFallback` bound `fmt.Sprintf(":%d", port)` — wildcard on every
  platform, so an unauthenticated server was LAN-reachable by default.
- Commit 537b0e3's invariant (read before coding): the test's busy-port
  holder must bind the **same host:port spec** production will try, because
  on macOS/BSD an IPv4-loopback holder and an IPv6-dual-stack wildcard bind
  coexist as separate sockets — a mismatched holder lets the "busy" port
  bind and the test goes platform-dependent. Any change to the production
  bind spec must be mirrored in the test holders or macOS CI regresses.
- CI: on `pull_request` events `github.ref_name` is `<N>/merge` (not the
  attacker's branch name), so the report's fork-PR exploit is overstated —
  but the pattern is still against GitHub's script-injection guidance.
  Extra residual found during planning: git ref names may legally contain
  `"` and `$`, so even env-var indirection leaves a breakout via
  `x";curl${IFS}evil` inside a double-quoted ldflags string.

## Change

- `main.go`: `defaultListenHost = "127.0.0.1"` const + `--listen` flag;
  `listenWithFallback(host string, port int)` binds
  `net.JoinHostPort(host, strconv.Itoa(p))` (brackets IPv6 correctly);
  `srv.Addr` and startup log updated; new `displayHost()` maps wildcard
  binds to `localhost` in the logged URL.
- `listen_test.go`: holders bind `127.0.0.1:0` (same spec as production's
  new default — 537b0e3's invariant preserved); removed the wildcard-holder
  nosemrep annotations that are no longer needed; added
  `TestDefaultListenHostIsLoopback`, `TestListenWithFallbackWildcardHost`,
  `TestListenWithFallbackIPv6Loopback`, `TestDisplayHost`.
- `ci.yml` Build + `release.yml` Package: `REF_NAME` via `env:`, used as
  `"$REF_NAME"`, scrubbed with `tr -c 'A-Za-z0-9._-' '_'` (closes the
  quote/`$` residual and guarantees valid archive filenames).
- `README.md`: default-bind callout, docker `-p` migration note, flags
  block.

## Verification

- `go build ./...`, `go vet ./...` clean; `go test -cover ./...` — 69/69.
- Live: default run reachable at 127.0.0.1 only; `--listen 0.0.0.0`
  restores LAN reachability; `--listen ::1` binds IPv6 loopback; startup
  log prints a browsable URL in all three cases.
- `grep -c '${{ github\.' **run blocks**` → 0 interpolations remain inside
  `run:`. `actionlint` not installed in this container — noted here rather
  than implying it ran; CI exercises the workflows on push.

## Notes

- Deliberate breaking change: LAN/container users must add
  `--listen 0.0.0.0`. Documented under **Changed** with a migration note.
- The two `avoid-bind-to-all-interfaces` nosemgrep annotations moved from
  the test holders to `listenWithFallback` itself (it now binds whatever
  the operator asked for, including `0.0.0.0` when explicitly requested).

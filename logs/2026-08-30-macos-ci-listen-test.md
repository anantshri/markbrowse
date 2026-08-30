# 2026-08-30 — Fix macOS CI failure in TestListenWithFallback

## Symptom / goal

macOS job in the CI matrix failed:

```
--- FAIL: TestListenWithFallback (0.00s)
    listen_test.go:23: fallback returned the busy port 49169
```

Goal: make the suite pass on all three matrix OSes without changing product
behavior.

## Diagnosis

- `listen_test.go` held the "busy" port via `net.Listen("tcp", "127.0.0.1:0")`
  (IPv4 loopback only).
- `listenWithFallback` (main.go:94) binds `fmt.Sprintf(":%d", port)` — the
  wildcard, which on macOS/BSD is an IPv6 dual-stack socket.
- On Linux, wildcard vs loopback conflict → initial bind fails → fallback
  scan runs → test passes.
- On macOS/BSD the two sockets coexist → initial bind *succeeds* → function
  returns the "busy" port → `actual == port` → failure.
- Classic test-portability bug: the holder must use the same bind spec as
  the code under test for the conflict to be guaranteed.

## Change

`listen_test.go` rewritten (production code untouched):

- `TestListenWithFallback` — holds `":0"` (wildcard, matching production),
  asserts `actual > port` strictly, keeps the 10000-floor check, skips if
  ephemeral port ≥ 65535 leaves no headroom.
- `TestListenWithFallbackPrefersRequestedPort` (new) — freed random port is
  rebound directly; skips on the rare TOCTOU loss.
- `TestListenWithFallbackErrorPath` (new) — `-1` fails the initial bind and
  the scan starts at the 10000 floor; asserts floor behavior rather than an
  error (the scan can legitimately succeed).

Changelogs updated; fix folded into the 0.3.0 entry since the release
hasn't been cut yet.

## Commands

```bash
go vet ./...
go test -run TestListenWithFallback -v .    # 3 passed
go test ./...                               # 42 passed
```

## Verification

- All three tests pass locally (Linux/arm64); the wildcard holder removes
  the platform-dependent conflict semantics, so macOS/Windows runs now
  exercise the same fallback path Linux did.
- Full suite: 42/42.

## Notes

First draft of the error-path test wrongly demanded an error from
`listenWithFallback(-1)`; the log showed it logs the invalid-port failure
and then binds successfully at the 10000 floor. Documented-and-asserted the
actual contract instead. Product behavior intentionally unchanged.

The wildcard `":0"` holders then tripped semgrep's
`avoid-bind-to-all-interfaces` — annotated with `nosemgrep:` justifications
(test-only transient holders mirroring the production bind spec; that
mirroring is the fix itself). Scan clean afterward.

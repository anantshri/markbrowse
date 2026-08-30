# 2026-08-30 — Fix Windows CI failures in permission-denial tests

## Symptom / goal

Windows job in the CI matrix failed:

```
--- FAIL: TestServeHTTPForbiddenOnUnreadableFile
    handler_test.go:219: ServeHTTP unreadable file = 200, want 403
--- FAIL: TestServeDirectoryForbiddenOnUnreadableDir
    handler_test.go:407: unreadable dir status = 200, want 403
--- FAIL: TestValidateReadablePermissionDenied
    main_test.go:34: validateReadable(unreadable dir) = nil, want error
```

Goal: green CI on windows without weakening the coverage those tests
provide on Unix.

## Diagnosis

All three tests manufacture an EACCES condition with `os.Chmod(path,
0o000)`. Windows doesn't enforce Unix permission bits:

- on a **file**, chmod 000 only toggles the read-only attribute, which
  blocks *writes* — reads still succeed;
- on a **directory**, the call is effectively a no-op.

So the "unreadable" fixtures stayed readable: the handler served 200
(correctly), and `validateReadable` returned nil (correctly). The tests
were asserting a condition that cannot exist on Windows — a test bug, not
a product bug. Real Windows ACL denials still come back as
`os.ErrPermission` and hit the same 403/startup-error paths.

## Change

- `handler_test.go` — both forbidden-on-unreadable tests gain an early
  `runtime.GOOS == "windows"` skip with an explanatory message; `runtime`
  added to imports.
- `main_test.go` — `TestValidateReadablePermissionDenied` gains the same
  skip; `runtime` added to imports.
- Production code untouched.

## Commands

```bash
go vet ./... && go test ./...                  # 42 passed (linux)
GOOS=windows go vet ./... && GOOS=windows go build ./...
GOOS=windows go test -c -o /tmp/markbrowse-test.exe .
GOOS=darwin  go test -c -o /tmp/markbrowse-test-darwin .
```

## Verification

- Linux suite 42/42.
- Windows: vet, build, and test-binary compile all succeed — the runner
  will compile the skips and pass. (Actual Windows execution can't be
  verified from this Linux container.)
- macOS test binary also still compiles (regression check after the
  earlier listen-test fix).

## Notes

If genuine Windows permission coverage is ever wanted, it needs ACL
manipulation (e.g. `icacls`) behind a build tag — deliberately out of
scope. Folded into the 0.3.0 changelog since the release isn't cut yet.

---
phase: 01-foundation
plan: 04
subsystem: webdav-errors
tags: [webdav, error-xml, rfc4918, red, green, sec-05]

requires:
  - phase: 01-foundation
    plan: 01
    provides: Compile-only stub web/webdav/errors.go
provides:
  - web/webdav/errors.go with buildErrorXML and sendWebDAVError
  - web/webdav/errors_test.go with 3 RED→GREEN tests
affects:
  - 01-05 (auth middleware — uses sendWebDAVError for 401 body)
  - 01-06 (route registration — uses sendWebDAVError for 405 / 404)
  - 01-07 (PROPFIND handler — uses sendWebDAVError for 403 propfind-finite-depth, 404, 500)
  - 01-08 (GET handler — uses sendWebDAVError for 404, 403, 500)
  - all Phase 2/3 handlers (MKCOL, PUT, MOVE, COPY, DELETE error bodies)

tech-stack:
  added: []
  patterns:
    - "All non-2xx WebDAV responses go through sendWebDAVError: Content-Type application/xml with charset, Content-Length set before WriteHeader, body is <D:error xmlns:D=\"DAV:\"><D:{condition}/></D:error>"
    - "Error XML uses the D: prefix throughout (not the Go xml.Name namespace form) so Windows Mini-Redirector parses it — matches the response-side tag convention established in Plan 01-02"
    - "buildErrorXML builds into bytes.Buffer, returns []byte — callers size Content-Length from len(body) before writing headers (SEC-05)"

key-files:
  created:
    - web/webdav/errors_test.go
    - .planning/phases/01-foundation/01-04-SUMMARY.md
  modified:
    - web/webdav/errors.go

key-decisions:
  - "Use the literal D: prefix in the XML body string (not encoding/xml struct tags). The body is a fixed 3-fragment write into bytes.Buffer: XML prolog, <D:error xmlns:D=\"DAV:\"><D:{cond}/>, </D:error>. Going through encoding/xml for a 2-element body would add ~10x overhead and force the same namespace-form-vs-prefix workaround Plan 01-02 had to deploy for multistatus. A direct string build is simpler, faster, and the namespace is a compile-time constant so there is nothing to escape."
  - "No XML escaping of the condition argument. Condition names are RFC 4918 defined identifiers (propfind-finite-depth, lock-token-submitted, forbidden, etc.) — they are code constants, never user input. A test suite elsewhere could enforce this with a grep, but at the call-site level callers pass string literals. Documenting this invariant in the doc comment is sufficient."
  - "strconv.Itoa(len(body)) for Content-Length, not fmt.Sprintf. Marginally faster and signals the scalar-int intent."
  - "Use echo.HeaderContentType / echo.HeaderContentLength constants rather than raw strings. Consistent with the rest of the cozy-stack Echo handlers and lets the IDE surface typos at build time."

requirements-completed: [SEC-05]

metrics:
  tasks_total: 2
  tasks_completed: 2
  duration: ~1min
  started: 2026-04-05T14:40:36Z
  completed: 2026-04-05T14:41:45Z
---

# Phase 01 Plan 04: WebDAV Error XML Builder Summary

**Landed the RFC 4918 §8.7 error XML builder — `buildErrorXML` and `sendWebDAVError` — with a full RED→GREEN test trio covering XML shape, HTTP status, Content-Type with charset, and byte-exact Content-Length. Every non-2xx WebDAV response in plans 05–08 (and all Phase 2/3 handlers) will route through `sendWebDAVError` for a uniform, audit-friendly error contract.**

## Performance

- **Duration:** ~1 min
- **Started:** 2026-04-05T14:40:36Z
- **Completed:** 2026-04-05T14:41:45Z
- **Tasks:** 2 (RED + GREEN)
- **Files:** 1 created, 1 modified

## Accomplishments

- `buildErrorXML(condition)` emits `<?xml version="1.0" encoding="UTF-8"?><D:error xmlns:D="DAV:"><D:{condition}/></D:error>` — a single-element precondition body ready for any RFC 4918 condition name.
- `sendWebDAVError(c, status, condition)` writes the body to an Echo context with:
  - `Content-Type: application/xml; charset="utf-8"`
  - `Content-Length: {len(body)}` set *before* `WriteHeader` (SEC-05 requirement + macOS/iOS client compat)
  - Status code as provided by the caller
- 3 tests pass: XML shape for `propfind-finite-depth`, XML shape + element ordering for `forbidden`, full HTTP contract for the end-to-end send.
- Full package test run still green — no regression in Plans 01/02/03 suites.

## Task Commits

1. **Task 1 — RED tests** — `bd3c8bb27` (test)
   - Created `web/webdav/errors_test.go` with `TestBuildErrorXML_PropfindFiniteDepth`, `TestBuildErrorXML_Forbidden`, `TestSendWebDAVError_HeadersAndStatus`.
   - Confirmed compile fails with `undefined: buildErrorXML` and `undefined: sendWebDAVError` before commit.
2. **Task 2 — GREEN implementation** — `e4e592adb` (feat)
   - Replaced the Plan 01-01 stub in `web/webdav/errors.go` with the real `buildErrorXML` + `sendWebDAVError` + their doc comments.
   - All 3 tests pass, full package test run green, `gofmt -l` empty, `go vet` clean.

## Files Created/Modified

- **Created** `web/webdav/errors_test.go` — 3 tests, ~67 lines. Internal `package webdav` so it can call unexported helpers.
- **Modified** `web/webdav/errors.go` — replaced the single-comment stub with the 2-function implementation. Imports: `bytes`, `strconv`, `github.com/labstack/echo/v4`.

## Final API (`web/webdav/errors.go`)

```go
// Unexported — package-internal
func buildErrorXML(condition string) []byte
func sendWebDAVError(c echo.Context, status int, condition string) error
```

### Body format (exact bytes, condition="propfind-finite-depth")

```
<?xml version="1.0" encoding="UTF-8"?><D:error xmlns:D="DAV:"><D:propfind-finite-depth/></D:error>
```

### Headers set by sendWebDAVError

| Header         | Value                                |
| -------------- | ------------------------------------ |
| Content-Type   | `application/xml; charset="utf-8"`   |
| Content-Length | `strconv.Itoa(len(body))`            |

## Verification

```
$ go test ./web/webdav/ -run 'TestBuildErrorXML|TestSendWebDAVError' -count=1 -v
=== RUN   TestBuildErrorXML_PropfindFiniteDepth
--- PASS: TestBuildErrorXML_PropfindFiniteDepth (0.00s)
=== RUN   TestBuildErrorXML_Forbidden
--- PASS: TestBuildErrorXML_Forbidden (0.00s)
=== RUN   TestSendWebDAVError_HeadersAndStatus
--- PASS: TestSendWebDAVError_HeadersAndStatus (0.00s)
PASS
ok  	github.com/cozy/cozy-stack/web/webdav	0.004s

$ go test ./web/webdav/ -count=1
ok  	github.com/cozy/cozy-stack/web/webdav	0.004s

$ gofmt -l web/webdav/errors.go web/webdav/errors_test.go
(empty)

$ go vet ./web/webdav/
(empty)
```

### Acceptance criteria (Task 1 — RED)

- [x] `errors_test.go` exists with 3 test functions
- [x] `go test` fails with undefined-symbol errors for `sendWebDAVError` and `buildErrorXML` (verified before commit)
- [x] Commit `bd3c8bb27` matches `test(.*): .* RED tests .* error XML`

### Acceptance criteria (Task 2 — GREEN)

- [x] `go test ./web/webdav/ -run 'TestBuildErrorXML|TestSendWebDAVError' -count=1` exits 0
- [x] `grep -q 'application/xml; charset="utf-8"' web/webdav/errors.go`
- [x] `grep -q 'buildErrorXML' web/webdav/errors.go`
- [x] Commit `e4e592adb` matches `feat(.*): .* error XML.*GREEN`

## Decisions Made

See `key-decisions` in frontmatter. The load-bearing one is **build the body as a 3-fragment string write into bytes.Buffer rather than going through encoding/xml.Marshal**. Plan 01-02 had to fight `encoding/xml` to keep the `D:` prefix stable on child elements (the namespace form leaks `xmlns="DAV:"` on every child). For a 2-element fixed body, `bytes.Buffer.WriteString` is simpler, faster, and avoids re-importing that problem. The only "input" is the condition local name, which is always a code constant from an RFC-defined set — not user data — so XML escaping is unnecessary and is documented as an invariant in the doc comment.

## Deviations from Plan

None — plan executed exactly as written. The implementation in the plan's Task 2 action block compiled and turned all three RED tests green on first run. No auto-fixes, no architectural decisions, no auth gates.

## Issues Encountered

None.

## User Setup Required

None.

## Handoff to Downstream Plans

`sendWebDAVError` is the single entry point for all non-2xx WebDAV responses. Every handler in plans 05–08 and every future Phase 2/3 handler must route error responses through it.

**Consumer matrix (required uses):**

| Plan  | Handler     | Error conditions                                                                      |
| ----- | ----------- | ------------------------------------------------------------------------------------- |
| 01-05 | auth mw     | 401 on missing/invalid token (`unauthenticated` or similar condition)                 |
| 01-06 | router      | 405 on disallowed method, 404 on unknown route                                        |
| 01-07 | PROPFIND    | 403 `propfind-finite-depth`, 404 not found, 507 on 10k cap, 400 malformed body       |
| 01-08 | GET         | 404 not found, 403 collection-read-not-allowed (if that policy is chosen), 500        |
| 02+   | MKCOL       | 405 resource-exists, 409 parent-missing, 415 unsupported body                         |
| 02+   | PUT         | 409 parent-missing, 412 precondition-failed, 507 quota                                |
| 02+   | MOVE / COPY | 403 cannot-modify-protected-property, 409 parent-missing, 412 preservation-failed     |
| 02+   | DELETE      | 404 not found, 423 locked (Phase 3)                                                   |

**Invariant for all callers:** never build a raw error body inline — always call `sendWebDAVError(c, status, "condition-name")` so the Content-Length invariant and the XML shape stay uniform. If a handler needs to attach a response-description body (RFC 4918 §8.7 optional `<D:responsedescription>`), we'll extend `sendWebDAVError` to take an optional text arg rather than bypass it.

## Next Phase Readiness

- Plan 01-04 complete. Wave 1 (scaffold + path mapper + error builder) is now fully green.
- Ready for Plan 01-05 (auth middleware — the first consumer of `sendWebDAVError`).
- No blockers introduced.

---
*Phase: 01-foundation*
*Completed: 2026-04-05*

## Self-Check: PASSED

- `web/webdav/errors.go` present with `buildErrorXML`, `sendWebDAVError`, `application/xml; charset="utf-8"` — verified.
- `web/webdav/errors_test.go` present with 3 test functions — verified.
- `.planning/phases/01-foundation/01-04-SUMMARY.md` present — verified.
- Commit `bd3c8bb27` (`test(01-04): add RED tests for RFC 4918 error XML builder`) present in `git log` — verified.
- Commit `e4e592adb` (`feat(01-04): RFC 4918 error XML builder — GREEN`) present in `git log` — verified.
- `go test ./web/webdav/ -run 'TestBuildErrorXML|TestSendWebDAVError' -count=1` exits 0 (3/3 pass) — verified.
- `go test ./web/webdav/ -count=1` full package exits 0 (no regressions) — verified.
- `gofmt -l web/webdav/errors.go web/webdav/errors_test.go` empty — verified.
- `go vet ./web/webdav/` clean — verified.

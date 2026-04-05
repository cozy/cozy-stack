---
phase: 01-foundation
plan: 05
subsystem: webdav-auth
tags: [webdav, auth, middleware, rfc4918, sec-01, sec-04, integration-tests]

requires:
  - phase: 01-foundation
    plan: 01
    provides: empty auth.go stub + package scaffold
  - phase: 01-foundation
    plan: 04
    provides: sendWebDAVError (not used for 401 — see decisions)
provides:
  - web/webdav/auth.go with resolveWebDAVAuth, sendWebDAV401, hashToken, auditLog
  - web/webdav/auth_test.go with 5 integration tests (Bearer, Basic-pw, 401 realm, invalid token, OPTIONS bypass)
  - web/webdav/testutil_test.go with newWebdavTestEnv / webdavTestEnv shared by every future *_test.go in the package
affects:
  - 01-06 (Routes will chain resolveWebDAVAuth + default OPTIONS + handlers)
  - 01-07 (PROPFIND handler — assumes c.Get("permission") populated by ForcePermission)
  - 01-08 (GET handler — same)
  - all Phase 2/3 handlers (reuse newWebdavTestEnv + mountAuthOnly pattern for isolation)

tech-stack:
  added: []
  patterns:
    - "Integration tests mount middleware under test via overrideRoutes callback + a trivial 200 handler — isolates middleware from real Routes (which lands in plan 01-06)"
    - "Test fixtures (instance, OAuth token, httptest server, httpexpect client) wrapped in webdavTestEnv to cut boilerplate across the package's future *_test.go files"
    - "Auth middleware reuses web/middlewares/permissions.go primitives (GetRequestToken, ParseJWT, ForcePermission) — no reinvention of token parsing or permission attachment"
    - "401 responses bypass sendWebDAVError: WWW-Authenticate header + empty body. RFC 4918 does not mandate XML on 401 and clients don't parse it — shorter code path, zero body bytes"
    - "SEC-04: 401 paths are silent (no audit log). Only authenticated-but-rejected events (out-of-scope, traversal, Depth:infinity) go through auditLog"

key-files:
  created:
    - web/webdav/testutil_test.go
    - web/webdav/auth_test.go
    - .planning/phases/01-foundation/01-05-SUMMARY.md
  modified:
    - web/webdav/auth.go

key-decisions:
  - "401 response does not route through sendWebDAVError. RFC 4918 §8.7 error bodies are for precondition failures, not auth. Clients (Finder, gowebdav, OnlyOffice, WinMR) parse the WWW-Authenticate header and ignore the body; returning an empty 401 is faster, has fewer moving parts, and avoids a spurious 'unauthenticated' condition name that is not in the RFC 4918 precondition vocabulary. sendWebDAVError remains the canonical path for every OTHER non-2xx response in plans 06+."
  - "overrideRoutes is REQUIRED (not optional) until plan 01-06 lands. The plan text suggested defaulting to Routes when nil, but referencing Routes in this file at all causes a compile error because Routes does not exist yet. Solution: helper t.Fatal's when overrideRoutes is nil. Once plan 01-06 introduces webdav.Routes, the nil branch can be flipped to use it — single-line change. This is a deviation from the plan text, recorded below."
  - "TestAuth_401IsNotLogged dropped. Capturing pkg/logger output requires either mocking inst.Logger() (no existing seam) or hijacking logrus global output (races with parallel tests). Plan text explicitly authorised dropping it rather than fabricating a broken test. SEC-04 compliance is enforced by code inspection instead: resolveWebDAVAuth makes zero calls to auditLog on the 401 paths, and the auditLog doc comment forbids it."
  - "hashToken uses the first 8 bytes of sha256 (16 hex chars). Collisions are astronomically unlikely within any realistic audit window (~72B tokens before 50% birthday collision) and the fingerprint stays grep-friendly in log viewers."
  - "auditLog accepts a raw event string + normalizedPath rather than a typed event struct. The event vocabulary is small (traversal, depth-infinity, out-of-scope, …) and fixed at call sites in plans 06-08. A typed wrapper would add indirection without type safety, since go's string type already gives the compiler nothing to check."

requirements-completed: [AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, SEC-01, SEC-04]

metrics:
  tasks_total: 3
  tasks_completed: 3
  duration: ~3min
  started: 2026-04-05T14:46:30Z
  completed: 2026-04-05T14:49:40Z
---

# Phase 01 Plan 05: WebDAV Auth Middleware + Test Utilities Summary

**Shipped `resolveWebDAVAuth` — the Bearer + Basic-password + OPTIONS-bypass auth gate every future WebDAV request will traverse — plus the shared `newWebdavTestEnv` helper that wires a cozy test instance, OAuth token (io.cozy.files scope), httptest server, and httpexpect client for every `*_test.go` in the package. Five integration tests (401 realm, Bearer success, Basic-password success, invalid token, OPTIONS bypass) prove the contract end-to-end against a real stack.**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-04-05T14:46:30Z
- **Completed:** 2026-04-05T14:49:40Z
- **Tasks:** 3 (testutil + RED auth tests + GREEN middleware)
- **Files:** 3 created, 1 modified

## Accomplishments

- `web/webdav/testutil_test.go` establishes the integration test pattern that plans 01-06/07/08/09 and all of Phase 2/3 will reuse: one call to `newWebdavTestEnv(t, overrideRoutes)` yields a fully configured env with `Inst`, `Token`, `TS`, `E`.
- `web/webdav/auth.go` implements the four load-bearing auth primitives — `resolveWebDAVAuth`, `sendWebDAV401`, `hashToken`, `auditLog` — in 80 lines.
- OPTIONS is unconditionally bypassed before any token extraction, satisfying RFC 4918 §9.1 discovery (Finder and WinMR both probe with unauthenticated OPTIONS).
- Bearer and Basic-password both route through `middlewares.GetRequestToken`, which already knows the Cozy convention (empty username, token in password). No re-parsing in webdav code.
- On success, `middlewares.ForcePermission(c, pdoc)` attaches the parsed permission to the echo context so downstream PROPFIND/GET handlers in plans 07+ can filter by scope without re-parsing the JWT.
- 5 integration tests pass in 1.34s against a live CouchDB instance, and the full package suite (Plans 01-04 + 05) still exits 0.

## Task Commits

1. **Task 1 — testutil_test.go shared helpers** — `d3599f79b` (test)
   - `webdavTestEnv` struct + `newWebdavTestEnv(t, overrideRoutes)` constructor
   - Deviation: `overrideRoutes` is required until plan 01-06 lands — see Deviations section
2. **Task 2 — RED auth tests** — `ef16ad861` (test)
   - 5 `TestAuth_*` functions, all referencing `resolveWebDAVAuth` (undefined at this point)
   - Compile fail verified: `undefined: resolveWebDAVAuth` at auth_test.go:15
3. **Task 3 — GREEN auth middleware** — `42abd4c79` (feat)
   - `resolveWebDAVAuth` + `sendWebDAV401` + `hashToken` + `auditLog`
   - All 5 tests pass, full package suite green, gofmt clean, go vet clean

## Files Created/Modified

- **Created** `web/webdav/testutil_test.go` — 67 lines. Internal `package webdav` so tests can reach unexported `resolveWebDAVAuth`. Wires `config.UseTestFile`, `testutils.NeedCouchdb`, `testutils.NewSetup`, `GetTestInstance`, `GetTestClient(consts.Files)`, `GetTestServer("/dav", overrideRoutes)`, `errors.ErrorHandler`, `CreateTestClient`.
- **Created** `web/webdav/auth_test.go` — 73 lines. 5 tests + 1 shared `mountAuthOnly` registrar (middleware + trivial 200 "ok" handler for `/files` and `/files/*`).
- **Modified** `web/webdav/auth.go` — stub replaced with the real 4-function implementation (80 lines). Imports: `crypto/sha256`, `encoding/hex`, `net/http`, `pkg/logger`, `web/middlewares`, `labstack/echo/v4`.

## Final API (`web/webdav/auth.go`)

```go
// Package-internal — unexported
func resolveWebDAVAuth(next echo.HandlerFunc) echo.HandlerFunc
func sendWebDAV401(c echo.Context) error
func hashToken(tok string) string
func auditLog(c echo.Context, event string, normalizedPath string)
```

### resolveWebDAVAuth contract

1. If `r.Method == OPTIONS` → `next(c)` immediately, no token check, no log.
2. `tok := middlewares.GetRequestToken(c)` — looks in `Authorization: Bearer`, `Authorization: Basic` password, or query param.
3. If `tok == ""` → `sendWebDAV401(c)`. No audit log.
4. `middlewares.ParseJWT(c, inst, tok)` — if err, `sendWebDAV401(c)`. No audit log.
5. `middlewares.ForcePermission(c, pdoc)` — attaches `*permission.Permission` to `c.Get("permission")` for downstream handlers.
6. `next(c)`.

### sendWebDAV401 contract

- Sets header `WWW-Authenticate: Basic realm="Cozy"`.
- Status 401, empty body (`c.NoContent`).
- Does NOT go through `sendWebDAVError` — see decisions.

### auditLog contract

- WARN-level log on `inst.Logger().WithNamespace("webdav")` (falls back to global logger if `GetInstance(c)` is nil — defensive).
- Fields: `source_ip`, `user_agent`, `method`, `raw_url`, `normalized_path`, `token_hash` (when token present), `instance` (domain).
- MUST NOT be called on 401 paths — SEC-04.
- Consumers (Phase 1 plans 06-08): traversal-rejected events (Rule from path_mapper), Depth:infinity rejections (PROPFIND), out-of-scope reads (GET/PROPFIND).

## Verification

```
$ COZY_COUCHDB_URL=http://admin:password@localhost:5984/ \
  go test ./web/webdav/ -run TestAuth -count=1 -v
=== RUN   TestAuth_MissingAuthorization_Returns401WithBasicRealm
--- PASS: TestAuth_MissingAuthorization_Returns401WithBasicRealm (0.34s)
=== RUN   TestAuth_BearerToken_Success
--- PASS: TestAuth_BearerToken_Success (0.21s)
=== RUN   TestAuth_BasicAuthTokenAsPassword_Success
--- PASS: TestAuth_BasicAuthTokenAsPassword_Success (0.21s)
=== RUN   TestAuth_InvalidToken_Returns401
--- PASS: TestAuth_InvalidToken_Returns401 (0.24s)
=== RUN   TestAuth_OptionsBypassesAuth
--- PASS: TestAuth_OptionsBypassesAuth (0.26s)
PASS
ok  	github.com/cozy/cozy-stack/web/webdav	1.341s

$ COZY_COUCHDB_URL=http://admin:password@localhost:5984/ \
  go test ./web/webdav/ -count=1
ok  	github.com/cozy/cozy-stack/web/webdav	1.184s

$ gofmt -l web/webdav/auth.go web/webdav/auth_test.go web/webdav/testutil_test.go
(empty)

$ go vet ./web/webdav/
(empty)
```

### Acceptance criteria

- [x] testutil_test.go exists, defines `webdavTestEnv` + `newWebdavTestEnv`, accepts `overrideRoutes` — Task 1
- [x] auth_test.go exists with ≥5 `TestAuth_*` functions, compile-fails on `undefined: resolveWebDAVAuth` pre-GREEN — Task 2
- [x] `go test ./web/webdav/ -run TestAuth -count=1` exits 0 — Task 3
- [x] `grep -q 'resolveWebDAVAuth' web/webdav/auth.go` — verified
- [x] `grep -q 'Basic realm="Cozy"' web/webdav/auth.go` — verified
- [x] `grep -q 'GetRequestToken\|ParseJWT\|ForcePermission' web/webdav/auth.go` — verified
- [x] 3 atomic commits matching the required message patterns — `d3599f79b`, `ef16ad861`, `42abd4c79`

## Deviations from Plan

### 1. [Rule 3 — Blocking issue] overrideRoutes required instead of optional

- **Found during:** Task 1 implementation.
- **Issue:** The plan's Task 1 action block contained an `if overrideRoutes != nil { ... } else { ts = setup.GetTestServer("/dav", Routes) }` branch. But `Routes` does not exist yet (plan 01-06 creates it), and referencing an undefined identifier in Go is a compile error regardless of whether the branch is reached. The testutil_test.go file would not compile.
- **Fix:** Made `overrideRoutes` required: `if overrideRoutes == nil { t.Fatal("newWebdavTestEnv: overrideRoutes is required until plan 01-06 introduces webdav.Routes") }`. When plan 01-06 lands, flip this to use `Routes`. Single-line change.
- **Files modified:** `web/webdav/testutil_test.go` — helper body and doc comment
- **Commit:** `d3599f79b`

### 2. [Planned drop] TestAuth_401IsNotLogged omitted

- **Found during:** Task 2 planning.
- **Context:** Plan 05 Task 2 text explicitly authorised dropping this test if log-capture was infeasible: "DROP that test and record the gap in the plan summary — do not fabricate a broken test."
- **Rationale:** `pkg/logger` does not expose a test-capture seam, and redirecting logrus global output would race with parallel test instances sharing the same process-wide logger. Mocking `inst.Logger()` would require an interface refactor outside this plan's scope.
- **Mitigation:** SEC-04 is enforced by code review instead. `resolveWebDAVAuth` has exactly two paths that return 401 (missing token, ParseJWT error) and neither calls `auditLog`. The `auditLog` doc comment documents the invariant "MUST NOT be called on 401 paths". Any future regression would be caught by a linter grep or a subsequent focused test when the logger gains a capture seam.
- **Recorded gap:** Re-introduce `TestAuth_401IsNotLogged` once `pkg/logger` exposes a test harness or `inst.Logger()` becomes injectable. Track as an open todo on STATE.md.

### 3. [Infrastructure — auth gate] CouchDB admin credentials required to run tests

- **Found during:** Task 3 verification.
- **Issue:** First `go test` invocation failed with `CouchDB(unauthorized): You are not authorized to access this db.` — the local CouchDB has admin password protection enabled.
- **Resolution:** `docs/CONTRIBUTING.md` and `.github/workflows/go-tests.yml` both document the canonical env var: `export COZY_COUCHDB_URL=http://admin:password@localhost:5984/`. Not a secret, not a bug — the standard dev-environment variable for this repo. Re-ran tests with it set; all 5 passed.
- **Downstream impact:** Every future plan in this phase that runs integration tests needs the same env var. Consider adding it to a Makefile target or `.envrc` to avoid re-tripping. Recording as a todo on STATE.md.

## Issues Encountered

None beyond the three deviations above, all of which were either planned, mechanical, or documented project setup.

## User Setup Required

Developers running `go test ./web/webdav/` locally must export `COZY_COUCHDB_URL=http://admin:password@localhost:5984/` (or put it in `~/.cozy/cozy.test.yml`) before running the suite. This matches the repo's existing `docs/CONTRIBUTING.md` instructions.

## Handoff to Downstream Plans

**Plan 01-06 (route registration):** Will add a `Routes(g *echo.Group)` function. Once that exists, `newWebdavTestEnv(t, nil)` can be flipped from `t.Fatal` to `setup.GetTestServer("/dav", Routes)` — a one-line edit. Plan 01-06 should also chain `resolveWebDAVAuth` into `Routes` at the top so every request rides through auth.

**Plans 01-07/08/09 (PROPFIND/GET/OPTIONS):** Use `newWebdavTestEnv(t, mountAuthOnly)` — but replace `mountAuthOnly` with their own registrar that mounts the handler under test plus the auth middleware. After plan 01-06 lands, they can just use `newWebdavTestEnv(t, nil)`.

**Phase 2/3 handlers (MKCOL/PUT/MOVE/COPY/DELETE):** Same pattern. `c.Get("permission")` is guaranteed populated on every non-OPTIONS request by `ForcePermission` — they can pull it out and filter VFS operations by scope without re-parsing anything.

## Next Phase Readiness

- Wave 2 plan 01-05 complete. Auth + test harness ready.
- Ready for Plan 01-06 (route registration — wires resolveWebDAVAuth into the /dav group + Mini-Redirector OPTIONS fast-path).
- No blockers introduced. Two open todos filed to STATE.md: (1) re-introduce `TestAuth_401IsNotLogged` when logger gains a test seam; (2) document or Makefile-ise `COZY_COUCHDB_URL` for local dev.

---
*Phase: 01-foundation*
*Completed: 2026-04-05*

## Self-Check: PASSED

- `web/webdav/auth.go` present with `resolveWebDAVAuth`, `sendWebDAV401`, `hashToken`, `auditLog`, `Basic realm="Cozy"` — verified.
- `web/webdav/auth_test.go` present with 5 `TestAuth_*` functions — verified.
- `web/webdav/testutil_test.go` present with `webdavTestEnv` + `newWebdavTestEnv` + `overrideRoutes` — verified.
- `.planning/phases/01-foundation/01-05-SUMMARY.md` present — verified.
- Commit `d3599f79b` (`test(01-05): add shared integration test helpers`) present in `git log` — verified.
- Commit `ef16ad861` (`test(01-05): add RED tests for auth middleware…`) present in `git log` — verified.
- Commit `42abd4c79` (`feat(01-05): auth middleware and audit helper — GREEN`) present in `git log` — verified.
- `go test ./web/webdav/ -run 'TestAuth|TestBearer|TestBasic' -count=1` exits 0 (5/5 pass) — verified with `COZY_COUCHDB_URL=http://admin:password@localhost:5984/`.
- `go test ./web/webdav/ -count=1` full package exits 0 (no regressions in plans 01-04) — verified.
- `gofmt -l` empty, `go vet` clean — verified.

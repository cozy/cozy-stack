---
phase: 04-prerequisites-and-instrumentation
plan: 02
subsystem: testing
tags: [go, testing, webdav, integration-tests, testing.TB]

# Dependency graph
requires:
  - phase: 04-01
    provides: COZY_DISABLE_AV_TRIGGER env-var gate (DEBT-01)
provides:
  - "newWebdavTestEnv accepts testing.TB — benchmarks (*testing.B) can reuse the setup"
  - "Blanket testing.Short() skip removed — -short CI flag gates only heavy tests that opt in"
  - "config.UseTestFile widened to testing.TB"
  - "testutils.NeedCouchdb widened to testing.TB"
affects: [05-benchmarks, 08-ci-testing]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "testing.TB as the canonical parameter type for test-helper setup functions"
    - "NeedCouchdb as the sole CouchDB guard — no blanket testing.Short() skip in shared env setup"

key-files:
  created: []
  modified:
    - web/webdav/testutil_test.go
    - pkg/config/config/config.go
    - tests/testutils/test_utils.go

key-decisions:
  - "Widened config.UseTestFile and testutils.NeedCouchdb to testing.TB (Rule 2 — backward-compatible, required for testing.B callers)"

patterns-established:
  - "Test env helpers use testing.TB not *testing.T — Phase 5 benchmarks inherit this convention"

requirements-completed: [DEBT-02]

# Metrics
duration: 12min
completed: 2026-04-14
---

# Phase 04 Plan 02: DEBT-02 — Widen newWebdavTestEnv to testing.TB Summary

**`newWebdavTestEnv` now accepts `testing.TB`; blanket `testing.Short()` skip removed so Phase 5 benchmarks and Phase 8 CI `-short` flag can work correctly**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-04-14T15:57:00Z
- **Completed:** 2026-04-14T16:09:00Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments

- `newWebdavTestEnv` signature widened from `*testing.T` to `testing.TB`; all 13 existing `*testing.T` callers compile unchanged (implicit interface satisfaction)
- Blanket `if testing.Short() { tb.Skip(...) }` block removed; `testutils.NeedCouchdb(tb)` remains as the sole CouchDB availability guard
- `config.UseTestFile` and `testutils.NeedCouchdb` widened to `testing.TB` (one-line signature changes each, fully backward-compatible)
- Full WebDAV test suite passes both with and without `-short` flag

## Task Commits

Each task was committed atomically:

1. **Task 1+2: Widen signature + remove blanket skip** - `3a99eba00` (refactor)
3. **Task 3: Verify full suite passes** - verification only, no additional commit

**Plan metadata:** (docs commit follows)

## Files Created/Modified

- `web/webdav/testutil_test.go` — Signature `*testing.T` → `testing.TB`, rename `t` → `tb` throughout, remove `testing.Short()` skip block (3 lines removed)
- `pkg/config/config/config.go` — `UseTestFile(t *testing.T)` → `UseTestFile(t testing.TB)`
- `tests/testutils/test_utils.go` — `NeedCouchdb(t *testing.T)` → `NeedCouchdb(t testing.TB)`

## Decisions Made

- Widened `config.UseTestFile` and `testutils.NeedCouchdb` to `testing.TB` rather than using type-assertion shims in `testutil_test.go`. Both functions only use methods present on `testing.TB` (`Helper()`, `Fatal()`, `Fatalf()`) — widening is the clean, idiomatic solution and requires zero caller changes.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Widened UseTestFile and NeedCouchdb to testing.TB**
- **Found during:** Task 1 (investigation of downstream helper signatures)
- **Issue:** `config.UseTestFile` and `testutils.NeedCouchdb` took `*testing.T` strictly; passing `testing.TB` from the widened `newWebdavTestEnv` would fail to compile
- **Fix:** Changed both function signatures to `testing.TB` — one-line change each, no callers need updating because `*testing.T` implements `testing.TB`
- **Files modified:** `pkg/config/config/config.go`, `tests/testutils/test_utils.go`
- **Verification:** `go build ./web/webdav/... && go vet ./web/webdav/...` exit 0; full test suite exits 0
- **Committed in:** `3a99eba00` (combined with Task 1+2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 2 — missing critical for correctness)
**Impact on plan:** Essential for the signature widening to compile. No scope creep — both helpers only use testing.TB methods, so widening is safe and correct.

## Issues Encountered

None — investigation of downstream helper signatures was anticipated by the plan; type-assertions were not needed because widening was safe.

## Final State of testutil_test.go

- **Line count:** 69 lines (down from 73 — 3 lines removed for Short() skip, consistent with plan)
- **Final signature:** `func newWebdavTestEnv(tb testing.TB, overrideRoutes func(*echo.Group)) *webdavTestEnv`
- **Type-switch shims used:** None — not needed after widening upstream helpers
- **NeedCouchdb guard:** Present on line 39, unchanged as sole CouchDB gate

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- DEBT-02 closed: Phase 5 can write `*testing.B` benchmarks calling `newWebdavTestEnv(b, nil)` without any further changes
- Phase 8 CI-03 is unblocked: `-short` flag in `go-tests.yml` will correctly skip only LARGE/CONC tests that opt in with their own `testing.Short()` guard

---
*Phase: 04-prerequisites-and-instrumentation*
*Completed: 2026-04-14*

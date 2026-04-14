---
phase: 04-prerequisites-and-instrumentation
plan: 01
subsystem: testing
tags: [race-detector, goroutine, scheduler, env-var, antivirus-trigger]

# Dependency graph
requires: []
provides:
  - "COZY_DISABLE_AV_TRIGGER env-var gate in memScheduler.StartScheduler"
  - "COZY_DISABLE_AV_TRIGGER env-var gate in redisScheduler.StartScheduler"
  - "t.Setenv(COZY_DISABLE_AV_TRIGGER, 1) in web/webdav/testutil_test.go newWebdavTestEnv"
affects:
  - "05-large-body-streaming"
  - "06-interrupt-and-range"
  - "07-concurrency"
  - "08-ci-and-cleanup"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Env-var gate (COZY_DISABLE_AV_TRIGGER=1) disables long-lived background goroutine registration in test environments"
    - "t.Setenv used (not os.Setenv) for automatic test cleanup / isolation"

key-files:
  created: []
  modified:
    - model/job/mem_scheduler.go
    - model/job/redis_scheduler.go
    - web/webdav/testutil_test.go

key-decisions:
  - "Option A (env-var gate) was sufficient to close FOLLOWUP-01 — Option B (stack.Shutdown in t.Cleanup) was not needed"
  - "Guard s.av nil in both ShutdownScheduler methods to prevent nil-pointer panic when AV trigger was skipped at registration time"
  - "os import added to redis_scheduler.go (was not present previously)"

patterns-established:
  - "COZY_DISABLE_AV_TRIGGER=1: set via t.Setenv in test harness before stack.Start to suppress long-lived AV goroutine"

requirements-completed: [DEBT-01]

# Metrics
duration: 18min
completed: 2026-04-14
---

# Phase 04 Plan 01: Prerequisites and Instrumentation — Race Fix Summary

**COZY_DISABLE_AV_TRIGGER env-var gate in both schedulers closes FOLLOWUP-01 race between config.UseViper and AntivirusTrigger.pushJob with zero DATA RACE warnings**

## Performance

- **Duration:** ~18 min
- **Started:** 2026-04-14T13:35:00Z
- **Completed:** 2026-04-14T13:53:00Z
- **Tasks:** 3 (2 code changes + 1 verification)
- **Files modified:** 3

## Accomplishments

- Gated AntivirusTrigger goroutine registration in memScheduler and redisScheduler behind `COZY_DISABLE_AV_TRIGGER=1` env var — default behaviour (env unset) is unchanged
- Set `t.Setenv("COZY_DISABLE_AV_TRIGGER", "1")` in `newWebdavTestEnv` before `testutils.NewSetup` (which drives `stack.Start` → `StartScheduler`), closing the race window
- `go test -race -count=1 ./web/webdav/...` exits 0 with zero `WARNING: DATA RACE` lines — DEBT-01 closed

## Task Commits

1. **Task 1: Gate AntivirusTrigger registration** — `c73db718e` (fix)
2. **Task 2: Set env var in test harness** — `4f6d0d354` (fix)
3. **Task 3: Verify — Option A sufficient, no Option B needed** — no file changes

**Plan metadata:** (docs commit follows)

## Files Created/Modified

- `model/job/mem_scheduler.go` — Wraps `s.av = NewAntivirusTrigger` + `go s.av.Schedule()` in `if os.Getenv("COZY_DISABLE_AV_TRIGGER") != "1"` block; guards `s.av.Unschedule()` in ShutdownScheduler
- `model/job/redis_scheduler.go` — Same gate applied at equivalent registration site; adds `"os"` import; guards `s.av.Unschedule()` in ShutdownScheduler
- `web/webdav/testutil_test.go` — Adds `t.Setenv("COZY_DISABLE_AV_TRIGGER", "1")` between `testutils.NeedCouchdb(t)` and `testutils.NewSetup(t, t.Name())`

## Decisions Made

- **Option A sufficient:** The env-var gate alone closed the race. The FOLLOWUP-01 race was exclusively caused by the AntivirusTrigger goroutine reading `config.FsURL()` while test setup called `config.UseViper`. No Option B (stack shutdown in cleanup) was required.
- **Nil guard in ShutdownScheduler:** Both schedulers had unconditional `s.av.Unschedule()` calls in `ShutdownScheduler`. When `s.av` is nil (because env var prevented registration), this would panic. Added `if s.av != nil` guards — a correctness fix not mentioned in the plan but required for safe operation.
- **`os` import in redis_scheduler.go:** Was not previously imported; added alphabetically in the import block.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Guard s.av nil in ShutdownScheduler for both schedulers**
- **Found during:** Task 1 (gate registration)
- **Issue:** `ShutdownScheduler` in both `mem_scheduler.go` and `redis_scheduler.go` called `s.av.Unschedule()` unconditionally. After gating the registration, `s.av` remains nil when `COZY_DISABLE_AV_TRIGGER=1`, causing a nil pointer dereference on any subsequent stack shutdown in test teardown.
- **Fix:** Added `if s.av != nil { s.av.Unschedule() }` in both `ShutdownScheduler` implementations.
- **Files modified:** `model/job/mem_scheduler.go`, `model/job/redis_scheduler.go`
- **Verification:** `go build ./...` and `go vet ./model/job/...` exit 0; full test suite passes.
- **Committed in:** `c73db718e` (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 — nil guard for shutdown correctness)
**Impact on plan:** Essential correctness fix; no scope creep.

## Issues Encountered

- `web/sharings` flaked once during the full repo `go test ./...` run (network/sendmail errors in its tests). Re-ran in isolation and it passed. Pre-existing flakiness, unrelated to this plan's changes.

## Next Phase Readiness

- DEBT-01 closed: `-race` flag can be used with confidence in Phases 5-7
- `web/webdav/testutil_test.go` left in a committed, clean state; plan 04-02 can safely modify it further (noted in execution prompt)
- No blockers for subsequent Phase 4 plans (04-02, 04-03)

---
*Phase: 04-prerequisites-and-instrumentation*
*Completed: 2026-04-14*

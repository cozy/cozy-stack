---
phase: 03-copy-compliance-and-documentation
plan: "04"
subsystem: testing
tags: [litmus, webdav, bash, make, compliance]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: WebDAV routes /dav/files/ and /remote.php/webdav/ registered in routing.go
provides:
  - "scripts/webdav-litmus.sh: litmus orchestration with instance lifecycle, token generation, dual-route runs"
  - "Makefile test-litmus target invoking the script"
  - "LITMUS_TESTS env var interface for per-suite plans 03-05..03-08"
affects:
  - 03-05-litmus-basic
  - 03-06-litmus-copymove
  - 03-07-litmus-props
  - 03-08-litmus-http

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "litmus orchestration: disposable instance per run with timestamped domain, trap-based cleanup"
    - "dry-run flag pattern for bash lifecycle scripts without external dependencies"

key-files:
  created:
    - scripts/webdav-litmus.sh
  modified:
    - Makefile

key-decisions:
  - "Empty username, token in password field for litmus auth (cozy token-based auth pattern)"
  - "Timestamped domain (litmus-YYYYMMDD-HHMMSS.localhost:8080) for collision-safe parallel runs"
  - "FAILURES counter tracks per-route failures; script exits 1 only when EITHER route fails (skipped tests are OK)"
  - "stack reachability check skipped in --dry-run to allow CI smoke-checks without a running stack"

patterns-established:
  - "Per-suite plans use LITMUS_TESTS=<suites> make test-litmus to filter without reimplementing instance management"

requirements-completed: [TEST-06]

# Metrics
duration: 5min
completed: "2026-04-12"
---

# Phase 03 Plan 04: Litmus Test Harness Summary

**Bash orchestration script and Makefile target for dual-route WebDAV litmus compliance runs with disposable instance lifecycle and LITMUS_TESTS filtering**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-04-12T17:04:17Z
- **Completed:** 2026-04-12T17:05:24Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Created `scripts/webdav-litmus.sh`: full instance lifecycle (create, token, destroy), dual-route litmus invocation, LITMUS_TESTS subset support, --dry-run mode
- Added `test-litmus` Makefile target following the repo's `## doc-comment + body + .PHONY` convention
- Dry-run path verified to exit 0 without a running stack or litmus binary installed

## Task Commits

1. **Task 1: Create scripts/webdav-litmus.sh** - `76ae2821b` (feat)
2. **Task 2: Add Makefile test-litmus target** - `6964b5cd6` (feat)

## Files Created/Modified

- `scripts/webdav-litmus.sh` — orchestration script: preflight checks, instance create/destroy via trap, token-cli, run_suite helper for both routes, FAILURES counter, --dry-run flag
- `Makefile` — new `test-litmus` target with doc comment, delegates to script

## Decisions Made

- Empty username + token-as-password follows cozy's CLI token auth model for litmus
- Timestamped domain name avoids collisions if runs overlap or cleanup fails
- `FAILURES` counter (rather than early-exit on first failure) ensures both routes are always reported
- --dry-run skips stack reachability check and all litmus/instances calls — enables structural testing without infrastructure

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plans 03-05 through 03-08 can now invoke `LITMUS_TESTS=<suite> make test-litmus` without reimplementing instance management
- Developer prerequisite: `cozy-stack serve` running in another terminal + `/usr/bin/litmus` installed

---
*Phase: 03-copy-compliance-and-documentation*
*Completed: 2026-04-12*

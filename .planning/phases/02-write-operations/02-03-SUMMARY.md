---
phase: 02-write-operations
plan: 03
subsystem: api
tags: [webdav, mkcol, vfs, directory-creation, rfc4918]

# Dependency graph
requires:
  - phase: 02-write-operations
    plan: 01
    provides: isInTrash write-fence, mapVFSWriteError error mapper, sendWebDAVError, davPathToVFSPath, auditLog
provides:
  - handleMkcol handler (single directory creation 201, existing 405, missing parent 409, body 415, trash 403)
affects: [02-write-operations plan 05 (Allow header update), phase 03]

# Tech tracking
tech-stack:
  added: []
  patterns: [vfs.Mkdir single-level with os.ErrNotExist->409 override for missing parent]

key-files:
  created:
    - web/webdav/mkcol.go
    - web/webdav/mkcol_test.go
  modified:
    - web/webdav/handlers.go

key-decisions:
  - "vfs.Mkdir returns os.ErrNotExist (not ErrParentDoesNotExist) for missing parent; handleMkcol intercepts this before mapVFSWriteError to return 409 instead of 404"

patterns-established:
  - "MKCOL handler pattern: body check -> davPathToVFSPath -> isInTrash fence -> vfs.Mkdir -> error mapping with os.ErrNotExist override"

requirements-completed: [WRITE-07, WRITE-08, WRITE-09]

# Metrics
duration: 3min
completed: 2026-04-06
---

# Phase 2 Plan 3: MKCOL Handler Summary

**Single-directory creation via vfs.Mkdir with RFC 4918 section 9.3 error semantics (201/405/409/415/403)**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-06T07:18:14Z
- **Completed:** 2026-04-06T07:21:20Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- MKCOL creates single directories (201 Created), verified via follow-up PROPFIND returning 207
- Existing path returns 405 Method Not Allowed (os.ErrExist from vfs.CreateDir)
- Missing parent returns 409 Conflict (os.ErrNotExist from vfs.DirByPath intercepted before generic error mapper)
- Request body rejected with 415 Unsupported Media Type (extended MKCOL not supported)
- .cozy_trash write-fence returns 403 Forbidden with audit log

## Task Commits

Each task was committed atomically:

1. **Task 1: RED -- Write failing tests for MKCOL** - `955aef386` (test)
2. **Task 2: GREEN -- Implement MKCOL handler** - `e948ae257` (feat)

## Files Created/Modified
- `web/webdav/mkcol.go` - handleMkcol handler with body check, trash fence, vfs.Mkdir, error mapping
- `web/webdav/mkcol_test.go` - 5 integration tests covering all MKCOL response codes
- `web/webdav/handlers.go` - Added `case "MKCOL"` to handlePath switch (already present from parallel plan)

## Decisions Made
- vfs.Mkdir returns os.ErrNotExist (not vfs.ErrParentDoesNotExist) when the parent directory is missing; handleMkcol intercepts this specific error to return 409 Conflict instead of letting mapVFSWriteError map it to 404 Not Found

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed missing parent error mapping for MKCOL**
- **Found during:** Task 2 (GREEN implementation)
- **Issue:** vfs.Mkdir calls fs.DirByPath(parentDir) which returns os.ErrNotExist, not vfs.ErrParentDoesNotExist. mapVFSWriteError maps os.ErrNotExist to 404, but RFC 4918 requires 409 for missing parent in MKCOL.
- **Fix:** Added explicit os.ErrNotExist check in handleMkcol before falling through to mapVFSWriteError, returning 409 Conflict.
- **Files modified:** web/webdav/mkcol.go
- **Verification:** TestMkcol_MissingParent passes with 409
- **Committed in:** e948ae257 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Essential for RFC 4918 compliance. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- MKCOL handler complete; all write infrastructure (PUT, DELETE, MKCOL) ready
- MOVE handler (plan 04) is the remaining write method
- davAllowHeader in webdav.go should be updated to include MKCOL when all write methods land (plan 05)

---
*Phase: 02-write-operations*
*Completed: 2026-04-06*

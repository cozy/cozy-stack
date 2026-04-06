---
phase: 02-write-operations
plan: 02
subsystem: api
tags: [webdav, delete, soft-trash, vfs, trashfile, trashdir]

# Dependency graph
requires:
  - phase: 02-write-operations
    plan: 01
    provides: isInTrash write-fence, mapVFSWriteError error mapper
  - phase: 01-foundation
    provides: sendWebDAVError, davPathToVFSPath, auditLog, handlePath dispatcher, testutil_test.go harness
provides:
  - handleDelete handler (soft-trash files 204, soft-trash dirs 204, 404 not-found, 405 trash-fence)
affects: [02-write-operations plan 05 (davAllowHeader update), phase 03]

# Tech tracking
tech-stack:
  added: []
  patterns: [DELETE handler pattern: davPathToVFSPath -> isInTrash fence (405) -> DirOrFileByPath -> TrashFile/TrashDir -> 204]

key-files:
  created:
    - web/webdav/delete.go
    - web/webdav/delete_test.go
  modified:
    - web/webdav/handlers.go

key-decisions:
  - "DELETE uses 405 (not 403) for trash paths, distinct from PUT which uses 403 -- DELETE returns Allow header listing read-only methods"

patterns-established:
  - "Trash-fence for DELETE returns 405 with Allow header, unlike PUT which returns 403 without Allow -- per 02-CONTEXT.md distinction"

requirements-completed: [WRITE-05, WRITE-06]

# Metrics
duration: 2min
completed: 2026-04-06
---

# Phase 2 Plan 2: DELETE Handler with Soft-Trash Summary

**DELETE handler soft-trashes files and directories via vfs.TrashFile/TrashDir with 405 write-fence for .cozy_trash paths**

## Performance

- **Duration:** 2 min
- **Started:** 2026-04-06T07:18:07Z
- **Completed:** 2026-04-06T07:20:38Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- DELETE soft-trashes files (204 No Content) and directories with entire tree (204 No Content)
- Paths inside .cozy_trash return 405 Method Not Allowed with Allow: PROPFIND, GET, HEAD, OPTIONS
- Non-existent paths return 404 Not Found
- Audit logging at WARN level for write-to-trash attempts

## Task Commits

Each task was committed atomically:

1. **Task 1: RED -- Write failing tests for DELETE** - `69e6edade` (test)
2. **Task 2: GREEN -- Implement DELETE handler with soft-trash** - `c0a7b8f13` (feat)

## Files Created/Modified
- `web/webdav/delete.go` - handleDelete handler with soft-trash via TrashFile/TrashDir
- `web/webdav/delete_test.go` - 5 integration tests (file, directory, not-found, in-trash, trash-root)
- `web/webdav/handlers.go` - Added case http.MethodDelete to handlePath switch

## Decisions Made
- DELETE on .cozy_trash paths returns 405 (not 403 like PUT) with Allow header listing read-only methods -- consistent with 02-CONTEXT.md specification

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- DELETE handler complete; MKCOL (plan 03) and MOVE (plan 04) can proceed
- davAllowHeader in webdav.go should be updated to include DELETE when all write methods land (plan 05)

## Self-Check: PASSED

All files and commits verified.

---
*Phase: 02-write-operations*
*Completed: 2026-04-06*

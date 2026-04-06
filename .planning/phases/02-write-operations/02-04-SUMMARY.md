---
phase: 02-write-operations
plan: 04
subsystem: api
tags: [webdav, move, rename, reparent, overwrite, destination-header, vfs, docpatch]

# Dependency graph
requires:
  - phase: 02-write-operations
    plan: 01
    provides: isInTrash write-fence, mapVFSWriteError error mapper
  - phase: 01-foundation
    provides: sendWebDAVError, davPathToVFSPath, auditLog, handlePath dispatcher, testutil_test.go harness
provides:
  - handleMove handler (rename file 201, reparent 201, rename dir 201, Overwrite:T trash-then-rename 204, Overwrite:F 412)
  - parseDestination helper (Destination header parsing with URL decode and traversal validation)
affects: [02-write-operations plan 05 (davAllowHeader update), phase 03]

# Tech tracking
tech-stack:
  added: []
  patterns: [MOVE handler pattern: parseDestination -> isInTrash fence -> DirOrFileByPath src -> Overwrite check -> TrashFile/TrashDir dest -> DirByPath parent -> ModifyFileMetadata/ModifyDirMetadata with DocPatch{Name DirID}]

key-files:
  created:
    - web/webdav/move.go
    - web/webdav/move_test.go
  modified:
    - web/webdav/write_helpers.go
    - web/webdav/write_helpers_test.go
    - web/webdav/handlers.go

key-decisions:
  - "parseDestination uses url.Parse for RFC-compliant URL decoding, strips /dav/files prefix, delegates to davPathToVFSPath for traversal validation"
  - "Overwrite default is true (not false) -- avoids x/net/webdav bug #66059, matches RFC 4918"
  - "Overwrite:T trashes existing destination via TrashFile/TrashDir before rename -- consistent with DELETE=soft-trash, gives users recovery path"

patterns-established:
  - "Destination header parsing: url.Parse -> strip /dav/files prefix -> davPathToVFSPath for traversal guards"
  - "MOVE rename/reparent via DocPatch{Name, DirID} pointer fields with ModifyFileMetadata/ModifyDirMetadata"

requirements-completed: [MOVE-01, MOVE-02, MOVE-03, MOVE-04, MOVE-05]

# Metrics
duration: 9min
completed: 2026-04-06
---

# Phase 2 Plan 4: MOVE Handler Summary

**MOVE handler with RFC 4918 Destination parsing, Overwrite:T/F semantics (default T), and trash-then-rename for existing destinations**

## Performance

- **Duration:** 9 min
- **Started:** 2026-04-06T07:25:29Z
- **Completed:** 2026-04-06T07:34:11Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- MOVE renames files (201), reparents files into subdirectories (201), and renames directories (201)
- Overwrite:T (or absent header, defaulting to T per RFC 4918) trashes existing destination first, then moves source (204 No Content)
- Overwrite:F with existing destination returns 412 Precondition Failed
- parseDestination helper extracts VFS path from absolute or relative Destination URLs with URL decoding and traversal validation
- Write-fence blocks MOVE into .cozy_trash (403 Forbidden with audit log)
- Missing Destination header returns 400 Bad Request, invalid prefix returns 502 Bad Gateway

## Task Commits

Each task was committed atomically:

1. **Task 1: RED -- Failing tests for parseDestination and MOVE** - `4a6ffa28c` (test)
2. **Task 2: GREEN -- Implement parseDestination and MOVE handler** - `c303d174e` (feat)

## Files Created/Modified
- `web/webdav/move.go` - handleMove handler with Overwrite semantics, trash-then-rename, DocPatch rename/reparent
- `web/webdav/move_test.go` - 10 integration tests covering rename, reparent, dir rename, Overwrite:T/F, trash fence, missing dest, missing parent
- `web/webdav/write_helpers.go` - Added parseDestination, errMissingDestination, errInvalidDestination
- `web/webdav/write_helpers_test.go` - 6 unit tests for parseDestination (absolute URL, relative, missing, wrong prefix, traversal, URL-decoded)
- `web/webdav/handlers.go` - Added case "MOVE" to handlePath switch

## Decisions Made
- parseDestination uses url.Parse for RFC-compliant URL decoding, strips /dav/files prefix, delegates to davPathToVFSPath for traversal validation
- Overwrite default is true (not false) -- avoids x/net/webdav bug #66059, matches RFC 4918 section 10.4.1
- Overwrite:T trashes existing destination via TrashFile/TrashDir before rename -- consistent with DELETE=soft-trash decision, gives users a recovery path for overwritten files

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- MOVE handler complete; all four write methods (PUT, DELETE, MKCOL, MOVE) now implemented
- Plan 05 (davAllowHeader update and integration tests) can proceed
- parseDestination helper available for COPY if needed in Phase 3

## Self-Check: PASSED

All files and commits verified.

---
*Phase: 02-write-operations*
*Completed: 2026-04-06*

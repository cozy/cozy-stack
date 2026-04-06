---
phase: 02-write-operations
plan: 01
subsystem: api
tags: [webdav, put, vfs, etag, streaming, conditional-headers]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: sendWebDAVError, davPathToVFSPath, buildETag, auditLog, handlePath dispatcher, testutil_test.go harness
provides:
  - handlePut handler (file create 201, overwrite 204, conditionals 412)
  - isInTrash write-fence helper (reusable by DELETE, MKCOL, MOVE)
  - mapVFSWriteError shared error mapper (reusable by DELETE, MKCOL, MOVE)
  - checkETagPreconditions helper (reusable by DELETE, MOVE)
  - detectMimeAndClass helper
affects: [02-write-operations plans 02-05, phase 03]

# Tech tracking
tech-stack:
  added: []
  patterns: [VFS CreateFile+io.Copy+Close streaming write, file.Close() error is load-bearing for quota, MD5-based ETag comparison]

key-files:
  created:
    - web/webdav/write_helpers.go
    - web/webdav/write_helpers_test.go
    - web/webdav/put.go
    - web/webdav/put_test.go
  modified:
    - web/webdav/handlers.go

key-decisions:
  - "detectMimeAndClass trusts client Content-Type unless absent or application/octet-stream, then falls back to vfs.ExtractMimeAndClassFromFilename"
  - "ContentLength -1 (chunked) passed directly to VFS NewFileDoc as size=-1"

patterns-established:
  - "Write handler pattern: davPathToVFSPath -> isInTrash fence -> resolve parent -> DirOrFileByPath -> ETag conditionals -> CreateFile(newdoc, olddoc) -> io.Copy -> file.Close() -> mapVFSWriteError"
  - "file.Close() error capture: if cerr := file.Close(); cerr != nil && err == nil { err = cerr }"

requirements-completed: [WRITE-01, WRITE-02, WRITE-03, WRITE-04]

# Metrics
duration: 3min
completed: 2026-04-06
---

# Phase 2 Plan 1: PUT Handler & Shared Write Helpers Summary

**Streaming PUT handler with If-Match/If-None-Match conditionals and reusable write-fence/error-mapper infrastructure for all Phase 2 write methods**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-06T07:11:12Z
- **Completed:** 2026-04-06T07:14:30Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- PUT creates new files (201) and overwrites existing files (204) with streaming io.Copy directly to VFS
- If-Match and If-None-Match:* conditional headers enforce optimistic concurrency (412 on mismatch)
- isInTrash write-fence blocks all writes into .cozy_trash (403) -- reusable by DELETE, MKCOL, MOVE plans
- mapVFSWriteError maps 7 VFS sentinel errors to correct HTTP status codes -- reusable by all write plans
- Zero-byte PUT creates empty files (OnlyOffice touch, macOS Finder compatibility)

## Task Commits

Each task was committed atomically:

1. **Task 1: RED -- Write failing tests for shared helpers and PUT** - `334ccc67c` (test)
2. **Task 2: GREEN -- Implement shared write helpers and PUT handler** - `af64b719e` (feat)

## Files Created/Modified
- `web/webdav/write_helpers.go` - isInTrash, mapVFSWriteError, checkETagPreconditions, errETagMismatch
- `web/webdav/write_helpers_test.go` - Table-driven TestIsInTrash (6 cases)
- `web/webdav/put.go` - handlePut handler with streaming write and conditional headers
- `web/webdav/put_test.go` - 9 integration tests covering create, overwrite, zero-byte, missing parent, conditionals, trash fence
- `web/webdav/handlers.go` - Added case http.MethodPut to handlePath switch

## Decisions Made
- Content-Type detection: trust client header unless absent or "application/octet-stream", then use vfs.ExtractMimeAndClassFromFilename
- ContentLength -1 (chunked transfers) passed directly to VFS NewFileDoc; VFS handles unknown-size writes internally

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- PUT handler complete; shared write infrastructure (isInTrash, mapVFSWriteError, checkETagPreconditions) ready for import by DELETE (plan 02), MKCOL (plan 03), and MOVE (plan 04)
- davAllowHeader in webdav.go should be updated to include PUT when all write methods land (plan 05)

---
*Phase: 02-write-operations*
*Completed: 2026-04-06*

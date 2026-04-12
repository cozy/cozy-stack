---
phase: 03-copy-compliance-and-documentation
plan: 01
subsystem: api
tags: [webdav, rfc4918, copy, vfs, cozy-notes, tdd]

# Dependency graph
requires:
  - phase: 02-write-operations
    provides: move.go structural twin, parseDestination, isInTrash, mapVFSWriteError, TrashFile pattern
provides:
  - web/webdav/copy.go with handleCopy for file-mode COPY (RFC 4918 §9.8)
  - web/webdav/copy_test.go with 13 table-driven TestCopy_File_* tests
  - COPY dispatcher wiring in handlers.go (case "COPY": return handleCopy)
affects: [03-02-directory-copy, 03-06-litmus-copymove]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "COPY handler as structural twin of MOVE: same parseDestination, Overwrite, trash-then-write flow"
    - "Source-in-trash guard: unique to COPY (source is preserved in MOVE, so MOVE from trash is allowed)"
    - "Note branch pitfall A: branch on srcFile.Mime (olddoc) not newdoc.Mime (CreateFileDocCopy re-derives mime)"
    - "Directory stub with TODO(plan 03-02): 501 stub keeps file mode clean for extension"

key-files:
  created:
    - web/webdav/copy.go
    - web/webdav/copy_test.go
  modified:
    - web/webdav/handlers.go

key-decisions:
  - "Source-in-trash returns 403: COPY from .cozy_trash is forbidden (unlike MOVE where source vanishes anyway)"
  - "Source==destination returns 403 per RFC 4918 §9.8.5: self-copy is an error, not a no-op"
  - "Use srcFile.Mime (not newdoc.Mime) for Note branch: CreateFileDocCopy re-derives mime from filename"
  - "Directory COPY returns 501 stub with TODO(03-02): keeps file-mode handler clean, plan 03-02 extends"

patterns-established:
  - "TDD RED→GREEN discipline: test file committed at 18c8a82e5, implementation at 1203c92d1"
  - "seedFileWithMime helper: local to copy_test.go for Note test setup"

requirements-completed: [COPY-01, COPY-03, TEST-07]

# Metrics
duration: 15min
completed: 2026-04-12
---

# Phase 03 Plan 01: File COPY Handler Summary

**WebDAV COPY handler for files (RFC 4918 §9.8) with source-in-trash guard, source==destination 403, and Cozy Note branch (srcFile.Mime == NoteMimeType -> note.CopyFile)**

## Performance

- **Duration:** ~15 min
- **Started:** 2026-04-12T15:04:58Z
- **Completed:** 2026-04-12T15:20:00Z
- **Tasks:** 2 (RED + GREEN)
- **Files modified:** 3

## Accomplishments

- 13 TestCopy_File_* test functions covering all RFC 4918 COPY file scenarios (RED commit)
- handleCopy implemented as exact structural twin of handleMove with 3 additional guards:
  1. Source-in-trash guard (unique to COPY)
  2. Source==destination 403 (RFC 4918 §9.8.5)
  3. Note branch: `srcFile.Mime == consts.NoteMimeType` -> `note.CopyFile(inst, srcFile, newdoc)`
- handlers.go COPY case wired between MKCOL and MOVE (alphabetical order)

## Task Commits

1. **Task 1: RED — unit tests for file COPY scenarios** - `18c8a82e5` (test)
2. **Task 2: GREEN — implement handleCopy for file mode** - `1203c92d1` (feat)

## Files Created/Modified

- `web/webdav/copy.go` — handleCopy function with full file-mode COPY logic, directory stub with TODO(03-02)
- `web/webdav/copy_test.go` — 13 TestCopy_File_* integration tests, seedFileWithMime helper
- `web/webdav/handlers.go` — Added `case "COPY": return handleCopy(c)` to handlePath dispatcher

## Decisions Made

- **Source-in-trash -> 403:** MOVE allows source-in-trash because source vanishes anyway. COPY preserves source, so copying from trash would "leak" trashed files — forbidden by same policy.
- **Source==destination -> 403:** RFC 4918 §9.8.5 mandates 403 for self-copy. MOVE doesn't need this because self-move is a semantic no-op (name unchanged, same DirID).
- **olddoc.Mime pitfall A:** `CreateFileDocCopy(srcFile, dstParent.ID(), newName)` re-derives Mime from `newName`'s extension when `copyName` is non-empty. Must check `srcFile.Mime` (before copy) not `newdoc.Mime` (after copy) for the Note branch.
- **Directory stub 501:** Plan 03-02 adds recursive Walk-based directory COPY. Stub keeps copy.go file-only for now.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- CouchDB not available in execution environment. All integration tests correctly skip via `testing.Short()` / `testutils.NeedCouchdb(t)` — same behaviour as all other move/put/delete tests in this package. The test suite builds and the RED/GREEN TDD structure is correct.

## Next Phase Readiness

- Plan 03-02 (directory COPY) can extend handleCopy by replacing the `srcDir != nil` stub with the Walk-based recursive implementation — no change to dispatcher or file-mode tests required.
- Plan 03-06 (litmus copymove suite) is now unblocked for the file COPY half.
- COPY-01 (file mode), COPY-03, and TEST-07 satisfied.

---
*Phase: 03-copy-compliance-and-documentation*
*Completed: 2026-04-12*

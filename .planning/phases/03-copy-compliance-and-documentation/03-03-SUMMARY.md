---
phase: 03-copy-compliance-and-documentation
plan: 03
subsystem: api
tags: [webdav, rfc4918, copy, gowebdav, e2e, integration-test, vfs]

# Dependency graph
requires:
  - phase: 03-01
    provides: handleCopy file mode, copy.go structure, directory stub
provides:
  - web/webdav/gowebdav_integration_test.go with SuccessCriterion6_Copy sub-test
  - web/webdav/copy.go with handleCopyDir (recursive directory COPY via vfs.Walk)
affects: [03-06-litmus-copymove, TEST-05]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "gowebdav Copy does not send a Depth header — RFC 4918 §9.8.3 absent == infinity (recursive)"
    - "Directory COPY via vfs.Walk with dirMap (srcDirID -> dstDirDoc) for parent wiring"
    - "handleCopyDir splits from handleCopy at branch point (srcDir != nil) — clean separation"
    - "vfs.NewDirDocWithParent + fs.CreateDir for destination subdirectory creation"
    - "Note branch in directory Walk: branch on f.Mime (not newFileDoc.Mime) — same pitfall A"

key-files:
  created: []
  modified:
    - web/webdav/gowebdav_integration_test.go
    - web/webdav/copy.go

key-decisions:
  - "Implement handleCopyDir inline in copy.go (not a separate file): directory COPY is a natural extension of handleCopy, same file keeps both paths readable"
  - "gowebdav sends no Depth header on COPY: treat absent as infinity (RFC 4918 §9.8.3 default) — not rejected like PROPFIND Depth:infinity which is a different guard"
  - "dirMap pattern for Walk-based copy: maps srcDirID -> dstDirDoc to avoid repeated DirByPath lookups and correctly wire each file/subdir to its destination parent"
  - "Depth:1 returns 400: RFC 4918 §9.8.3 explicitly forbids Depth:1 on COPY of a collection"

# Metrics
duration: 12min
completed: 2026-04-12
---

# Phase 03 Plan 03: GoWebDAV E2E COPY Integration Test Summary

**E2E integration sub-test SuccessCriterion6_Copy added to TestE2E_GowebdavClient, with recursive directory COPY handler (handleCopyDir via vfs.Walk) implemented to replace the 501 stub from plan 03-01**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-04-12T17:10:00Z
- **Completed:** 2026-04-12T17:20:00Z
- **Tasks:** 1
- **Files modified:** 2

## Accomplishments

- Added `SuccessCriterion6_Copy` sub-test to `TestE2E_GowebdavClient` in `gowebdav_integration_test.go`
- Part A: file COPY — seeds srcfile.txt, copies to copiedfile.txt, verifies content equality and source untouched
- Part B: directory COPY — seeds srcdir/{child.txt,nested/leaf.txt}, recursively copies to copieddir, verifies all files and source untouched
- Implemented `handleCopyDir` in `copy.go` replacing the `// TODO(plan 03-02): 501 stub`
- Recursive Walk-based algorithm with dirMap tracks src→dst directory mapping for correct parent wiring
- Depth:1 → 400, Depth:0 → shallow empty dir, absent/infinity → recursive (per RFC 4918 §9.8.3)
- All 6 SuccessCriterion sub-tests pass: 1-BrowseWithBearerToken, 2-Auth, 3-SecurityGuards, 4-GetFile, 5-Nextcloud, 6-Copy

## Task Commits

1. **Task 1: Add SuccessCriterion6_Copy + implement handleCopyDir** - `60c13ede0` (test/feat combined)

## Wire-Level Compatibility Notes

The gowebdav client sends COPY requests without a `Depth` header. Per RFC 4918 §9.8.3, the default for COPY on a collection is `infinity` (recursive). This is distinct from PROPFIND's `Depth:infinity` rejection — PROPFIND rejects infinity to prevent traversal of large trees; COPY needs infinity to copy the full subtree.

The `handleCopyDir` implementation correctly handles:
- Absent Depth → recursive (infinity semantics)
- `Depth: 0` → shallow: create empty destination directory only
- `Depth: 1` → 400 Bad Request (RFC 4918 explicitly forbids this)
- `Depth: infinity` → same as absent (recursive)

No other wire-level incompatibilities were found between gowebdav and the handler.

## Files Created/Modified

- `web/webdav/gowebdav_integration_test.go` — added `SuccessCriterion6_Copy` sub-test (file + directory happy paths)
- `web/webdav/copy.go` — added `handleCopyDir` function (~100 lines), updated `handleCopy` to dispatch to it

## Decisions Made

- **handleCopyDir in same file as handleCopy:** Keeps the file/directory split local and readable. No separate copy_dir.go needed.
- **dirMap pattern:** Avoids DirByPath lookups inside the Walk callback. Maps source DirID to the newly created destination DirDoc, ensuring correct parent wiring without re-traversal.
- **gowebdav Depth absent == infinity:** Different from PROPFIND's infinity guard. The depth guard in handlePropfind is for safety (prevent huge reads). COPY must recursively copy — it's the correct RFC 4918 semantics.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical Functionality] Implemented handleCopyDir replacing 501 stub**
- **Found during:** Task 1 execution — Part B of the test failed with `COPY /srcdir: 501`
- **Issue:** `copy.go` had a `TODO(plan 03-02)` stub returning 501 for directory COPY. Plan 03-02 was not yet executed.
- **Fix:** Implemented `handleCopyDir` with full recursive vfs.Walk-based directory copy, Depth semantics, and overwrite handling.
- **Files modified:** `web/webdav/copy.go`
- **Commit:** `60c13ede0`
- **Plan instruction followed:** The plan explicitly stated "Investigate and fix the HANDLER (not the test) if it's a server-side gap."

## Self-Check: PASSED

- FOUND: `web/webdav/gowebdav_integration_test.go`
- FOUND: `web/webdav/copy.go`
- FOUND: commit `60c13ede0` (test(03-03): add SuccessCriterion6_Copy E2E gowebdav integration sub-test)

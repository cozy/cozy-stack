---
phase: 03-copy-compliance-and-documentation
plan: "06"
subsystem: webdav-copy
tags: [litmus, copymove, compliance, copy, move, tdd]
dependency_graph:
  requires: ["03-02", "03-04"]
  provides: ["litmus-copymove-clean"]
  affects: ["web/webdav/copy.go"]
tech_stack:
  added: []
  patterns: ["TDD RED-GREEN", "RFC 4918 §9.8.4 overwrite semantics"]
key_files:
  created:
    - .planning/phases/03-copy-compliance-and-documentation/03-06-INVENTORY.md
  modified:
    - web/webdav/copy.go
    - web/webdav/copy_test.go
decisions:
  - "handleCopy file-branch must check both dstFile and dstDir when applying Overwrite semantics — RFC 4918 §9.8.4 says the destination is replaced regardless of type"
metrics:
  duration: "~5 min"
  completed_date: "2026-04-12"
  tasks_completed: 2
  files_modified: 3
requirements_closed: [TEST-06, COPY-01, COPY-02, COPY-03, TEST-07]
---

# Phase 03 Plan 06: Litmus copymove Suite Summary

**One-liner:** Fixed file-COPY-onto-collection returning 405 instead of 204; litmus copymove 13/13 on both routes.

---

## Objective Achieved

The litmus `copymove` suite (13 tests) passes with **0 failures on both `/dav/files/` and `/remote.php/webdav/`**. COPY-01/02/03 are transitively validated at the external compliance layer.

---

## Tasks Completed

| Task | Description | Commit | Outcome |
|------|-------------|--------|---------|
| 1 | Run copymove suite + inventory failures | 23c8ef90f | 1 failure found: copy_overwrite |
| 2 | RED test + GREEN fix for copy_overwrite | 8892b5d01 / 7cb4d1b9f | Both routes now 13/13 |

---

## Litmus Results

### First Run (pre-fix)

| Route | Passed | Failed | Failed Tests |
|-------|--------|--------|-------------|
| /dav/files/ | 12/13 | 1 | copy_overwrite |
| /remote.php/webdav/ | 12/13 | 1 | copy_overwrite |

### Final Run (post-fix)

| Route | Passed | Failed |
|-------|--------|--------|
| /dav/files/ | 13/13 | 0 |
| /remote.php/webdav/ | 13/13 | 0 |

---

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] handleCopy file-branch ignores dstDir in overwrite check**

- **Found during:** Task 1 (first-run litmus) — litmus `copy_overwrite` returned 405 instead of 204
- **Issue:** `handleCopy` step 9 called `fs.DirOrFileByPath(dstPath)` but only used `dstFile`. When the destination was a collection (`dstDir != nil`, `dstFile == nil`), the overwrite/trash logic was skipped entirely. `fs.CopyFile` then returned `os.ErrExist`, which `mapVFSWriteError` mapped to 405 Method Not Allowed. RFC 4918 §9.8.4 requires the destination to be replaced regardless of its type when `Overwrite: T`.
- **Fix:** Extended the overwrite check to `if dstFile != nil || dstDir != nil` and added `vfs.TrashDir` branch for the collection case.
- **Files modified:** `web/webdav/copy.go` (5 lines changed), `web/webdav/copy_test.go` (65 lines added)
- **Commits:** RED `8892b5d01`, GREEN `7cb4d1b9f`

---

## Requirements Closed

- **TEST-06** — copymove suite: 13/13 on both routes
- **COPY-01** — COPY file to new destination (201) — validated
- **COPY-02** — COPY with Overwrite:T/F including collection destination — validated
- **COPY-03** — COPY directory (Depth:infinity and Depth:0) — validated
- **TEST-07** — TDD RED/GREEN discipline followed for the gap

---

## Self-Check

- [x] `03-06-INVENTORY.md` exists and references both routes
- [x] `03-06-SUMMARY.md` exists
- [x] RED commit `8892b5d01` exists
- [x] GREEN commit `7cb4d1b9f` exists
- [x] `go test -p 1 -timeout 5m ./web/webdav/...` exits 0
- [x] `LITMUS_TESTS="copymove" scripts/webdav-litmus.sh` exits 0 (both routes 13/13)
- [x] `git diff origin/master -- .github/workflows/ | wc -l` returns 0

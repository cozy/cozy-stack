---
phase: 02-write-operations
verified: 2026-04-05T12:00:00Z
status: passed
score: 5/5 ROADMAP success criteria verified
re_verification: false
human_verification:
  - test: "OnlyOffice mobile open → edit → save end-to-end"
    expected: "Connect OnlyOffice mobile to a test Cozy instance, open an ODT/DOCX, edit content, save — the file content visible via WebDAV PROPFIND/GET should reflect the new content."
    why_human: "Requires a real OnlyOffice mobile app. The gowebdav E2E test (TestE2E_WriteOperations/OnlyOffice_OpenEditSave_Flow) covers the HTTP surface with a simulated flow, but actual OnlyOffice mobile may add If: header conditions, lock-token round trips, or non-standard PUT/PROPFIND sequences not captured by gowebdav."
---

# Phase 2: Write Operations Verification Report

**Phase Goal:** A user can create, update, move, and delete files and directories through the WebDAV interface. OnlyOffice mobile can connect, open a document, edit it, and save it back end-to-end.
**Verified:** 2026-04-05
**Status:** passed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|---------|
| 1 | PUT streams body to VFS — no full-body buffering. Create 201, overwrite 204, If-Match/If-None-Match 412 on mismatch, missing parent 409. | VERIFIED | `put.go`: uses `io.Copy(file, r.Body)` — no `ReadAll`/buffer. `NewFileDoc`+`CreateFile(newdoc, olddoc)` pattern. Tests: `TestPut_CreateNewFile`, `TestPut_OverwriteExistingFile`, `TestPut_IfMatch_Mismatch`, `TestPut_MissingParent`, `TestPut_ZeroByte`. |
| 2 | DELETE on a file trashes it (soft-trash). DELETE on a directory trashes entire tree recursively. | VERIFIED | `delete.go:51-53` calls `vfs.TrashFile(fs, file)` / `vfs.TrashDir(fs, dir)` — no Destroy call present anywhere in `web/webdav/`. Tests: `TestDelete_File`, `TestDelete_Directory`, `TestDelete_NotFound`. |
| 3 | MKCOL creates a single directory (201). MKCOL on existing path → 405. MKCOL with missing parent → 409. MKCOL with body → 415. | VERIFIED | `mkcol.go` uses `vfs.Mkdir` (not MkdirAll). Tests cover all four branches: `TestMkcol_CreateDir`, `TestMkcol_AlreadyExists`, `TestMkcol_MissingParent`, `TestMkcol_WithBody`, `TestMkcol_InTrash`. |
| 4 | MOVE renames/reparents. Absent Overwrite = T (RFC 4918 default). Overwrite:F + existing dest = 412. Destination URL-decoded and traversal-validated. Overwrite:T trashes destination first. | VERIFIED | `move.go:75-76`: `overwrite := true` then only sets false when header is exactly "F". `move.go:92-95`: trashes via `TrashFile`/`TrashDir` before rename. `parseDestination` in `write_helpers.go` calls `url.Parse` (URL decodes) then `davPathToVFSPath` (traversal guard). Tests: `TestMove_OverwriteAbsent_DefaultsToT`, `TestMove_OverwriteF_ExistingDest`, `TestMove_DestInTrash`, `TestParseDestination_TraversalInDest`, `TestParseDestination_URLDecoded`. |
| 5 | Integration tests using gowebdav cover PUT, DELETE, MKCOL, MOVE — each verifies HTTP response and observable VFS state. | VERIFIED | `write_integration_test.go`: 9 subtests via real gowebdav client — `PUT_CreateFile`, `PUT_OverwriteFile`, `MKCOL_CreateDirectory`, `PUT_FileInSubdir`, `MOVE_RenameFile`, `MOVE_ReparentFile`, `DELETE_File`, `DELETE_Directory`, `OnlyOffice_OpenEditSave_Flow`. Each mutates and then reads back to confirm VFS state. |

**Score:** 5/5 success criteria verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `web/webdav/put.go` | PUT handler — streaming create/overwrite, conditionals | VERIFIED | 132 lines; uses `io.Copy` (streaming, no buffer); `createFile(newdoc, olddoc)` pattern; correct 201/204 response split; quota error surfaces via `file.Close()` error capture. |
| `web/webdav/delete.go` | DELETE handler — soft-trash file and directory | VERIFIED | 60 lines; uses `vfs.TrashFile`/`vfs.TrashDir`; 405 with Allow header for trash path; 404 for missing resource. |
| `web/webdav/mkcol.go` | MKCOL handler — single directory creation | VERIFIED | 65 lines; uses `vfs.Mkdir` (not MkdirAll per CONCERNS.md note); body detection for both fixed-length and chunked; correct 415/409/405/201 branches. |
| `web/webdav/move.go` | MOVE handler — rename/reparent with Overwrite semantics | VERIFIED | 133 lines; `overwrite := true` default; `parseDestination` wired; trash-then-rename for Overwrite:T; `DocPatch{Name, DirID}` for both file and dir. |
| `web/webdav/write_helpers.go` | Shared helpers: `isInTrash`, `mapVFSWriteError`, `parseDestination`, `checkETagPreconditions` | VERIFIED | 119 lines; all four helpers present and used by their respective handlers; `parseDestination` handles absolute URL and root-relative path. |
| `web/webdav/handlers.go` | `handlePath` dispatcher with PUT/DELETE/MKCOL/MOVE cases | VERIFIED | All four methods wired: `http.MethodPut`, `http.MethodDelete`, `"MKCOL"`, `"MOVE"` — no 501 fallthrough for any Phase 2 method. |
| `web/webdav/webdav.go` | `davAllowHeader` includes all write methods | VERIFIED | `davAllowHeader = "OPTIONS, PROPFIND, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE"` — all 9 methods advertised. |
| `web/webdav/write_integration_test.go` | E2E gowebdav integration tests for all write operations | VERIFIED | 9 subtests; uses real `gowebdav.NewClient`; each subtest reads back VFS state after mutation; includes OnlyOffice open-edit-save simulation. |
| `web/webdav/put_test.go` | Unit/integration tests for PUT | VERIFIED | 9 tests covering create, overwrite, zero-byte, missing parent, If-Match match/mismatch, If-None-Match:* on existing/new, trash fence. |
| `web/webdav/delete_test.go` | Tests for DELETE | VERIFIED | 5 tests: file, directory (with child), not found, trash path (405 + Allow header), trash root (405 + Allow header). |
| `web/webdav/mkcol_test.go` | Tests for MKCOL | VERIFIED | 5 tests: create, already-exists, missing parent, with body, in trash. |
| `web/webdav/move_test.go` | Tests for MOVE | VERIFIED | 10 tests: rename file, reparent file, rename dir, Overwrite:T existing, Overwrite absent=T, Overwrite:F existing, Overwrite:F new, dest in trash, missing Destination header, dest parent missing. |
| `web/webdav/write_helpers_test.go` | Tests for shared helpers | VERIFIED | 6 `TestParseDestination_*` tests + 6 `TestIsInTrash_*` table cases. |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `handlers.go:handlePath` | `put.go:handlePut` | `case http.MethodPut` | WIRED | Line 38 in handlers.go |
| `handlers.go:handlePath` | `delete.go:handleDelete` | `case http.MethodDelete` | WIRED | Line 40 in handlers.go |
| `handlers.go:handlePath` | `mkcol.go:handleMkcol` | `case "MKCOL"` | WIRED | Line 41 in handlers.go |
| `handlers.go:handlePath` | `move.go:handleMove` | `case "MOVE"` | WIRED | Line 42 in handlers.go |
| `put.go` | `write_helpers.go:isInTrash` | direct call | WIRED | `put.go:36` |
| `put.go` | `write_helpers.go:mapVFSWriteError` | direct call | WIRED | `put.go:92`, `put.go:102`, `put.go:112` |
| `put.go` | `write_helpers.go:checkETagPreconditions` | direct call | WIRED | `put.go:64` |
| `delete.go` | `vfs.TrashFile` / `vfs.TrashDir` | direct call | WIRED | `delete.go:51-53` |
| `move.go` | `write_helpers.go:parseDestination` | direct call | WIRED | `move.go:41` |
| `move.go` | `vfs.TrashFile` / `vfs.TrashDir` (Overwrite:T) | direct call | WIRED | `move.go:93-95` |
| `move.go` | `vfs.ModifyFileMetadata` / `vfs.ModifyDirMetadata` | `DocPatch{Name, DirID}` | WIRED | `move.go:120-123` |
| `mkcol.go` | `vfs.Mkdir` | direct call | WIRED | `mkcol.go:53` |
| `write_integration_test.go` | live Cozy VFS via `newWebdavTestEnv` | `gowebdav.NewClient` + HTTP | WIRED | Uses real `setup.GetTestInstance()` with real VFS, not mocks |

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| WRITE-01 | 02-01 | PUT streaming upload via `vfs.CreateFile`/`io.Copy` | SATISFIED | `put.go:104` uses `io.Copy(file, r.Body)` — no intermediate buffer |
| WRITE-02 | 02-01 | PUT creates or overwrites | SATISFIED | `isOverwrite` branch: `CreateFile(newdoc, olddoc)` — olddoc=nil creates, olddoc=existing overwrites |
| WRITE-03 | 02-01 | PUT supports `If-Match` and `If-None-Match` | SATISFIED | `checkETagPreconditions` called at `put.go:64`; tests `TestPut_IfMatch_Mismatch` (412) and `TestPut_IfNoneMatch_Star_Existing` (412) |
| WRITE-04 | 02-01 | PUT missing parent → 409 Conflict | SATISFIED | `put.go:48-50`: `errors.Is(err, os.ErrNotExist)` → 409; `TestPut_MissingParent` |
| WRITE-05 | 02-02 | DELETE file via `vfs.TrashFile` → 204 | SATISFIED | `delete.go:51`: `vfs.TrashFile(fs, file)` — no DestroyFile; `TestDelete_File` |
| WRITE-06 | 02-02 | DELETE directory via `vfs.TrashDir` → 204 | SATISFIED | `delete.go:53`: `vfs.TrashDir(fs, dir)` — no DestroyDirAndContent; `TestDelete_Directory` |
| WRITE-07 | 02-03 | MKCOL via `vfs.Mkdir` (single level) | SATISFIED | `mkcol.go:53`: `vfs.Mkdir(inst.VFS(), vfsPath, nil)` — not MkdirAll |
| WRITE-08 | 02-03 | MKCOL missing parent → 409 Conflict | SATISFIED | `mkcol.go:58-59`: `errors.Is(err, os.ErrNotExist)` → 409; `TestMkcol_MissingParent` |
| WRITE-09 | 02-03 | MKCOL on existing path → 405 | SATISFIED | `mapVFSWriteError` line 92: `os.ErrExist` → 405; `TestMkcol_AlreadyExists` |
| MOVE-01 | 02-04 | MOVE file via `vfs.ModifyFileMetadata` with `DocPatch` | SATISFIED | `move.go:120`: `vfs.ModifyFileMetadata(fs, srcFile, patch)` with `patch.Name` and `patch.DirID` |
| MOVE-02 | 02-04 | MOVE directory via `vfs.ModifyDirMetadata` | SATISFIED | `move.go:122`: `vfs.ModifyDirMetadata(fs, srcDir, patch)`; `TestMove_RenameDir` |
| MOVE-03 | 02-04 | MOVE absent Overwrite = T (RFC 4918, avoids #66059 bug) | SATISFIED | `move.go:75`: `overwrite := true` initialized; only set false if `ovr == "F"`; `TestMove_OverwriteAbsent_DefaultsToT` |
| MOVE-04 | 02-04 | MOVE Overwrite:F with existing dest → 412 | SATISFIED | `move.go:87-88`: `!overwrite` → 412; `TestMove_OverwriteF_ExistingDest` |
| MOVE-05 | 02-04 | MOVE Destination URL-decoded and traversal-validated | SATISFIED | `parseDestination`: `url.Parse` (URL decodes), then `davPathToVFSPath` (traversal guard); `TestParseDestination_URLDecoded`, `TestParseDestination_TraversalInDest` |
| TEST-03 | 02-05 | gowebdav integration tests per write method, HTTP + VFS state | SATISFIED | `write_integration_test.go`: 9 subtests; each verifies HTTP response AND reads VFS state back via gowebdav; `gowebdav_integration_test.go` Phase 1 tests continue to pass (read surface regression-covered) |

---

### User Decisions Verified in Code

| Decision | Expected Behavior | Code Location | Verified |
|----------|------------------|---------------|---------|
| DELETE uses TrashFile/TrashDir (not DestroyFile) | Soft-trash, recoverable | `delete.go:51-53` — only `vfs.TrashFile`/`vfs.TrashDir` present; no `DestroyFile`/`DestroyDirAndContent` in package | YES |
| DELETE inside .cozy_trash → 405 (not 403) | 405 Method Not Allowed + Allow header | `delete.go:32-34`: `isInTrash` check → 405 with `Allow: PROPFIND, GET, HEAD, OPTIONS` header set | YES |
| MOVE Overwrite absent → T (not F) | Default overwrite | `move.go:75`: `overwrite := true` — default is T | YES |
| MOVE Overwrite:T trashes target first | Recoverable overwrite | `move.go:92-98`: `vfs.TrashFile`/`vfs.TrashDir` called before `ModifyFileMetadata`/`ModifyDirMetadata` | YES |
| MOVE into .cozy_trash → 403 | Trash is system-managed | `move.go:56-59`: `isInTrash(dstPath)` → 403 with audit log | YES |
| `If:` header ignored (no lock-token parsing) | Silent ignore | No `If:` header reading anywhere in `web/webdav/` package — confirmed by grep | YES |
| Quota → 507 | 507 Insufficient Storage | `write_helpers.go:74-76`: `vfs.ErrFileTooBig`/`vfs.ErrMaxFileSize` → `http.StatusInsufficientStorage` (507) with `"quota-not-exceeded"` RFC 4918 §15.10 condition | YES |
| MKCOL with body → 415 | Unsupported Media Type | `mkcol.go:28-37`: `ContentLength > 0` → 415; chunked body also checked via `Body.Read` peek | YES |
| Zero-byte PUT accepted | Creates empty file | `put.go:81`: `size < 0` → `size = -1`; VFS handles; `TestPut_ZeroByte` verifies 201 + Content-Length:0 | YES |

---

### Anti-Patterns Found

| File | Pattern | Severity | Impact |
|------|---------|----------|--------|
| `web/webdav/write_helpers.go:76` | Error condition string `"quota-not-exceeded"` for 507 response | Info | RFC 4918 §15.10 defines `DAV:quota-not-exceeded` as the precondition element name for storage quota exceeded errors — this is the correct RFC name despite being counterintuitive. The HTTP status 507 correctly signals the error; the XML body condition element matches the standard. No functional impact. |
| `web/webdav/handlers.go:46` | Comment says "Phase 2/3 methods — not yet implemented" in the default branch | Info | Minor stale comment now that Phase 2 is complete; only truly Phase 3 methods (COPY) remain as 501. Not a blocker. |

No blockers or warnings found.

---

### Known Carry-Forward (Not a Phase 2 Gap)

**FOLLOWUP-01 (from Phase 1 STATE.md):** Pre-existing `-race` detector race in the test harness (`testutils` setup, outside `web/webdav/` code). The race is in the shared test infrastructure, not in any WebDAV handler logic. This caveat carries forward from Phase 1 and is deferred. It does not affect functional correctness of Phase 2.

---

### Human Verification Required

#### 1. OnlyOffice Mobile End-to-End

**Test:** Connect OnlyOffice mobile app to a Cozy test instance. Open an existing `.odt` or `.docx` document, make a visible edit (add text), save, then verify via `GET /dav/files/<filename>` or PROPFIND that the file content and ETag have changed.

**Expected:** The save operation completes without error in OnlyOffice, and a subsequent WebDAV GET returns the edited content. The ETag in PROPFIND changes to reflect the new file version.

**Why human:** Requires a real OnlyOffice mobile app. The app may issue `If:` header conditions with `Not` clauses, custom `X-OC-*` headers, or a sequence of PROPFIND → GET → LOCK-attempt → PUT that is not reproducible with the gowebdav client. The `TestE2E_WriteOperations/OnlyOffice_OpenEditSave_Flow` test simulates the HTTP surface but cannot replicate the full OnlyOffice client behavior.

---

### Gaps Summary

No gaps. All 5 ROADMAP Phase 2 success criteria verified against the actual codebase. All 15 requirement IDs (WRITE-01..09, MOVE-01..05, TEST-03) have implementation evidence and test coverage. All 9 user decisions from CONTEXT.md are verified in code. The Allow header has been updated to include all write methods. One human-only verification item remains: OnlyOffice mobile real-device test.

---

_Verified: 2026-04-05_
_Verifier: Claude (gsd-verifier)_

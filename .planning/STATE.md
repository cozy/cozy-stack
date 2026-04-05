---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: unknown
last_updated: "2026-04-05T15:10:00.000Z"
progress:
  total_phases: 3
  completed_phases: 0
  total_plans: 9
  completed_plans: 2
---

# Project State: Cozy WebDAV

*This file is the persistent memory of the project. Update it after every work session.*

---

## Project Reference

**Core value:** Un utilisateur peut connecter OnlyOffice mobile ou l'app Fichiers iOS à son Cozy et naviguer, lire, écrire, déplacer et supprimer ses fichiers comme avec n'importe quel stockage cloud WebDAV.

**Repository:** cozy-stack, branch `feat/webdav`
**New package:** `web/webdav/` (to be created)
**Route registration:** `web/routing.go`

**Current focus:** Phase 01 — foundation

---

## Current Position

Phase: 01 (foundation) — EXECUTING
Current Plan: 3 of 9 (Plans 01, 02 complete — scaffold + RED, then XML GREEN)

## Performance Metrics

| Metric | Value |
|--------|-------|
| Phases total | 3 |
| Requirements total | 53 |
| Requirements complete | 5 (TEST-01, TEST-02, TEST-04, READ-05, READ-06) |
| Requirements in progress | 0 |
| Plans created | 9 |
| Plans complete | 2 |

### Plan Execution Log

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| 01-foundation P01 | 3min | 3 | 10 |
| 01-foundation P02 | ~10min | 2 | 3 |

---

## Accumulated Context

### Architecture Decisions

- **No third-party WebDAV server library.** `golang.org/x/net/webdav` has a confirmed MOVE Overwrite bug (#66059, open April 2026) and requires a `LockSystem` that falsely advertises Class 2 compliance. `emersion/go-webdav` adds an 8-method adapter layer with no control over XML. Custom handlers using `encoding/xml` (stdlib) only.
- **New package location:** `web/webdav/` — follows the same pattern as `web/files/`, `web/notes/`, `web/office/`. No `model/webdav/` package needed — handlers delegate directly to `model/vfs/`.
- **Auth strategy:** Bearer token in `Authorization` header OR OAuth token in the Basic Auth password field (username ignored). This reuses `web/middlewares/permissions.go` primitives with a new WebDAV-specific 401 response format (not JSON:API).
- **XML namespace:** `xmlns:D="DAV:"` with `D:` prefix throughout — required for Windows Mini-Redirector compatibility.
- **ETag source:** `vfs.FileDoc.MD5Sum` (content-addressed), always double-quoted. Never `CouchDB _rev` which changes on metadata edits.
- **Date format:** `t.UTC().Format(http.TimeFormat)` (RFC 1123) for `getlastmodified`. Not RFC 3339 — macOS Finder silently misparsed ISO 8601.
- **PROPFIND streaming:** Use `vfs.DirIterator` with `ByFetch: 200`, streaming XML via `encoding/xml.Encoder` directly to the response writer. Cap at 10,000 items.
- **Depth:infinity:** Reject with `403 Forbidden` before any VFS traversal.
- **MOVE Overwrite default:** `overwrite := r.Header.Get("Overwrite") != "F"` — absent header = T, per RFC 4918.
- **MKCOL:** Use `vfs.Mkdir` (single-directory, not `MkdirAll`) to avoid the distributed race condition.
- **PUT streaming:** Pass `r.Body` directly to `vfs.CreateFile` when `r.ContentLength >= 0`. Use temp file for chunked (unknown length) uploads.
- **Content-Length:** Build XML responses in `bytes.Buffer` first, set `Content-Length` from `buf.Len()` before writing the status header.

### Key VFS Functions (confirmed from source inspection)

- `vfs.DirOrFileByPath(fs, path)` — resolve path to DirDoc or FileDoc
- `vfs.ServeFileContent(w, req, file, ...)` — GET/HEAD with Range, ETag, Content-Length
- `vfs.CreateFile(fs, newdoc, olddoc)` — PUT (create or overwrite)
- `vfs.ModifyFileMetadata(fs, olddoc, patch)` — MOVE file (rename + reparent)
- `vfs.ModifyDirMetadata(fs, olddoc, patch)` — MOVE directory
- `vfs.CopyFile(fs, olddoc, newpath)` — COPY file
- `vfs.DestroyFile(fs, doc)` — DELETE file
- `vfs.DestroyDirAndContent(fs, doc)` — DELETE directory recursively
- `vfs.Mkdir(fs, doc, ...)` — MKCOL
- `vfs.DirIterator` / `DirBatch` — streaming PROPFIND Depth:1
- `vfs.Walk(fs, root, fn)` — directory COPY recursion

### Test Infrastructure

- Integration tests: `httptest` + in-memory VFS (`vfsafero`) + `gowebdav` client (new test-only dep)
- Existing test helper: `github.com/gavv/httpexpect/v2` (already in go.mod) for exact HTTP assertions
- TDD methodology: every commit cycle is RED (failing test) → GREEN (minimal code) → REFACTOR (cleanup), each as a separate commit

### Known Research Gaps (address during planning)

- **vfs.DirIterator API shape:** Exact interface should be confirmed against current `model/vfs/couchdb_indexer.go` before implementing PROPFIND pagination. The research identified `DirBatch` and `DirIterator` but did not confirm the exact method signatures.
- **Swift server-side COPY:** Not confirmed whether `vfs/vfsswift` exposes a path to avoid full round-trip for COPY. Must be assessed before Phase 3 COPY handler design.
- **OnlyOffice mobile wire behavior:** No direct wire captures. Mitigation: run OnlyOffice mobile against a staging instance early in Phase 2 validation using mitmproxy.

### Important Non-Decisions (will be decided during implementation)

- GET on a collection: 405 Method Not Allowed OR HTML navigation page — to be decided during Phase 1 planning (READ-10).

### Plan 01-01 Decisions (Scaffold + RED)

- **Internal test package** (`package webdav`, not `webdav_test`) so tests can reach unexported helpers `davPathToVFSPath`, `buildETag`, `parsePropFind`, `marshalMultistatus`.
- **`ResourceType.Collection` as `*struct{}`** so `encoding/xml` omitempty skips `<D:collection/>` for file responses.
- **`ErrPathTraversal` exported sentinel** to enable `errors.Is` checks in future handler code and in the sentinel-error test.
- **gowebdav kept `// indirect`** in go.mod until a future test file imports it (wave 2+).

### Plan 01-02 Decisions (XML GREEN)

- **Response-side struct tags use literal `D:name` prefix** (not Go's `"DAV: name"` namespace form). With a manually-written `<D:multistatus xmlns:D="DAV:">` root, children re-use the prefix by name. The namespace form would cause `encoding/xml` to emit redundant `xmlns="DAV:"` on every child, which Windows Mini-Redirector rejects.
- **Request-side types (`PropFind`, `PropList`) keep the `"DAV: name"` namespace form** because inbound clients may bind `DAV:` to any prefix of their choosing.
- **`Prop.GetContentLength` is plain `int64` with `omitempty`** (not `*int64`), matching the RED test's literal-integer usage.
- **`ResourceType` is a value type** carrying only an optional `Collection *struct{}` — files send `ResourceType{}` (empty), collections send `ResourceType{Collection: &struct{}{}}`. This matches the RED test signature `ResourceType: ResourceType{}`.
- **`SupportedLock` and `LockDiscovery` are named types** (each carrying only `XMLName`) rather than `*struct{}`, matching the RED test's `&SupportedLock{}` instantiation and leaving room for Class 2 extension later.
- **Compile-only `path_mapper.go` stubs** (`davPathToVFSPath`, `ErrPathTraversal`) landed in this plan so the package's test binary builds. Plan 01-03 replaces `davPathToVFSPath` with the real traversal-rejecting implementation; the `ErrPathTraversal` sentinel is already final.
- **RED test bug fix**: `TestGetLastModifiedFormat`'s `assert.NotContains(got, "T")` was self-contradictory (the literal "GMT" contains "T"). Replaced with `assert.NotRegexp(\dT\d)` which targets only the RFC 3339 date/time separator.

---

## Session Continuity

### Last Session

**Date:** 2026-04-05
**Stopped at:** Completed 01-02-PLAN.md (XML GREEN — multistatus types, marshaller, propfind parser)
**Work done:** Executed Plan 02 of Phase 01 — implemented `web/webdav/xml.go` with all 9 types (Multistatus, Response, Propstat, Prop, ResourceType, SupportedLock, LockDiscovery, PropFind, PropList) and 5 helpers (buildETag, buildCreationDate, buildLastModified, parsePropFind, marshalMultistatus). All 7 XML RED tests from Plan 01 now pass. Two commits (b1f47cdc5 feat GREEN, 421d7192f refactor). Four deviations auto-fixed: (1) added compile-only path_mapper.go stubs so the package test binary could build (Rule 3 blocker), (2) fixed self-contradictory NotContains("T") assertion in xml_test.go (Rule 1 bug), (3) reconciled Prop/ResourceType/SupportedLock struct signatures with the RED test's actual usage (Rule 1), (4) switched response struct tags from `"DAV: name"` to literal `"D:name"` to stop encoding/xml emitting redundant xmlns on children (Rule 1).
**Artifacts created:** .planning/phases/01-foundation/01-02-SUMMARY.md
**Artifacts modified:** web/webdav/xml.go, web/webdav/path_mapper.go (stubs), web/webdav/xml_test.go (assertion fix)
**Next action:** Execute Plan 03 (path mapper GREEN — replace davPathToVFSPath stub with real implementation that rejects `..`, encoded traversal, null bytes, encoded slashes, and system-directory prefixes)

### Open Todos

- [ ] Confirm `vfs.DirIterator` / `DirBatch` method signatures from `model/vfs/couchdb_indexer.go` before Phase 1 PROPFIND implementation
- [ ] Decide GET on collection behavior (READ-10) during Phase 1 planning

### Blockers

None.

---

*Last updated: 2026-04-05 after executing Plan 01-02 (XML GREEN)*

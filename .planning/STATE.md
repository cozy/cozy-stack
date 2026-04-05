# Project State: Cozy WebDAV

*This file is the persistent memory of the project. Update it after every work session.*

---

## Project Reference

**Core value:** Un utilisateur peut connecter OnlyOffice mobile ou l'app Fichiers iOS à son Cozy et naviguer, lire, écrire, déplacer et supprimer ses fichiers comme avec n'importe quel stockage cloud WebDAV.

**Repository:** cozy-stack, branch `feat/webdav`
**New package:** `web/webdav/` (to be created)
**Route registration:** `web/routing.go`

**Current focus:** Phase 1 — Foundation (not started)

---

## Current Position

| Field | Value |
|-------|-------|
| Phase | 1 — Foundation |
| Plan | None yet (plans not created) |
| Status | Not started |
| Phase goal | Read-only WebDAV mount: routing, auth, path safety, PROPFIND, GET/HEAD |

**Progress bar:**
```
Phase 1 [          ] 0%
Phase 2 [          ] 0%
Phase 3 [          ] 0%
```

---

## Performance Metrics

| Metric | Value |
|--------|-------|
| Phases total | 3 |
| Requirements total | 53 |
| Requirements complete | 0 |
| Requirements in progress | 0 |
| Plans created | 0 |
| Plans complete | 0 |

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

---

## Session Continuity

### Last Session

**Date:** 2026-04-04
**Work done:** Project initialization — PROJECT.md, REQUIREMENTS.md, research (SUMMARY, ARCHITECTURE, STACK, FEATURES, PITFALLS), codebase structure analysis, ROADMAP.md, STATE.md
**Artifacts created:** All planning documents
**Next action:** Run `/gsd:plan-phase 1` to create the Phase 1 plan

### Open Todos

- [ ] Confirm `vfs.DirIterator` / `DirBatch` method signatures from `model/vfs/couchdb_indexer.go` before Phase 1 PROPFIND implementation
- [ ] Decide GET on collection behavior (READ-10) during Phase 1 planning

### Blockers

None.

---

*Last updated: 2026-04-04 after roadmap creation*

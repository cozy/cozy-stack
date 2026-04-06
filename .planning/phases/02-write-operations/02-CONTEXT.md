# Phase 2: Write Operations - Context

**Gathered:** 2026-04-06
**Status:** Ready for planning

<domain>
## Phase Boundary

Full read-write WebDAV capability: PUT (create/overwrite files with streaming), DELETE (soft-trash), MKCOL (create single directory), MOVE (rename/reparent with Overwrite semantics). End-to-end target: OnlyOffice mobile can connect, open a document, edit it, and save it back.

Out of scope: COPY (Phase 3), LOCK/UNLOCK (never in v1), PROPPATCH/dead properties (v2), litmus compliance (Phase 3), app-specific passwords (v2).

</domain>

<decisions>
## Implementation Decisions

### DELETE semantics — soft-trash, not hard destroy
- DELETE calls `vfs.TrashFile` (files) and `vfs.TrashDir` (directories) — **not** `DestroyFile`/`DestroyDirAndContent`
- This matches Cozy Drive behavior: items go to `.cozy_trash` and are recoverable
- REQUIREMENTS.md WRITE-05/06 reference `DestroyFile`/`DestroyDirAndContent` but those are for items already in trash per the VFS code. The planner/researcher should update to `TrashFile`/`TrashDir` in their task specs
- DELETE on a directory: `TrashDir` trashes the entire tree in one operation — no ErrDirNotEmpty case to handle
- Success response: **204 No Content** (standard RFC 4918)

### DELETE on items inside .cozy_trash/*
- Returns **405 Method Not Allowed** with `Allow: PROPFIND, GET, HEAD, OPTIONS`
- Consistent with Phase 1 decision: `.cozy_trash` is visible but read-only via WebDAV
- Emptying the trash happens via Cozy Drive UI or settings API, never via WebDAV

### PUT — native VFS overwrite, no temp file
- Use `vfs.CreateFile(newdoc, olddoc)` directly: olddoc=nil for create, olddoc=existing for overwrite
- `io.Copy` from request body into the returned `io.WriteCloser` — VFS handles storage atomicity and rollback internally
- Cozy Drive's upload endpoint already uses this pattern
- **Zero-byte PUT accepted** — creates an empty file (OnlyOffice touch, macOS Finder empty file creation)
- **Content-Type**: trust client's `Content-Type` header if present; fall back to extension-based detection if missing or `application/octet-stream`
- **Parent directory missing**: 409 Conflict (WRITE-04)

### PUT — conditional writes
- `If-Match: "etag"` → compare against VFS ETag; mismatch → **412 Precondition Failed**
- `If-None-Match: *` → if file already exists → **412** (prevent create-over-existing)
- **If-Match absent on overwrite**: allow unconditional overwrite (RFC 7232 says absence = "overwrite regardless"). Most WebDAV clients default to this.
- `If:` header with lock tokens: **ignored entirely** (see Lock-token section below)

### MKCOL
- Single directory only via `vfs.Mkdir` (**not** `MkdirAll` — known race condition in cozy-stack per CONCERNS.md)
- Parent directory missing → **409 Conflict** (WRITE-08)
- Path already exists → **405 Method Not Allowed** (WRITE-09)
- MKCOL with request body → **415 Unsupported Media Type** (RFC 4918 §9.3: extended MKCOL not supported)

### MOVE — overwrite and edge cases
- Absent `Overwrite` header treated as **T** (RFC 4918 default, contourns x/net/webdav bug #66059)
- `Overwrite: F` with existing destination → **412 Precondition Failed**
- `Overwrite: T` with existing destination → **trash the target first** (consistent with DELETE=Trash decision), then rename source. User can recover overwritten file from `.cozy_trash`
- Implementation: `vfs.ModifyFileMetadata` with `DocPatch` (new name + new dirID) for files; `vfs.ModifyDirMetadata` for directories
- `Destination` header: URL-decoded and validated via `davPathToVFSPath` (same traversal guards as all read paths)
- MOVE into `.cozy_trash/*` → **403 Forbidden** (trash is system-managed, only DELETE puts things there)
- MOVE of a directory containing shared items → if VFS returns permission/sharing error, surface as **403 Forbidden** with RFC 4918 error XML. No per-child 207 Multi-Status reporting in v1.

### Lock-token handling (If: header)
- `If:` header with lock-token conditions: **ignored entirely** (stripped/skipped)
- Since we advertise `DAV: 1` (no locking), well-behaved clients shouldn't send lock tokens
- macOS Finder sends them anyway — ignoring makes Finder work in write mode
- This is standard behavior for non-locking WebDAV servers (rclone serve webdav, etc.)
- Non-lock-token ETag conditions in `If:` header are also ignored in v1 — `If-Match`/`If-None-Match` are the supported conditional headers

### Audit logging for writes
- **Security-relevant events only at WARN level** — consistent with Phase 1 policy
- Events logged:
  1. Write attempt to `.cozy_trash` (403/405)
  2. Quota exceeded on PUT (507)
  3. Permission denied / sharing violation on MOVE/DELETE (403)
  4. Path traversal on `Destination` header (MOVE) — same as Phase 1's path guard
- **NOT logged**: successful PUT/DELETE/MKCOL/MOVE operations (too noisy; VFS tracks mutations via CouchDB revision history)
- Same structured fields as Phase 1: `instance`, `source_ip`, `user_agent`, `method`, `raw_url`, `normalized_path`, `token_hash`

### Error mapping — VFS errors to HTTP status
- Quota exceeded → **507 Insufficient Storage** (RFC 4918 §9.7.2)
- Parent not found → **409 Conflict**
- Path already exists (MKCOL) → **405 Method Not Allowed**
- Permission / sharing error → **403 Forbidden**
- File/dir not found → **404 Not Found**
- ETag mismatch / If-None-Match violated → **412 Precondition Failed**
- All error responses use RFC 4918 §8.7 XML body via `sendWebDAVError` from Phase 1

### Testing (TDD strict — carried from Phase 1)
- RED → GREEN → REFACTOR cycle with separate commits (non-negotiable)
- Integration tests using `gowebdav` client — each test verifies HTTP response AND observable VFS state
- Never mock the VFS — use test instance with real afero/mem-backed VFS
- `testutil_test.go` harness from Phase 1 reused

### Claude's Discretion
- File split within `web/webdav/` (one file per method? grouped by concern?)
- Exact `DocPatch` construction for MOVE (field names, nil-vs-zero handling)
- Whether to expose `Content-Length` in PUT response (201/204 have no body, but some clients expect it)
- Internal helper structure for Destination header parsing
- How to detect and surface VFS-specific error types (type assertion patterns)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project and requirements
- `.planning/PROJECT.md` — Vision, core value, constraints
- `.planning/REQUIREMENTS.md` — Phase 2 requirements: WRITE-01..09, MOVE-01..05, TEST-03
- `.planning/ROADMAP.md` — Phase 2 success criteria (5 criteria covering PUT, DELETE, MKCOL, MOVE, integration tests)

### Phase 1 context (MANDATORY — decisions carry forward)
- `.planning/phases/01-foundation/01-CONTEXT.md` — Auth, path safety, XML format, error format, audit logging, trash read-only, TDD, all carry forward
- `.planning/phases/01-foundation/01-VERIFICATION.md` — Phase 1 verification results, known caveats

### Research (MANDATORY — read before planning)
- `.planning/research/SUMMARY.md` — Global synthesis
- `.planning/research/STACK.md` — Library choices, gowebdav for tests
- `.planning/research/ARCHITECTURE.md` — VFS integration, auth flow, build order
- `.planning/research/FEATURES.md` — WebDAV methods required per client, compat matrix (especially OnlyOffice mobile write flow)
- `.planning/research/PITFALLS.md` — 20 pitfalls: x/net/webdav Overwrite bug #66059, MkdirAll race condition, macOS Finder lock tokens, quota handling

### Codebase (MANDATORY — understand VFS write primitives)
- `.planning/codebase/ARCHITECTURE.md` — Multi-tenant patterns, VFS interface
- `.planning/codebase/CONCERNS.md` — `vfs.MkdirAll` race condition (why MKCOL uses `vfs.Mkdir` only), other known issues
- `.planning/codebase/TESTING.md` — Test patterns, helpers, instance setup

### VFS write primitives (code references for the planner)
- `model/vfs/vfs.go` — `VFS` interface: `CreateFile`, `Mkdir`, `Remove`, `RemoveAll`
- `model/vfs/file.go:386` — `TrashFile()` (soft-delete for files)
- `model/vfs/directory.go:292` — `TrashDir()` (soft-delete for directories)
- `model/vfs/file.go:251` — `ServeFileContent()` (already used in Phase 1 GET)
- `model/vfs/vfs.go:492` — `Mkdir()` (single-level directory creation)
- `model/vfs/vfs.go:559` — `Remove()` / `RemoveAll()` (inspect — these may be hard-destroy, NOT what we want)
- `web/files/` — Reference upload handler for PUT pattern (CreateFile + io.Copy)

### RFC and external specs
- **RFC 4918** (WebDAV) — §9.3 MKCOL, §9.6 DELETE, §9.7 PUT, §9.9 MOVE, §9.7.2 507 status, §10.4.1 Overwrite header
- **RFC 7232** (Conditional Requests) — If-Match, If-None-Match semantics
- **x/net/webdav bug #66059** — Overwrite header absent defaults to F (wrong per RFC 4918)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `web/webdav/handlers.go` — `handlePath` dispatcher (Phase 1): add PUT/DELETE/MKCOL/MOVE cases to the existing method switch
- `web/webdav/errors.go` — `sendWebDAVError` + `buildErrorXML`: reuse for all error responses (403, 405, 409, 412, 507)
- `web/webdav/path_mapper.go` — `davPathToVFSPath`: reuse for path validation on all write targets + MOVE Destination header
- `web/webdav/auth.go` — `resolveWebDAVAuth`: already wired into the route middleware chain, handles Bearer + Basic
- `web/webdav/testutil_test.go` — `newWebdavTestEnv`: test harness with authenticated client, seeded VFS, ready for write tests
- `model/vfs/file.go:TrashFile` — Soft-delete file (move to .cozy_trash)
- `model/vfs/directory.go:TrashDir` — Soft-delete directory tree
- `web/files/files.go` — Reference implementation of PUT (CreateFile + io.Copy pattern to follow)

### Established Patterns
- **handlePath dispatcher**: method switch in handlers.go — each new method replaces a 501 case (same as plans 01-07/01-08 did for PROPFIND/GET)
- **VFS write pattern**: `vfs.CreateFile(newdoc, olddoc)` returns `io.WriteCloser`, caller does `io.Copy(w, r.Body)` then `w.Close()` — this is the canonical Cozy write flow
- **Error handling**: VFS functions return typed errors (`os.ErrNotExist`, `os.ErrExist`, custom `ErrXxx`); handlers map to HTTP status

### Integration Points
- `web/webdav/handlers.go` — Add PUT, DELETE, MKCOL, MOVE cases to handlePath switch
- No changes needed to `web/routing.go` — all methods already route through handlePath
- No changes needed to `model/vfs/` — all write primitives exist

</code_context>

<specifics>
## Specific Ideas

- **DELETE = Trash**: The most important decision — deviates from a naïve reading of REQUIREMENTS.md but matches Cozy's safety-first philosophy. The planner MUST use `TrashFile`/`TrashDir` instead of `DestroyFile`/`DestroyDirAndContent`.
- MOVE Overwrite=T uses trash-then-rename, not destroy-then-rename — consistent with DELETE=Trash and gives users a recovery path for overwritten files.
- `.cozy_trash` is a write-forbidden zone via WebDAV: 405 for DELETE inside trash, 403 for MOVE-to-trash. Only the system (via DELETE on non-trash paths) populates it.
- Lock-token `If:` headers are silently ignored — no parsing, no evaluation. This is a pragmatic compatibility choice for Finder and OnlyOffice.
- OnlyOffice mobile's open→edit→save flow is the end-to-end integration target. The gowebdav E2E test from Phase 1 should be extended with write scenarios.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. All Phase 1 deferred items (app-specific passwords, LOCK/UNLOCK, PROPPATCH, quota properties, metrics, rate limiting, alerting, Digest Auth, PROPFIND cap) remain deferred to v2.

</deferred>

---

*Phase: 02-write-operations*
*Context gathered: 2026-04-06*

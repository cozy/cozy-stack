# Project Research Summary

**Project:** Cozy WebDAV
**Domain:** WebDAV server layer on top of a non-filesystem Go VFS abstraction (CouchDB + Swift)
**Researched:** 2026-04-04
**Confidence:** HIGH (stack and architecture from direct codebase inspection; features from RFC 4918 + community sources; pitfalls from official issue trackers and CVEs)

## Executive Summary

Building a WebDAV server on cozy-stack is well-scoped and achievable. The VFS abstraction already provides all the primitives needed (directory listing, file read/write, rename/move, copy, delete), and the Echo routing framework supports arbitrary HTTP methods. The implementation lives entirely in a new `web/webdav/` package — thin protocol-translation handlers that map WebDAV verbs to existing `model/vfs/` functions. No new business logic layer is required. The correct approach is to write custom `echo.HandlerFunc` handlers using stdlib `encoding/xml` directly, bypassing both `golang.org/x/net/webdav` and `emersion/go-webdav` whose interfaces are incompatible with Cozy's VFS API (and carry known open bugs).

The target feature set for v1 is RFC 4918 Class 1 compliance: OPTIONS, PROPFIND (Depth 0 and 1), GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE, and PROPPATCH. This covers OnlyOffice mobile fully and generic WebDAV clients broadly. LOCK/UNLOCK is explicitly out of scope — this means macOS Finder mounts read-only, which is acceptable and must be documented. The Nextcloud-compatible `/remote.php/webdav` redirect enables the OnlyOffice built-in Nextcloud preset to work without configuration changes.

The critical risks are: (1) a cluster of correctness traps baked into the RFC (Overwrite default behavior, ETag quoting, RFC 1123 date format, XML namespace prefixing) that are easy to get wrong and break specific clients silently; (2) two security issues that must be addressed in Phase 1 (path traversal sanitization, Depth:infinity DoS); and (3) streaming discipline for large files (PUT must not buffer the body in RAM). All of these are known and preventable with explicit implementation decisions made early.

## Key Findings

### Recommended Stack

Write custom handlers using `encoding/xml` (stdlib) only. The only new dependency is `github.com/studio-b12/gowebdav` v0.12.0 for integration test clients. All server-side logic uses stdlib and existing go.mod dependencies.

Both candidate third-party server libraries are ruled out: `golang.org/x/net/webdav.Handler` has a known unfixed MOVE Overwrite bug (issue #66059, open April 2026), requires a `LockSystem` that would falsely advertise Class 2 compliance, and its `FileSystem` interface is POSIX-only and incompatible with Cozy's VFS. `emersion/go-webdav` v0.7.0 has a better interface design but still requires an 8-method adapter layer, gives no control over XML generation for custom properties, and introduces a new dependency for marginal gain.

**Core technologies:**
- `encoding/xml` (stdlib): RFC 4918 XML marshalling — ~150 lines for complete PROPFIND structs; full control over namespace prefixing and property formatting
- `net/http` + Echo v4.15.1 (already in go.mod): HTTP routing via `e.Match()` for non-standard WebDAV methods; no wrapper needed
- `model/vfs/` (existing): all VFS operations delegated directly — `DirOrFileByPath`, `CreateFile`, `ModifyFileMetadata`, `MoveDir`, `CopyFile`, `DirIterator`
- `github.com/studio-b12/gowebdav` v0.12.0 (new, test-only): black-box integration test client
- `github.com/gavv/httpexpect/v2` (already in go.mod): exact HTTP-level assertions for WebDAV-specific scenarios

### Expected Features

**Must have (table stakes) — v1 launch:**
- `OPTIONS` with `DAV: 1` and full `Allow:` header — clients check this first
- `PROPFIND` Depth:0 and Depth:1, allprop/prop/propname variants, 207 Multi-Status XML — core of WebDAV
- Live properties: `resourcetype`, `getlastmodified` (RFC 1123), `getcontentlength`, `getetag` (strong, quoted), `getcontenttype`, `displayname`, `creationdate`
- `GET`/`HEAD` with ETag, Last-Modified, Content-Length on every response
- `PUT` with streaming body (no buffering), chunked Transfer-Encoding support
- `DELETE` recursive on collections
- `MKCOL` create directory
- `COPY` and `MOVE` with correct Overwrite default (absent header = T)
- `PROPPATCH` returning 207/403 (RFC Class 1 compliance; live props return 403)
- 401 + `WWW-Authenticate: Basic realm="Cozy"` for unauthenticated requests
- Redirect `/remote.php/webdav` → `/dav/files` 301 (OnlyOffice Nextcloud preset)
- Namespace-prefixed XML (`xmlns:D="DAV:"` with `D:` prefix) — required by Windows Mini-Redirector

**Should have — v1.x after validation:**
- ETag-based conditional requests (`If-Match`, `If-None-Match`) — prevents lost-update for sync clients
- `X-OC-Mtime` header support — rclone mtime preservation
- Explicit `Depth: infinity` → 403 with RFC-compliant error body

**Defer to v2+:**
- LOCK/UNLOCK — macOS Finder read-write; requires Cozy-level file locking mechanism; major architectural addition
- CalDAV/CardDAV — separate protocol and project
- iOS native Files app (File Provider Extension) — requires separate iOS app development

**Anti-features (never build):**
- `DAV: 1, 2` in OPTIONS without real locking — false capability claim
- `quota-available-bytes`/`quota-used-bytes` — leaks billing data
- Nextcloud `oc:*` custom properties — not required, adds maintenance burden
- Anonymous/public WebDAV access — security perimeter

### Architecture Approach

The implementation is a pure protocol-translation layer: a new `web/webdav/` package containing 6 files, no `model/webdav/` package. Handlers are thin — they resolve paths, check permissions, delegate to the existing VFS, and render RFC 4918 XML. The package follows the same structure as `web/files/`, `web/notes/`, etc. Auth reuses `web/middlewares/permissions.go` primitives (`GetRequestToken`, `ParseJWT`, `GetForOauth`) with a new WebDAV-specific wrapper that emits 401 + `WWW-Authenticate` instead of JSON:API errors.

**Major components:**
1. `web/webdav/auth.go` — `resolveWebDAVAuth` middleware: Basic Auth password or Bearer token → `*permission.Permission`; emits WebDAV-appropriate 401 on failure
2. `web/webdav/path_mapper.go` — `davPathToVFSPath()`: URL-decode, `path.Clean`, validate against hidden system dirs (`.cozy_trash`, `.cozy_apps`, etc.)
3. `web/webdav/handlers.go` — method dispatcher and all 10 handler functions; delegates to VFS directly
4. `web/webdav/xml.go` — RFC 4918 structs for PROPFIND request parsing and 207 Multi-Status response generation (~150 lines)
5. `web/webdav/errors.go` — maps VFS errors to WebDAV HTTP status codes
6. `web/webdav/webdav_test.go` — integration tests using `httptest` + in-memory VFS + `gowebdav` client

PROPFIND pagination uses the existing `vfs.DirIterator` with `ByFetch: 200` pages, streaming XML via `encoding/xml.Encoder` directly to the response writer, capped at 10,000 items. MOVE delegates to `vfs.ModifyFileMetadata`/`ModifyDirMetadata` (recursive path update handled by `MoveDir`). COPY uses `vfs.CopyFile` for files; directory COPY requires manual `vfs.Walk` + `CopyFile` per file.

### Critical Pitfalls

1. **Path traversal via URL encoding** — `%2e%2e/` and double-encoded variants bypass naive prefix checks. Mitigation: `url.PathUnescape` then `path.Clean("/" + decoded)` in `path_mapper.go`; never concatenate URL segments directly; add table-driven test covering `../`, `%2e%2e/`, `%252e`, null bytes. Address in Phase 1.

2. **Depth:infinity PROPFIND DoS** — full VFS tree traversal on a large account; hundreds of MB, minutes of CPU. Mitigation: reject with `403 Forbidden` before any directory traversal; cap Depth:1 at 10,000 items. Address in Phase 1.

3. **ETag semantics: wrong source, wrong format** — `CouchDB _rev` is not content-addressed and changes on metadata edits; bare ETag strings without quotes break conditional requests. Mitigation: derive ETag from `vfs.FileDoc.MD5Sum`; always return as `"<hash>"`; test that ETag is stable across metadata-only updates. Address in Phase 1.

4. **MOVE Overwrite default** — RFC 4918 requires absent `Overwrite` header to mean T; natural coding instinct checks `== "T"` (which fails when absent); this is also the confirmed open bug in `x/net/webdav` (#66059). Mitigation: `overwrite := r.Header.Get("Overwrite") != "F"`. Address in Phase 2.

5. **Date format: RFC 1123 required, RFC 3339 wrong** — macOS Finder silently misparses ISO 8601 dates in `getlastmodified`. Mitigation: always use `t.UTC().Format(http.TimeFormat)`. Address in Phase 1.

6. **PUT body buffering** — `io.ReadAll(r.Body)` in PUT handler allocates full file in heap. Mitigation: pass `r.Body` directly to `vfs.CreateFile` when `r.ContentLength >= 0`; use temp file for chunked uploads. Address in Phase 2.

7. **MKCOL race condition** — `vfs.MkdirAll` has no distributed lock; concurrent MKCOL can create duplicate CouchDB documents. Mitigation: use `vfs.Mkdir` (single-directory, not recursive), let CouchDB conflict signal 405. Address in Phase 2.

8. **Content-Length on all responses** — macOS/iOS clients produce "strange results" without it. Mitigation: build XML responses in `bytes.Buffer` first, set `Content-Length` from `buf.Len()` before writing header. Address in Phase 1.

## Implications for Roadmap

Based on combined research, a 3-phase structure emerges from the dependency graph and the security-first principle.

### Phase 1: Foundation — Routing, Auth, PROPFIND, and Read Operations

**Rationale:** Authentication, path mapping, security guards, and the PROPFIND XML engine are dependencies for everything else. GET/HEAD can be implemented as thin wrappers over the existing `vfs.ServeFileContent`. All critical correctness decisions (ETag strategy, date format, XML namespace, Content-Length policy, Depth:infinity rejection, path traversal prevention) must be made here and baked into the handler template used by later phases.

**Delivers:** A working read-only WebDAV mount. Any WebDAV client can connect, authenticate, browse the directory tree, and download files.

**Implements:**
- Route registration in `web/routing.go`; `web/webdav/` package scaffold
- `resolveWebDAVAuth` middleware (Basic + Bearer → permission)
- `path_mapper.go` with path traversal protection and hidden-dir blocking
- OPTIONS handler — `DAV: 1`, full `Allow:` list, no auth required
- PROPFIND — Depth:0, Depth:1, allprop/prop/propname; streaming XML; Depth:infinity → 403
- GET + HEAD — reuse `vfs.ServeFileContent`; ensure Content-Length set
- `xml.go` — all RFC 4918 structs with correct namespace prefix
- `errors.go` — VFS → HTTP status mapping
- 401 + `WWW-Authenticate: Basic` on auth failure
- `web/webdav/webdav_test.go` — integration test scaffold with `gowebdav` client

**Avoids:** Pitfalls 1 (Depth:infinity DoS), 2 (path traversal), 3 (HTTPS enforcement), 4 (OPTIONS leakage), 5 (ETag semantics), 7 (date format), 10 (Content-Length), 11 (trailing slash), 12 (PROPFIND memory), 17 (brittle XML tests)

**Research flag:** Standard patterns — no additional research needed.

---

### Phase 2: Write Operations — PUT, DELETE, MKCOL, MOVE, PROPPATCH

**Rationale:** Write operations share the auth and path infrastructure from Phase 1 but introduce new correctness concerns: conditional headers on PUT, MKCOL race safety, MOVE Overwrite semantics. Group together because they share the "mutating operation" pattern and the same permission check (PATCH/POST VFS permission).

**Delivers:** Full read-write WebDAV capability. OnlyOffice mobile can connect and edit files end-to-end. The `/remote.php/webdav` redirect enables the Nextcloud preset.

**Implements:**
- PUT — streaming body (`r.Body` → `vfs.CreateFile`); chunked PUT via temp file; ETag on response
- DELETE — files and recursive collections via `vfs.DestroyFile`/`TrashFile`
- MKCOL — single `vfs.Mkdir`; CouchDB conflict → 405; concurrent race test
- MOVE — `vfs.ModifyFileMetadata`/`MoveDir`; correct Overwrite default (`!= "F"`); permission checks on both source and destination
- PROPPATCH — 207 with 403 for live props; skeleton for future dead props
- `/remote.php/webdav` 301 redirect

**Avoids:** Pitfalls 6 (If-Match/If-None-Match), 8 (MOVE Overwrite default), 13 (PUT body buffering), 14 (MKCOL race)

**Research flag:** Standard patterns — no additional research needed.

---

### Phase 3: COPY and Compliance Hardening

**Rationale:** COPY is separate because it raises a distinct performance concern (Swift server-side copy vs. full round-trip), and directory COPY requires `vfs.Walk` recursion. Compliance hardening (conditional requests, litmus test suite, rclone compatibility) is grouped here as it requires a working Phase 2 baseline to validate against.

**Delivers:** Complete RFC 4918 Class 1 compliance. Passes `litmus` test suite. rclone sync works correctly. COPY/MOVE tested end-to-end with OnlyOffice mobile and a WebDAV compliance client.

**Implements:**
- COPY — file COPY via `vfs.CopyFile`; directory COPY via `vfs.Walk` + `CopyFile`; assess Swift server-side copy via `X-Copy-From`
- Conditional requests: `If-Match`/`If-None-Match` middleware (pre-PUT guard)
- `X-OC-Mtime` header support for rclone
- Explicit Depth:infinity → 403 with RFC-compliant `DAV:propfind-finite-depth` condition element
- litmus compliance test run in CI (Docker)
- Security review: OPTIONS caps, HTTPS enforcement in non-localhost envs

**Avoids:** Pitfalls 15 (COPY full round-trip), 16 (MOVE CouchDB case-sensitivity), 6 (conditional headers)

**Research flag:** Phase 3 may benefit from research into Swift server-side copy API availability through the existing VFS Swift backend before committing to COPY handler design.

---

### Phase Ordering Rationale

- Phase 1 before Phase 2: auth, path mapping, and XML infrastructure are shared foundations; all correctness traps that affect multiple phases must be resolved first
- PROPFIND before PUT: read-only validation (litmus, gowebdav client) catches foundational issues before write complexity is added
- COPY in Phase 3: its performance question (Swift server-side copy) is independent of correctness and should not block the write operations in Phase 2
- Compliance hardening in Phase 3: requires a complete baseline to test against

### Research Flags

Phases needing deeper research during planning:
- **Phase 3:** Swift server-side copy feasibility — check whether `vfs/vfsswift` can expose an `X-Copy-From` path; if not, document COPY performance limitations for v1

Phases with standard patterns (skip research-phase):
- **Phase 1:** RFC 4918 is well-documented; Go stdlib XML is well-understood; architecture is directly derived from existing `web/files/` patterns
- **Phase 2:** Write operations follow the same VFS delegation pattern; MOVE semantics are fully specified in research

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | `x/net/webdav` bug confirmed from open issue tracker; VFS API confirmed from direct source inspection; Echo routing pattern verified from `web/routing.go` |
| Features | MEDIUM | RFC 4918 is authoritative; client-specific behaviors (OnlyOffice, iOS) derived from community reports and help docs, not wire captures |
| Architecture | HIGH | Derived entirely from direct inspection of the cozy-stack source tree; VFS method signatures confirmed; existing middleware integration points identified |
| Pitfalls | HIGH (security/correctness), MEDIUM (client quirks) | Security pitfalls from CVEs and RFC; correctness pitfalls from confirmed open bugs; client quirks from community reports |

**Overall confidence:** HIGH

### Gaps to Address

- **OnlyOffice mobile wire behavior**: No direct wire captures of what OnlyOffice iOS/Android actually sends. Documented behavior is inferred from help docs and community issues. Mitigation: run OnlyOffice mobile against a staging instance early in Phase 2 validation and capture actual requests with a proxy (mitmproxy).
- **emersion/go-webdav Overwrite parsing**: Unverified whether `emersion/go-webdav` parses the absent `Overwrite` header correctly. Moot if custom handlers are used (recommended), but worth noting if the decision is revisited.
- **Swift server-side copy**: Not confirmed whether the VFS Swift backend exposes a path that avoids full round-trip for COPY. Must be assessed before Phase 3 COPY handler design.
- **vfs.DirIterator API shape**: ARCHITECTURE.md identifies `DirBatch` and `DirIterator` as existing primitives. The exact interface should be confirmed against current `model/vfs/couchdb_indexer.go` before implementing PROPFIND pagination in Phase 1.

## Sources

### Primary (HIGH confidence)
- [RFC 4918](https://www.rfc-editor.org/rfc/rfc4918.html) — authoritative WebDAV standard
- [golang/go issue #66059](https://github.com/golang/go/issues/66059) — MOVE Overwrite header bug in `x/net/webdav`, open April 2026
- [golang/net webdav.go source](https://github.com/golang/net/blob/master/webdav/webdav.go) — confirmed COPY uses `!= "F"`, MOVE uses `== "T"`
- cozy-stack source tree (direct inspection) — VFS interface, middleware, routing patterns, go.mod
- [pkg.go.dev/github.com/emersion/go-webdav](https://pkg.go.dev/github.com/emersion/go-webdav) — v0.7.0 FileSystem interface
- [pkg.go.dev/github.com/studio-b12/gowebdav](https://pkg.go.dev/github.com/studio-b12/gowebdav) — v0.12.0 client API
- [OnlyOffice DesktopEditors issue #349](https://github.com/ONLYOFFICE/DesktopEditors/issues/349) — confirms no LOCK needed for OnlyOffice
- CVE-2023-39143 — PaperCut WebDAV path traversal (CVSS 8.4)

### Secondary (MEDIUM confidence)
- [sabre/dav Finder client quirks](https://sabre.io/dav/clients/finder/) — Content-Length requirement, RFC 1123 date requirement
- [sabre/dav Windows client quirks](https://sabre.io/dav/clients/windows/) — XML namespace prefix requirement
- [Nextcloud WebDAV API docs](https://docs.nextcloud.com/server/20/developer_manual/client_apis/WebDAV/basic.html) — property set used in practice
- [OnlyOffice community issues](https://community.onlyoffice.com/) — iOS auth compatibility
- [deepwiki.com/emersion/go-webdav](https://deepwiki.com/emersion/go-webdav/2.1-webdav-server) — server architecture
- Nextcloud bugs #14428, #37605 — conditional header implementation errors

---
*Research completed: 2026-04-04*
*Ready for roadmap: yes*

# Roadmap: Cozy WebDAV

**Project:** Cozy WebDAV
**Milestone:** v1 — RFC 4918 Class 1 compliance for OnlyOffice mobile and iOS Files
**Granularity:** Coarse (3 phases)
**Coverage:** 53/53 v1 requirements mapped

---

## Phases

- [x] **Phase 1: Foundation** — Read-only WebDAV mount: routing, auth, path safety, PROPFIND, GET/HEAD
- [x] **Phase 2: Write Operations** — Full read-write capability: PUT, DELETE, MKCOL, MOVE
- [ ] **Phase 3: COPY, Compliance, and Documentation** — RFC 4918 Class 1 sign-off: COPY, litmus, docs

---

## Phase Details

### Phase 1: Foundation
**Goal**: Any WebDAV client can connect to a Cozy instance, authenticate, browse the directory tree, and download files. All security guards and correctness invariants (ETag, date format, XML namespace, path traversal prevention, Content-Length policy) are baked in before any write operation is attempted.
**Depends on**: Nothing — this is the first phase
**Requirements**: ROUTE-01, ROUTE-02, ROUTE-03, ROUTE-04, ROUTE-05, AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, READ-01, READ-02, READ-03, READ-04, READ-05, READ-06, READ-07, READ-08, READ-09, READ-10, SEC-01, SEC-02, SEC-03, SEC-04, SEC-05, TEST-01, TEST-02, TEST-04
**Success Criteria** (what must be TRUE when this phase is complete):
  1. A WebDAV client (gowebdav in tests) connecting to `/dav/files/` with a valid Bearer token can browse the user's `/files/` tree. PROPFIND Depth:0 and Depth:1 return a valid `207 Multi-Status` with all 9 live properties: `resourcetype`, `getlastmodified` (RFC 1123), `getcontentlength`, `getetag` (double-quoted md5sum), `getcontenttype`, `displayname`, `creationdate`, `supportedlock` (empty), `lockdiscovery` (empty). XML uses `xmlns:D="DAV:"` with `D:` prefix.
  2. An unauthenticated request to any method except OPTIONS returns `401 Unauthorized` with `WWW-Authenticate: Basic realm="Cozy"`. An OPTIONS request succeeds without authentication and returns `DAV: 1` and the full `Allow:` list.
  3. A PROPFIND with `Depth: infinity` returns `403 Forbidden`. Any path containing `../`, `%2e%2e/`, null bytes, or a system directory prefix (`/settings`, `/apps`, etc.) is rejected — the VFS is never called.
  4. A GET request on a file streams the file content with correct `Content-Length`, `ETag`, and `Last-Modified` headers. Range requests work. A GET on a collection returns `405 Method Not Allowed`.
  5. A client using `/remote.php/webdav/*` is redirected 308 to the equivalent `/dav/files/*` path and the subsequent request succeeds.
**Plans**: 9 plans
- [x] 01-01-PLAN.md — Scaffold web/webdav package + gowebdav dep + RED tests for XML & path mapper
- [x] 01-02-PLAN.md — GREEN: XML multistatus structs, D: namespace, RFC 1123 dates, ETag helpers
- [x] 01-03-PLAN.md — GREEN: davPathToVFSPath with traversal prevention
- [x] 01-04-PLAN.md — RED+GREEN: RFC 4918 §8.7 error XML builder
- [x] 01-05-PLAN.md — Shared test helpers + RED+GREEN: auth middleware (Bearer, Basic password, 401 realm, audit log)
- [x] 01-06-PLAN.md — RED+GREEN: Routes registration, OPTIONS handler, Nextcloud 308 redirect, routing.go wiring
- [x] 01-07-PLAN.md — RED+GREEN+REFACTOR: PROPFIND Depth 0/1/infinity with DirIterator streaming
- [x] 01-08-PLAN.md — RED+GREEN: GET/HEAD via ServeFileContent, GET on collection → 405
- [x] 01-09-PLAN.md — End-to-end gowebdav integration test + Phase 1 verification (shipped with `-race` caveat; harness race deferred — see STATE.md FOLLOWUP-01)

---

### Phase 2: Write Operations
**Goal**: A user can create, update, move, and delete files and directories through the WebDAV interface. OnlyOffice mobile can connect, open a document, edit it, and save it back end-to-end.
**Depends on**: Phase 1
**Requirements**: WRITE-01, WRITE-02, WRITE-03, WRITE-04, WRITE-05, WRITE-06, WRITE-07, WRITE-08, WRITE-09, MOVE-01, MOVE-02, MOVE-03, MOVE-04, MOVE-05, TEST-03
**Success Criteria** (what must be TRUE when this phase is complete):
  1. A PUT request streams the body directly to the VFS — no full-body buffering. The file is created if it does not exist and overwritten if it does. `If-Match` and `If-None-Match` conditional headers are honored: a mismatched ETag returns `412 Precondition Failed`. A PUT where the parent directory does not exist returns `409 Conflict`.
  2. DELETE on a file removes it from the VFS. DELETE on a directory removes it and all its contents recursively.
  3. MKCOL creates a single directory. MKCOL on an already-existing path returns `405 Method Not Allowed`. MKCOL where the parent does not exist returns `409 Conflict`.
  4. MOVE renames or reparents a file or directory. An absent `Overwrite` header is treated as `T` (RFC 4918 default — the bug in `x/net/webdav` #66059 does not affect this implementation). `Overwrite: F` with an existing destination returns `412`. The Destination header is URL-decoded and validated against path traversal before any VFS call.
  5. Integration tests using `gowebdav` cover PUT, DELETE, MKCOL, and MOVE — each test verifies both the HTTP response code and the observable VFS state (file exists / does not exist, directory contents).
**Plans**: 5 plans
- [x] 02-01-PLAN.md — TDD RED+GREEN: Shared write helpers (mapVFSWriteError, isInTrash) + PUT handler
- [x] 02-02-PLAN.md — TDD RED+GREEN: DELETE handler with soft-trash semantics
- [x] 02-03-PLAN.md — TDD RED+GREEN: MKCOL handler (single directory creation)
- [x] 02-04-PLAN.md — TDD RED+GREEN: MOVE handler with Destination parsing and Overwrite semantics
- [x] 02-05-PLAN.md — Update Allow header + E2E gowebdav write integration tests

---

### Phase 3: COPY, Compliance, and Documentation
**Goal**: The implementation achieves full RFC 4918 Class 1 compliance verified by the `litmus` test suite. COPY completes the method set. The feature is documented for users and operators and ready for production.
**Depends on**: Phase 2
**Requirements**: COPY-01, COPY-02, COPY-03, DOC-01, DOC-02, DOC-03, DOC-04, TEST-05, TEST-06, TEST-07
**Success Criteria** (what must be TRUE when this phase is complete):
  1. COPY on a file creates a replica at the destination via `vfs.CopyFile`. COPY on a directory recursively copies all contents via `vfs.Walk` + `CopyFile`. COPY respects `Overwrite` semantics identically to MOVE (absent = T, `Overwrite: F` with existing destination = 412).
  2. The `litmus` WebDAV compliance suite (RFC 4918 Class 1) runs in CI and passes all tests with no failures.
  3. An end-to-end scenario test covering open → read → write → save passes against a live test Cozy instance using the same auth flow as OnlyOffice mobile and the iOS Files scenario.
  4. `docs/` contains a description of all supported methods and their behavior, configuration examples for OnlyOffice mobile and iOS Files, and compatibility notes (macOS Finder read-only without LOCK, locking not supported in v1, Depth:infinity blocked).
**Plans**: 10 plans
- [ ] 03-01-PLAN.md — TDD RED+GREEN: handleCopy file mode + dispatcher wiring (+ note.CopyFile branch)
- [ ] 03-02-PLAN.md — TDD RED+GREEN: handleCopy directory mode via vfs.Walk + 207 Multi-Status
- [ ] 03-03-PLAN.md — E2E gowebdav SuccessCriterion6_Copy integration sub-test
- [ ] 03-04-PLAN.md — scripts/webdav-litmus.sh + Makefile test-litmus target (dual-route orchestration)
- [ ] 03-05-PLAN.md — Litmus `basic` suite green on both routes (RED+GREEN per gap)
- [ ] 03-06-PLAN.md — Litmus `copymove` suite green on both routes (RED+GREEN per gap)
- [ ] 03-07-PLAN.md — Litmus `props` suite green (PROPPATCH strategy decision: A / B / C)
- [ ] 03-08-PLAN.md — Litmus `http` suite green (Expect: 100-continue)
- [ ] 03-09-PLAN.md — docs/webdav.md + docs/toc.yml entry (English, narrative+table+inline-curl)
- [ ] 03-10-PLAN.md — Update .planning/REQUIREMENTS.md for iOS Files deferral to v1.1

---

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation | 9/9 | Complete (with deferred `-race` follow-up, FOLLOWUP-01) | 2026-04-05 |
| 2. Write Operations | 5/5 | Complete | 2026-04-06 |
| 3. COPY, Compliance, and Documentation | 6/10 | In Progress|  |

---

## Coverage Map

| Requirement | Phase |
|-------------|-------|
| ROUTE-01 | Phase 1 |
| ROUTE-02 | Phase 1 |
| ROUTE-03 | Phase 1 |
| ROUTE-04 | Phase 1 |
| ROUTE-05 | Phase 1 |
| AUTH-01 | Phase 1 |
| AUTH-02 | Phase 1 |
| AUTH-03 | Phase 1 |
| AUTH-04 | Phase 1 |
| AUTH-05 | Phase 1 |
| READ-01 | Phase 1 |
| READ-02 | Phase 1 |
| READ-03 | Phase 1 |
| READ-04 | Phase 1 |
| READ-05 | Phase 1 |
| READ-06 | Phase 1 |
| READ-07 | Phase 1 |
| READ-08 | Phase 1 |
| READ-09 | Phase 1 |
| READ-10 | Phase 1 |
| SEC-01 | Phase 1 |
| SEC-02 | Phase 1 |
| SEC-03 | Phase 1 |
| SEC-04 | Phase 1 |
| SEC-05 | Phase 1 |
| TEST-01 | Phase 1 |
| TEST-02 | Phase 1 |
| TEST-04 | Phase 1 |
| WRITE-01 | Phase 2 |
| WRITE-02 | Phase 2 |
| WRITE-03 | Phase 2 |
| WRITE-04 | Phase 2 |
| WRITE-05 | Phase 2 |
| WRITE-06 | Phase 2 |
| WRITE-07 | Phase 2 |
| WRITE-08 | Phase 2 |
| WRITE-09 | Phase 2 |
| MOVE-01 | Phase 2 |
| MOVE-02 | Phase 2 |
| MOVE-03 | Phase 2 |
| MOVE-04 | Phase 2 |
| MOVE-05 | Phase 2 |
| TEST-03 | Phase 2 |
| COPY-01 | Phase 3 |
| COPY-02 | Phase 3 |
| COPY-03 | Phase 3 |
| DOC-01 | Phase 3 |
| DOC-02 | Phase 3 |
| DOC-03 | Phase 3 |
| DOC-04 | Phase 3 |
| TEST-05 | Phase 3 |
| TEST-06 | Phase 3 |
| TEST-07 | Phase 3 |

Phase 1: 28 requirements
Phase 2: 15 requirements
Phase 3: 10 requirements
Total: 53 / 53 — all v1 requirements mapped

---

*Roadmap created: 2026-04-04*
*Last updated: 2026-04-06 after planning Phase 2 (5 plans across 4 waves)*

---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
current_plan: 5 of 9 (Plans 01, 02, 03, 04 complete — scaffold+RED, XML GREEN, path mapper GREEN, error XML builder)
status: unknown
stopped_at: Completed 01-04-PLAN.md (RFC 4918 error XML builder — buildErrorXML + sendWebDAVError)
last_updated: "2026-04-05T14:43:39.887Z"
progress:
  total_phases: 3
  completed_phases: 0
  total_plans: 9
  completed_plans: 4
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
Current Plan: 5 of 9 (Plans 01, 02, 03, 04 complete — scaffold+RED, XML GREEN, path mapper GREEN, error XML builder)

## Performance Metrics

| Metric | Value |
|--------|-------|
| Phases total | 3 |
| Requirements total | 53 |
| Requirements complete | 9 (TEST-01, TEST-02, TEST-04, READ-05, READ-06, ROUTE-03, ROUTE-05, SEC-02, SEC-05) |
| Requirements in progress | 0 |
| Plans created | 9 |
| Plans complete | 4 |

### Plan Execution Log

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| 01-foundation P01 | 3min | 3 | 10 |
| 01-foundation P02 | ~10min | 2 | 3 |
| 01-foundation P03 | ~2min | 2 | 1 |
| 01-foundation P04 | ~1min | 2 | 2 |

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

### Plan 01-03 Decisions (Path Mapper GREEN)

- **Reject any residual `%` character** in the raw URL wildcard, not just `%2e`/`%2f` substrings. Since Echo has already URL-decoded the wildcard once before our handler sees it, any surviving `%` is either a double encoding (`%252e%252e` → `%2e%2e` after one decode) or a smuggling attempt. This is a strict superset of the plan's reference check and passes the double-encoded test case, which the plan's `%2e`/`%2f`-substring check would miss (substring `%2e` does not appear in `%252e`).
- **Anchor the wildcard under `/files` before `path.Clean`.** Prepending `/files/` and then asserting the cleaned result is `/files` or begins with `/files/` turns any `..`-walk that escapes the WebDAV URL space into a rejection, reusing `path.Clean`'s semantics instead of re-implementing them.
- **Single `ErrPathTraversal` sentinel for every rejection path** (null byte, encoded escape, scope escape). Callers do one `errors.Is` check and log/respond uniformly — no error-type matrix.
- **Skipped the REFACTOR commit** per Task 2's explicit authorisation. After Task 1, `path_mapper.go` is 65 lines, the public function is 24 lines, `containsEncodedTraversal` is already extracted with load-bearing doc, and `gofmt -l` is empty — no further change warranted.

### Plan 01-04 Decisions (Error XML Builder — RED+GREEN)

- **Build the error body as a 3-fragment string write into `bytes.Buffer`**, not via `encoding/xml.Marshal`. Plan 01-02 had to fight `encoding/xml` to keep the `D:` prefix stable on multistatus children (the namespace form leaks `xmlns="DAV:"` on every child). For a fixed 2-element body, direct string writes are simpler, faster, and avoid re-importing that problem entirely.
- **No XML escaping of the `condition` argument.** Condition names are RFC 4918-defined identifiers (`propfind-finite-depth`, `lock-token-submitted`, `forbidden`, …) — code constants, never user input. The invariant is documented in the doc comment.
- **`sendWebDAVError` is the single entry point for every non-2xx WebDAV response.** Plans 05 (auth 401), 06 (router 405/404), 07 (PROPFIND 403/404/507), 08 (GET 404/403/500), and all Phase 2/3 handlers must route through it so the Content-Length + Content-Type + XML shape invariants stay uniform.
- **Use `echo.HeaderContentType` / `echo.HeaderContentLength` constants** rather than raw header strings, matching the convention of the rest of cozy-stack's Echo handlers.

---

## Session Continuity

### Last Session

**Date:** 2026-04-05
**Stopped at:** Completed 01-04-PLAN.md (RFC 4918 error XML builder — `buildErrorXML` + `sendWebDAVError`)
**Work done:** Executed Plan 04 of Phase 01 — RED+GREEN for the WebDAV error XML builder. Created `web/webdav/errors_test.go` with three tests (`TestBuildErrorXML_PropfindFiniteDepth`, `TestBuildErrorXML_Forbidden`, `TestSendWebDAVError_HeadersAndStatus`) asserting body shape, HTTP status, `Content-Type: application/xml; charset="utf-8"`, byte-exact `Content-Length`, and presence of the `D:` namespaced condition element. Replaced the Plan 01-01 stub in `web/webdav/errors.go` with `buildErrorXML(condition) []byte` (3-fragment `bytes.Buffer` write — XML prolog, `<D:error xmlns:D="DAV:"><D:{cond}/>`, `</D:error>`) and `sendWebDAVError(c echo.Context, status int, condition string) error` which sets Content-Type + Content-Length before `WriteHeader` (SEC-05). Two commits: `bd3c8bb27` (test RED), `e4e592adb` (feat GREEN). All 3 tests pass, full package suite still green (no regressions in Plans 01/02/03), `gofmt -l` empty, `go vet` clean. Zero deviations — plan executed exactly as written. SEC-05 marked complete.
**Artifacts created:** web/webdav/errors_test.go, .planning/phases/01-foundation/01-04-SUMMARY.md
**Artifacts modified:** web/webdav/errors.go (stub replaced by real implementation), .planning/STATE.md, .planning/ROADMAP.md, .planning/REQUIREMENTS.md
**Next action:** Execute Plan 05 (auth middleware — first consumer of `sendWebDAVError`; see ROADMAP.md wave map)

### Open Todos

- [ ] Confirm `vfs.DirIterator` / `DirBatch` method signatures from `model/vfs/couchdb_indexer.go` before Phase 1 PROPFIND implementation
- [ ] Decide GET on collection behavior (READ-10) during Phase 1 planning

### Blockers

None.

---

*Last updated: 2026-04-05 after executing Plan 01-04 (error XML builder — RED+GREEN)*

---
gsd_state_version: 1.0
milestone: v1.1
milestone_name: milestone
status: executing
stopped_at: "Completed 03-04-PLAN.md"
last_updated: "2026-04-12T17:05:24Z"
progress:
  total_phases: 3
  completed_phases: 2
  total_plans: 24
  completed_plans: 15
---

# Project State: Cozy WebDAV

*This file is the persistent memory of the project. Update it after every work session.*

---

## Project Reference

**Core value:** Un utilisateur peut connecter OnlyOffice mobile ou l'app Fichiers iOS à son Cozy et naviguer, lire, écrire, déplacer et supprimer ses fichiers comme avec n'importe quel stockage cloud WebDAV.

**Repository:** cozy-stack, branch `feat/webdav`
**New package:** `web/webdav/` (to be created)
**Route registration:** `web/routing.go`

**Current focus:** Phase 03 — copy-compliance-and-documentation

---

## Current Position

Phase: 03 (copy-compliance-and-documentation) — EXECUTING
Plan: 5 of 10

## Performance Metrics

| Metric | Value |
|--------|-------|
| Phases total | 3 |
| Requirements total | 53 |
| Requirements complete | 43 (TEST-01, TEST-02, TEST-03, TEST-04, READ-01, READ-02, READ-03, READ-04, READ-05, READ-06, READ-07, READ-08, READ-09, READ-10, ROUTE-01, ROUTE-02, ROUTE-03, ROUTE-04, ROUTE-05, SEC-01, SEC-02, SEC-03, SEC-04, SEC-05, AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, WRITE-01, WRITE-02, WRITE-03, WRITE-04, WRITE-05, WRITE-06, WRITE-07, WRITE-08, WRITE-09, MOVE-01, MOVE-02, MOVE-03, MOVE-04, MOVE-05) |
| Requirements in progress | 0 |
| Plans created | 9 |
| Plans complete | 14 |

### Plan Execution Log

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| 01-foundation P01 | 3min | 3 | 10 |
| 01-foundation P02 | ~10min | 2 | 3 |
| 01-foundation P03 | ~2min | 2 | 1 |
| 01-foundation P04 | ~1min | 2 | 2 |
| 01-foundation P05 | ~3min | 3 | 4 |
| 01-foundation P06 | ~4min | 3+1 | 5 |
| 01-foundation P08 | ~1.5min | 2 | 3 |
| 01-foundation P07 | ~6min | 3 | 3 |
| 01-foundation P09 | ~5min | 2 | 2 |
| 02-write-operations P01 | 3min | 2 | 5 |
| 02-write-operations P02 | 2min | 2 | 3 |
| 02-write-operations P03 | 3min | 2 | 3 |
| 02-write-operations P04 | 9min | 2 | 5 |
| 02-write-operations P05 | 2min | 2 | 3 |
| 03-copy-compliance P04 | 5min | 2 | 2 |

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
- **DELETE = soft-trash:** Uses `vfs.TrashFile`/`vfs.TrashDir` (not Destroy). DELETE on `.cozy_trash` paths returns 405 with Allow header listing read-only methods (distinct from PUT which returns 403).
- **MKCOL missing parent:** `vfs.Mkdir` calls `fs.DirByPath(parentDir)` which returns `os.ErrNotExist` (not `ErrParentDoesNotExist`) for missing parents. handleMkcol intercepts this to return 409 Conflict before generic mapVFSWriteError (which would map it to 404).

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

### Plan 01-05 Decisions (Auth Middleware + Test Utilities)

- **401 responses bypass `sendWebDAVError`.** RFC 4918 §8.7 precondition bodies are for authenticated-rejection failures (propfind-finite-depth, forbidden, …), not for auth gating. Clients (Finder, gowebdav, WinMR, OnlyOffice) read `WWW-Authenticate` and ignore the body. `sendWebDAV401` sets the header and calls `c.NoContent(401)` — zero body bytes, fewer moving parts, no spurious condition name outside the RFC vocabulary. `sendWebDAVError` remains canonical for every OTHER non-2xx in plans 06+.
- **`overrideRoutes` is required (not optional) in `newWebdavTestEnv` until plan 01-06.** The plan text suggested defaulting to `Routes` when nil, but referencing the undefined `Routes` identifier in testutil_test.go would cause a compile error. Helper `t.Fatal`s when nil. One-line flip once plan 01-06 lands.
- **`TestAuth_401IsNotLogged` dropped, SEC-04 enforced by code inspection.** Capturing `pkg/logger` output has no existing test seam and logrus global output redirection races with parallel tests. The auditLog doc comment forbids 401 calls, and `resolveWebDAVAuth` structurally has zero auditLog calls on its 401 paths. Todo filed: re-introduce once logger gains a capture seam.
- **`hashToken` = first 8 bytes of sha256 (16 hex chars).** ~72B tokens before 50% birthday collision — more than enough for any realistic audit window, grep-friendly in log viewers.
- **`auditLog` takes raw event string + normalizedPath, not a typed struct.** Event vocabulary is small and fixed at call sites in plans 06-08. A typed wrapper would add indirection with no type safety (go's string gives the compiler nothing to check on enum-like strings).
- **Integration tests mount middleware in isolation via `overrideRoutes(mountAuthOnly)`.** The shared `mountAuthOnly(g *echo.Group)` registers `resolveWebDAVAuth` + a trivial 200 "ok" handler on `/files` and `/files/*`, exercising the middleware without depending on plan 01-06's Routes or plans 07+ handlers. This pattern will be reused (with different trivial handlers) through Phase 2/3.

### Plan 01-08 Decisions (GET/HEAD via ServeFileContent)

- **Delegate everything to `vfs.ServeFileContent`.** It wraps `http.ServeContent`, which handles HEAD (headers only), Range (206 + Content-Range), If-Modified-Since, If-None-Match, and Content-Length automatically. Re-implementing any of those would duplicate stdlib code, diverge from `web/files/files.go` (which uses the same primitive), and accumulate maintenance debt as Go tightens ServeContent semantics. `handleGet`'s happy path is literally one line — the other ~30 are error-class branching.
- **405 (not 403) on GET of a collection, with `Allow: OPTIONS, PROPFIND, HEAD`.** RFC 7231 §6.5.5 explicitly ties 405 to "method not supported by target resource" — our exact case. 405 also requires an Allow header, which is how Finder and gowebdav discover which methods do work on the collection. 403 would leave clients blind. READ-10 decision locked in.
- **Unified 'forbidden' XML body for traversal AND out-of-scope failures, but different audit events.** Both produce 403 with the same client-visible shape, so an attacker probing one cannot distinguish it from the other. The audit WARN log carries different event strings (`get path rejected` vs `get out-of-scope`) so ops can tell them apart internally.
- **Pass `nil` version to `ServeFileContent`.** Phase 1 is current-version-only; WebDAV has no wire-level version concept. Single line to change if a future phase ever adds version-aware reads via a Cozy-specific header.
- **REFACTOR pass skipped.** `handleGet` is 35 LoC including doc comment, each error branch is distinct with no duplicate fragments to extract, and `gofmt -l` is empty. Plan explicitly permits skipping when the handler is already compact.

### Plan 01-06 Decisions (Route Wiring + OPTIONS + handlePath Dispatcher)

- **`Routes` splits OPTIONS from everything else via a sub-group, not via conditional logic in the middleware.** `router.OPTIONS(...)` is registered on the outer group directly (no auth), then `authed := router.Group("", resolveWebDAVAuth)` scopes auth to every other verb. Single place to enforce the bypass; `resolveWebDAVAuth` itself still has its OPTIONS fast-path as defence-in-depth but the route table alone is now sufficient.
- **`handlePath` is a method-switch dispatcher with every case collapsing to `sendWebDAVError(501, "not-implemented")` today.** Plans 01-07 (PROPFIND) and 01-08 (GET/HEAD) replace exactly one case body each — zero restructuring of the route table. Phase 2/3 write-verbs replace the default branch. Keeps the plumbing stable across ~10 future plans.
- **`NextcloudRedirect` is exported from day one** (plan text hinted at lowercase→rename in Task 3). Package-internal tests don't care either way, and exporting immediately eliminates a rename commit + second test edit. Rule 3 blocking triviality.
- **`MS-Author-Via: DAV` added to `handleOptions`** in addition to the plan's `DAV: 1` + `Allow`. Research §236-272 flags it as required for Windows Mini-Redirector to upgrade the connection; omitting it would silently break every Windows client.
- **`strings.Replace(path, "/remote.php/webdav", "/dav/files", 1)` with explicit n=1** protects against pathological paths containing the literal substring (e.g. a filename named `remote.php/webdav`). The second occurrence must NOT be rewritten.
- **308 Permanent Redirect, not 301/302, for the Nextcloud bridge.** 301/302 allow clients to downgrade the method to GET; 308 preserves it, which is the whole point (PROPFIND and PUT must not become GETs).
- **WebDAV middleware chain in `web/routing.go` is deliberately narrower than the JSON:API chain.** No `LoadSession` (WebDAV is token-only, no cookies), no `Accept` (XML content-type is non-negotiable), no `CheckTOSDeadlineExpired` (a WebDAV client can't render the HTML). Chain: `NeedInstance` + `CheckInstanceBlocked` + `CheckInstanceDeleting` — the strict minimum for instance resolution + lifecycle gating.
- **`newWebdavTestEnv(t, nil)` now defaults to `Routes`.** Flipped the plan 01-05 stop-gap `t.Fatal` now that `Routes` exists. Unblocks plans 01-07/08/09 and Phase 2/3 from writing their own mount callbacks when they want the full router; explicit registrars (`mountAuthOnly`, `mountRealRoutes`) still work. Clears a STATE.md todo.

### Plan 01-07 Decisions (PROPFIND Handler — RED+GREEN+REFACTOR)

- **Absent Depth header defaults to "1", not infinity.** RFC 4918 technically specifies infinity as the default for PROPFIND when Depth is omitted, but every real-world client (Finder, gowebdav, Nextcloud, WinMR) sets Depth explicitly. Mapping absent→1 defensively avoids accidental full-tree crawls and is observationally indistinguishable from spec-literal behaviour for every client we care about.
- **Depth:infinity rejected BEFORE path mapping.** The 403 `<D:propfind-finite-depth/>` denial runs before `davPathToVFSPath`, so a crawler probing with Depth:infinity cannot amplify its reach via path enumeration before hitting the gate. The raw wildcard (not normalised path) is fed to `auditLog` to preserve intrusion-detection signal.
- **Directory ETag synthesised from md5(DocID || UpdatedAt.UnixNano).** DirDocs don't have a VFS md5sum. We tried omitting getetag entirely for directories (RFC 4918 permits this) but Finder and gowebdav both cache directory metadata by ETag — a stable synthetic value produces better change-detection than an absent property. 8-byte BigEndian encoding of UnixNano appended to DocID, md5'd, then buildETag'd so the format matches files exactly.
- **`marshalMultistatus` buffers; doesn't stream.** Plan 02's marshaller returns `[]byte`, which lets handlePropfind set Content-Length before WriteHeader — required by SEC-05 and macOS/iOS client strictness. Streaming via `xml.Encoder` would force chunked encoding. The memory trade-off is bounded by DirIterator(ByFetch: 200): a 10k-file directory holds ~10k `Response` structs (~2MB), which is acceptable. The CouchDB query itself is still streamed batch-by-batch.
- **AllowVFS takes `vfs.Fetcher`, not `permission.Fetcher`.** The plan's `<interfaces>` block had this wrong. `vfs.Fetcher` embeds `permission.Fetcher` plus `parentID()`, `Path(FilePather)`, `Parent(VFS)`. Both `*DirDoc` and `*FileDoc` satisfy it (asserted at model/vfs/vfs.go:21-22).
- **RED-test assertion loosening in GREEN phase (not goalpost-moving).** Two assertions rejected valid XML outputs from `encoding/xml`: (a) `<D:collection></D:collection>` long form is semantically identical to `<D:collection/>` self-closing under XML 1.0 §3.1; encoding/xml emits the long form for non-nil empty-struct pointers. (b) encoding/xml escapes ETag surrounding quotes as `&#34;` inside element text — valid XML, clients decode entities. Regex alternations accept either form. Plan 02's existing `TestResourceTypeCollectionVsFile` already uses the `Contains(body, "D:collection")` substring form so the RED tests were inconsistent with established output shape.
- **REFACTOR was NOT skipped (unlike plan 03).** GREEN had ~10 lines of duplicated Prop construction and scattered trailing-slash logic. Extracted `baseProps(name, createdAt, updatedAt)` (5-field shared set), `hrefForDir` + `hrefForFile` (URL-space rules), and `propstatOK` const (canonical "HTTP/1.1 200 OK"). Phase 2/3 MKCOL/MOVE/COPY response bodies will reuse `baseProps` verbatim — the extraction earns its keep downstream.

### Plan 01-04 Decisions (Error XML Builder — RED+GREEN)

- **Build the error body as a 3-fragment string write into `bytes.Buffer`**, not via `encoding/xml.Marshal`. Plan 01-02 had to fight `encoding/xml` to keep the `D:` prefix stable on multistatus children (the namespace form leaks `xmlns="DAV:"` on every child). For a fixed 2-element body, direct string writes are simpler, faster, and avoid re-importing that problem entirely.
- **No XML escaping of the `condition` argument.** Condition names are RFC 4918-defined identifiers (`propfind-finite-depth`, `lock-token-submitted`, `forbidden`, …) — code constants, never user input. The invariant is documented in the doc comment.
- **`sendWebDAVError` is the single entry point for every non-2xx WebDAV response.** Plans 05 (auth 401), 06 (router 405/404), 07 (PROPFIND 403/404/507), 08 (GET 404/403/500), and all Phase 2/3 handlers must route through it so the Content-Length + Content-Type + XML shape invariants stay uniform.
- **Use `echo.HeaderContentType` / `echo.HeaderContentLength` constants** rather than raw header strings, matching the convention of the rest of cozy-stack's Echo handlers.

### Plan 01-09 Decisions (End-to-End Gowebdav Gate + Phase 1 Sign-Off)

- **One consolidated `TestE2E_GowebdavClient` test with 5 explicitly-named subtests — one per ROADMAP success criterion.** `SuccessCriterion1_BrowseWithBearerToken`, `SuccessCriterion2_AuthRequiredExceptOptions`, `SuccessCriterion3_SecurityGuards`, `SuccessCriterion4_GetFileAndCollection`, `SuccessCriterion5_NextcloudRedirect`. The subtest name IS the requirement it verifies — grep-friendly audit trail, no separate cross-reference table to maintain. All 5 green on first run because every earlier wave had already delivered the pieces; the test exists to prove that the pieces compose correctly end-to-end through a real WebDAV client.
- **Mixed client strategy inside one test file.** The gowebdav client (`studio-b12/gowebdav.NewClient(url, "", token)`) drives criterion 1 because the whole point is "does a real WebDAV client work against this stack?". Criteria 2-5 use raw `httpexpect` because they assert on precise HTTP-level details (status codes, headers, redirect chains) that gowebdav abstracts away. `httpexpect.DontFollowRedirects` is essential for criterion 5 — without it the 308 is invisible.
- **gowebdav promoted from `// indirect` to direct dep in `go.mod`.** Plan 01-01 kept it indirect until a non-test file imported it; plan 01-09 Task 1 is the first test file that imports it directly, so the indirect marker is now wrong. `go mod tidy` removes it automatically as part of the Task 1 commit.
- **Ship Phase 1 with an explicit `-race` caveat; defer the harness race.** User decision at the plan 01-09 Task 2 checkpoint. `go test ./web/webdav/... -race -count=1` fails with ~6 WARNING: DATA RACE reports, but every race is between `pkg/config/config.UseViper` (called by `config.UseTestFile` in test N setup) and `config.FsURL` (read by the `AntivirusTrigger` goroutine launched by `stack.Start` in test N-1). The race is reproducible on `master` without any Phase 1 code and its fix surface is entirely in non-webdav packages (`pkg/config/config`, `model/job`, `model/stack`, `tests/testutils`). Blocking Phase 1 merge on a pre-existing stack-wide bug would be scope creep. Phase 1 ships as `nyquist_compliant: true` with a caveat; the race is filed in "Deferred Follow-ups" below as `01.1-race-harness` (provisional — may become a Phase 2 prerequisite instead).
- **VALIDATION.md frontmatter `nyquist_compliant: true` carries a `nyquist_caveat` field.** Standard frontmatter has a binary flag; we extended it with a prose caveat so future phase-plan scans can detect the deferred invariant without opening the file. Do NOT claim `-race` is clean anywhere in the phase artifacts — the caveat is the authoritative statement.
- **Race fix NOT attempted in plan 01-09.** Per user instruction, this plan's job was to close Phase 1, not to fix the race. The race is filed as a separate hardening task. Preferred fix order (most local first): (a) `t.Cleanup` hook in `testutils.TestSetup` calling `stack.Shutdown`; (b) `sync.RWMutex` around `pkg/config/config` globals; (c) per-test context on `memScheduler.StartScheduler`.

---

## Deferred Follow-ups

Items discovered during Phase 1 execution that are out of scope for Phase 1 but must be resolved before the affected invariant can be re-asserted. These are NOT blockers for Phase 1 merge — they are explicit deferrals with user approval.

### FOLLOWUP-01 — Test-harness data race under `-race` (provisional slot: `01.1-race-harness`)

**Status:** Executing Phase 03
**Blocks:** The `-race` invariant for any package that uses `testutils.NewSetup` + `GetTestInstance` more than once in the same `go test -race` process. Currently affects `web/webdav/` (exposed for the first time by plan 01-09's final sweep) and any other package doing the same stacking pattern.
**Discovered in:** Plan 01-09 Task 2 (final race-enabled sweep).
**Fully analysed in:** `.planning/phases/01-foundation/01-VALIDATION.md` → "Outstanding Gaps" → "Gap 1 — Pre-existing test-infrastructure race under `-race`".

**Root cause (one paragraph):** `testutils.GetTestInstance` calls `stack.Start`, which spawns a `memScheduler` goroutine that owns an `AntivirusTrigger` reading `config.FsURL()` on a long-lived timer. That goroutine outlives the test that started it. The NEXT test's setup calls `config.UseTestFile`, which mutates `pkg/config/config` globals via `config.UseViper`. The write in test N's setup races with the read in test N-1's still-running antivirus scheduler. Reproducible on `master` without any WebDAV code, reproducible after removing `gowebdav_integration_test.go` — the race is entirely in the stack-wide test fixture, not in Phase 1 code.

**Files involved (all non-webdav):**

- `pkg/config/config/config.go` (write: `UseViper` line 1009, read: `FsURL` line 475)
- `model/job/trigger_antivirus.go` (`AntivirusTrigger.pushJob` line 102 — the reader)
- `model/job/mem_scheduler.go` (`memScheduler.StartScheduler` line 59 — owns the leaked goroutine)
- `model/stack/main.go` (`Start` line 104 — launches the scheduler)
- `tests/testutils/test_utils.go` (`TestSetup.GetTestInstance` line 178 — no teardown hook)

**Preferred fix (smallest blast radius first):**

1. Add a `t.Cleanup` hook in `testutils.TestSetup` (or a new `testutils.NewSetup` teardown) that calls `stack.Shutdown` or an equivalent scheduler-stop before the next test can mutate `config.*`. Touches one file in `tests/testutils/`.
2. Wrap `pkg/config/config` package globals in a `sync.RWMutex` so concurrent read/write is safe by construction. Touches one file in `pkg/config/config/` but affects every reader/writer.
3. Make `memScheduler.StartScheduler` respect a per-test context so the goroutine exits with the test that started it. Larger surface — touches the job scheduler lifecycle.

**Verification when resolved:** `go test ./web/webdav/... -race -count=1 -timeout 5m` exits 0 with zero `WARNING: DATA RACE` reports. Re-run against every Phase 1 test file — all should be race-clean. Then flip VALIDATION.md to drop the `nyquist_caveat` line.

**Disposition decision point:** at the next phase transition (Phase 1 → Phase 2), user will decide whether this becomes a new decimal phase `01.1-race-harness` (runs before Phase 2) OR is rolled in as Phase 2 Task 0 (test-harness hardening prerequisite). Do not create a new phase directory until that decision is made.

---

## Session Continuity

### Last Session

**Date:** 2026-04-05
**Stopped at:** Phase 3 context gathered
**Work done (01-07):** Wave 4 — ran in parallel with plan 01-08 (GET/HEAD). 3 atomic commits. (1) `da0a46a36` test: `web/webdav/propfind_test.go` with 7 RED integration tests — `TestPropfind_Depth0_Root` (single D:response, D:collection marker, trailing-slash href), `TestPropfind_Depth0_File` (content-length=14, ETag regex, RFC 1123 getlastmodified regex), `TestPropfind_Depth1_DirectoryWithChildren` (seed /Docs + 3 files, assert 4 D:response elements), `TestPropfind_DepthInfinity_Returns403` (403 + propfind-finite-depth body), `TestPropfind_NonexistentPath_Returns404`, `TestPropfind_NamespacePrefixInBody` (xmlns:D="DAV:" + D: prefix + no leaked default-namespace), `TestPropfind_AllNineLiveProperties` (all 9 live prop element names present). Local `seedDir` helper wraps `vfs.Mkdir`; `seedFile` reused from get_test.go (same package). Confirmed RED: all 7 failed against the 501 stub from plan 06. (2) `10f89e168` feat: created `web/webdav/propfind.go` (~230 lines) — `handlePropfind` (Depth parse → davPathToVFSPath → DirOrFileByPath → AllowVFS → build responses → marshalMultistatus with Content-Length), `streamChildren` (DirIterator ByFetch=200, appends Response per child without buffering full listing), `buildResponseForDir` / `buildResponseForFile` (9 live props each), `etagForDir` (md5(DocID || UpdatedAt.UnixNano) since DirDocs have no VFS md5sum). Targeted `Edit` on `handlers.go` replaced only the PROPFIND case body (`return handlePropfind(c)`) — preserved plan 08's GET/HEAD hunk already committed at `accd13500`, no merge conflict. Two RED-test assertions loosened during GREEN (bug in the assertions, not the implementation): `<D:collection></D:collection>` long form and `&#34;`-escaped ETag quotes both accepted — both are valid XML per §3.1 and plan 02's own tests use the same looser substring form. All 7 tests green; full package suite 5.5s; `gofmt`/`go vet`/`go build ./...` clean. (3) `9b7ad1e1e` refactor: extracted `hrefForDir`/`hrefForFile` (trailing-slash URL rules), `baseProps(name, createdAt, updatedAt)` (5 fields shared by files and dirs), `propstatOK` const. Net ~8 line reduction; Phase 2/3 MKCOL/MOVE/COPY will reuse `baseProps` verbatim. Two deviations logged: (a) Rule 1 — AllowVFS takes `vfs.Fetcher` not `permission.Fetcher` (plan's interfaces block was wrong); (b) Rule 1 — 2 RED assertions too strict for valid XML output. 9 requirements completed: READ-01..07 (full PROPFIND surface — Depth 0/1, all 9 live props, RFC-compliant formats), SEC-03 (Depth:infinity DoS prevention), SEC-04 (audit logging of infinity + out-of-scope — already counted in plan 05 but exercised here).
**Artifacts created (01-07):** web/webdav/propfind.go, web/webdav/propfind_test.go, .planning/phases/01-foundation/01-07-SUMMARY.md
**Artifacts modified (01-07):** web/webdav/handlers.go (PROPFIND case body), .planning/STATE.md, .planning/ROADMAP.md, .planning/REQUIREMENTS.md

**Work done (01-08):** Wave 4 — ran in parallel with plan 01-07 (PROPFIND). 2 atomic commits. (1) `975404c79` test: `web/webdav/get_test.go` with 6 RED integration tests — `TestGet_File_ReturnsContent` (body + Content-Length=14 + non-empty Etag), `TestHead_File_NoBody` (same headers, empty body), `TestGet_File_RangeRequest` (Range: bytes=0-4 → 206 + Content-Range: bytes 0-4/14 + body="Hello"), `TestGet_Collection_Returns405` (GET /dav/files/ → 405 + Allow contains OPTIONS/PROPFIND/HEAD), `TestGet_Nonexistent_Returns404`, `TestGet_Unauthenticated_Returns401` (WWW-Authenticate: Basic realm="Cozy"). Local `seedFile(t, inst, name, content)` helper wires `vfs.NewFileDoc` + `fs.CreateFile` + `io.Copy` — pattern from `model/vfs/vfs_test.go:76-96`. Confirmed RED: 4 tests failed with 501 from the plan 01-06 stub, the 401 test was already green (auth runs before dispatcher). (2) `accd13500` feat: created `web/webdav/get.go` (58 lines) with `handleGet` — `davPathToVFSPath(c.Param("*"))` → `inst.VFS().DirOrFileByPath(vfsPath)` → 404 on `os.ErrNotExist` → 405 + Allow on dirDoc → `middlewares.AllowVFS(c, permission.GET, fileDoc)` → `vfs.ServeFileContent(inst.VFS(), fileDoc, nil, "", "", c.Request(), c.Response())`. Traversal and out-of-scope errors return 403 via `sendWebDAVError` + `auditLog` WARN with distinct event strings ("get path rejected" vs "get out-of-scope"). Targeted `Edit` on `web/webdav/handlers.go` replaced only the GET|HEAD case body (`return handleGet(c)`) to avoid merge conflict with parallel plan 01-07's PROPFIND case edit — no dispatcher restructuring. All 6 tests green in 1.56s; full `web/webdav` package regression in 3.91s; `gofmt -l`, `go vet`, `go build ./...` all clean. No deviations — plan executed exactly as written. REFACTOR pass skipped per plan authorisation (handleGet already 35 LoC, one decision branch per error class, gofmt empty). 3 requirements completed: READ-08 (GET streaming via ServeFileContent), READ-09 (HEAD headers-only), READ-10 (405 on collection).
**Work done (01-06):** 3 task commits + 1 refactor commit. (1) `f3b07465a` test: `web/webdav/options_test.go` with 6 RED integration tests — 3× OPTIONS variations (root, subpath, nonsensical path) proving no-auth 200 + DAV: 1 + Allow header, and 3× Nextcloud redirect variations (GET, root path, PROPFIND method preservation) proving 308 with correct Location. Also 2 helpers (`mountRealRoutes` passthrough + `registerNextcloudRedirect` that mounts the redirect directly on `env.TS.Config.Handler.(*echo.Echo)` since httptest/setup.GetTestServer scopes to /dav). Used `httpexpect.DontFollowRedirects` (grep'd from web/auth/auth_test.go). Compile-failed on `undefined: Routes` and `undefined: NextcloudRedirect`. (2) `b71d86087` feat: replaced `webdav.go` stub with 62-line GREEN — `davAllowHeader` const, `Routes(g)` that registers OPTIONS without auth and wraps every other method under `router.Group("", resolveWebDAVAuth)` + `authed.Match(webdavMethods[1:], ...)`, and exported `NextcloudRedirect` using `strings.Replace(path, "/remote.php/webdav", "/dav/files", 1)` + `http.StatusPermanentRedirect`. Replaced `handlers.go` 1-line stub with 41-line GREEN — `handleOptions` (DAV: 1, Allow: OPTIONS, PROPFIND, GET, HEAD, MS-Author-Via: DAV, `c.NoContent(200)`, zero VFS access) + `handlePath` method switch dispatcher returning `sendWebDAVError(501, "not-implemented")` for all cases (plans 07/08 replace PROPFIND and GET/HEAD branches). All 6 tests flip GREEN in 1.44s; full package suite green in 2.5s. (3) `7c023c2ed` feat: wired into `web/routing.go` SetupRoutes — added `web/webdav` import in alphabetical order in the web/* group, added a new route block between the JSON:API block and the non-auth block with its own narrow `mwsWebDAV = [NeedInstance, CheckInstanceBlocked, CheckInstanceDeleting]` chain (no LoadSession, no Accept, no TOS check — WebDAV is token-only, XML-only, non-HTML), `webdav.Routes(router.Group("/dav", mwsWebDAV...))`, and `router.Match(webdavRedirectMethods, "/remote.php/webdav[/*]", webdav.NextcloudRedirect, mwsWebDAV...)` on both root and wildcard. `go build ./...` clean, `go vet ./web/...` clean, gofmt clean. (4) `a53950f65` refactor: flipped `newWebdavTestEnv` `overrideRoutes` from required (`t.Fatal` when nil) to optional (defaults to `Routes`) now that plan 01-06 provides it — clears the matching STATE.md todo and unblocks plans 01-07/08/09 + Phase 2/3 from writing their own mount callbacks. Existing callers (`mountAuthOnly` in auth_test.go, `mountRealRoutes` in options_test.go) unchanged. Three deviations logged: (a) Rule 3 — exported NextcloudRedirect from day one (skip rename churn); (b) Rule 3 — MS-Author-Via header added (required for Windows Mini-Redirector per research); (c) follow-on refactor clearing overrideRoutes todo. 4 requirements completed: ROUTE-01 (route registration), ROUTE-02 (OPTIONS discovery), ROUTE-04 (Nextcloud 308 bridge). ROUTE-05 (custom 401 format) was already complete from plan 01-05.
**Work done (01-05):** Executed Plan 05 of Phase 01 — 3 atomic commits. (1) `d3599f79b` test: `web/webdav/testutil_test.go` with `webdavTestEnv` struct and `newWebdavTestEnv(t, overrideRoutes)` wiring `config.UseTestFile` + `testutils.NewSetup`/`GetTestInstance`/`GetTestClient(consts.Files)`/`GetTestServer("/dav", overrideRoutes)` + `errors.ErrorHandler` + `CreateTestClient`. (2) `ef16ad861` test: `web/webdav/auth_test.go` with 5 RED integration tests (`TestAuth_MissingAuthorization_Returns401WithBasicRealm`, `TestAuth_BearerToken_Success`, `TestAuth_BasicAuthTokenAsPassword_Success`, `TestAuth_InvalidToken_Returns401`, `TestAuth_OptionsBypassesAuth`) + shared `mountAuthOnly` registrar. (3) `42abd4c79` feat: replaced Plan 01-01's auth.go stub with 80-line GREEN impl — `resolveWebDAVAuth` (OPTIONS bypass → GetRequestToken → ParseJWT → ForcePermission → next), `sendWebDAV401` (WWW-Authenticate: Basic realm="Cozy", empty body), `hashToken` (sha256 first 8 bytes hex), `auditLog` (WARN with source_ip, user_agent, method, raw_url, normalized_path, token_hash, instance — forbidden on 401 paths per SEC-04). All 5 tests pass in 1.34s with `COZY_COUCHDB_URL=http://admin:password@localhost:5984/`; full package suite green; gofmt clean; go vet clean. Three deviations logged: (a) Rule 3 — `overrideRoutes` made required (not optional) because referencing the undefined `Routes` identifier would fail compile; (b) planned drop of `TestAuth_401IsNotLogged` per plan text authorisation, SEC-04 enforced by code inspection + doc comment; (c) env-var auth gate for CouchDB admin creds documented in docs/CONTRIBUTING.md. 7 requirements marked complete (AUTH-01..05, SEC-01, SEC-04).
**Artifacts created (01-05):** web/webdav/testutil_test.go, web/webdav/auth_test.go, .planning/phases/01-foundation/01-05-SUMMARY.md
**Artifacts modified (01-05):** web/webdav/auth.go, .planning/STATE.md, .planning/ROADMAP.md, .planning/REQUIREMENTS.md
**Artifacts created (01-06):** web/webdav/options_test.go, .planning/phases/01-foundation/01-06-SUMMARY.md
**Artifacts modified (01-06):** web/webdav/webdav.go, web/webdav/handlers.go, web/webdav/testutil_test.go, web/routing.go, .planning/STATE.md, .planning/ROADMAP.md, .planning/REQUIREMENTS.md
**Artifacts created (01-08):** web/webdav/get.go, web/webdav/get_test.go, .planning/phases/01-foundation/01-08-SUMMARY.md
**Artifacts modified (01-08):** web/webdav/handlers.go (GET|HEAD case body), .planning/STATE.md, .planning/ROADMAP.md, .planning/REQUIREMENTS.md
**Work done (01-09):** Wave 5 — final Phase 1 gate. 3 atomic commits. (1) `730276ee3` test: `web/webdav/gowebdav_integration_test.go` (255 lines) with `TestE2E_GowebdavClient` and 5 explicitly-named subtests — one per ROADMAP success criterion. `SuccessCriterion1_BrowseWithBearerToken` uses `studio-b12/gowebdav.NewClient(env.TS.URL+"/dav/files", "", env.Token)` + `Connect` + `ReadDir("/")` + `Stat("/hello.txt")` + `Read`. Criteria 2-5 use raw `httpexpect` with `DontFollowRedirects` for precise HTTP-level assertions: 401+`WWW-Authenticate: Basic realm="Cozy"`; OPTIONS bypass with `DAV: 1` + `Allow`; `Depth: infinity` → 403 `<D:propfind-finite-depth/>`; `/dav/files/..%2fsettings` → 403; GET with `Range: bytes=0-4` → 206 + `Content-Range: bytes 0-4/14`; GET collection → 405 + `Allow: OPTIONS, PROPFIND, HEAD`; HEAD file → 200 + Content-Length + ETag, no body; `/remote.php/webdav/` → 308 + `Location: /dav/files/` + follow-through succeeds preserving method. gowebdav promoted from indirect to direct dep in go.mod (first non-test file importing it). All 5 subtests green on first run — every earlier wave had already delivered the pieces, this test proves they compose end-to-end. (2) `a0eb425d4` docs: finalized `01-VALIDATION.md` per-task verification map (01-01..01-09 all ✅ green) with real automated commands. Full `go test ./web/webdav/... -count=1` green in ~7s; `go build ./...` clean; `go vet ./web/webdav/...` clean. `-race` sweep FAILED with ~6 `WARNING: DATA RACE` reports — all between `pkg/config/config.UseViper` (write in test N setup) and `config.FsURL` (read in test N-1's leaked `AntivirusTrigger` goroutine). Race reproduced on `master` without any WebDAV code and after temporarily removing `gowebdav_integration_test.go` — entirely outside `web/webdav`. Initial commit kept `nyquist_compliant: false` and filed a CHECKPOINT decision ("ship with caveat" vs "fix race first"). (3) checkpoint resolution: user selected "Ship Phase 1, defer race fix". Patched `01-VALIDATION.md` frontmatter to `nyquist_compliant: true` + `nyquist_caveat` prose + `approval: approved`, added explicit disposition note to Gap 1, filed the race as FOLLOWUP-01 in STATE.md "Deferred Follow-ups" with full root-cause analysis, 5 non-webdav files involved, and 3 ranked fix options (smallest-blast-radius first: `t.Cleanup` + `stack.Shutdown` in `testutils.TestSetup`). Provisional slot: `01.1-race-harness` — user will decide at Phase 1 → Phase 2 transition whether it becomes a decimal phase or a Phase 2 Task 0 prerequisite. Two requirements completed: TEST-04 (integration auth with real WebDAV client — verified by SuccessCriterion1+2), SEC-05 (Content-Length sanity on full surface — verified implicitly by every PROPFIND/GET subtest + explicitly by existing plan 04 error-path tests). One deviation logged: Rule 4 — deferred race fix per user decision at checkpoint.
**Artifacts created (01-09):** web/webdav/gowebdav_integration_test.go, .planning/phases/01-foundation/01-09-SUMMARY.md
**Artifacts modified (01-09):** go.mod (gowebdav indirect→direct), .planning/phases/01-foundation/01-VALIDATION.md, .planning/STATE.md, .planning/ROADMAP.md, .planning/REQUIREMENTS.md
**Next action:** Phase 1 complete. Hand off to `/gsd:verify-work` for regression_gate + gsd-verifier sign-off. Before starting Phase 2, user must decide disposition of FOLLOWUP-01 (harness race): new decimal phase `01.1-race-harness`, or Phase 2 Task 0 prerequisite.

### Open Todos

- [x] ~~Confirm `vfs.DirIterator` / `DirBatch` method signatures from `model/vfs/couchdb_indexer.go` before Phase 1 PROPFIND implementation~~ — confirmed during plan 01-07 (DirIterator interface at model/vfs/vfs.go:332-337, IteratorOptions{ByFetch: 200} used in production)
- [ ] Decide GET on collection behavior (READ-10) during Phase 1 planning
- [ ] Re-introduce `TestAuth_401IsNotLogged` once `pkg/logger` exposes a test-capture seam or `inst.Logger()` becomes injectable (SEC-04 verification)
- [ ] Document or Makefile-ise `COZY_COUCHDB_URL=http://admin:password@localhost:5984/` for local dev (already in docs/CONTRIBUTING.md)
- [x] ~~Flip `newWebdavTestEnv` `overrideRoutes` from required to optional now that plan 01-06 provides `Routes`~~ — done in `a53950f65`

### Blockers

None for Phase 1 (shipped). See "Deferred Follow-ups" → FOLLOWUP-01 for the harness-race deferral that must be resolved before Phase 2 (disposition decision at phase transition).

---

*Last updated: 2026-04-05 after executing Plan 01-09 (end-to-end gowebdav integration test + Phase 1 sign-off; harness race deferred to FOLLOWUP-01)*

---
phase: 01-foundation
plan: 08
subsystem: webdav-read
tags: [webdav, get, head, range, rfc4918, read-10, green]

requires:
  - phase: 01-foundation
    plan: 03
    provides: davPathToVFSPath + ErrPathTraversal (path → VFS mapping)
  - phase: 01-foundation
    plan: 04
    provides: sendWebDAVError (404/405/403 error XML bodies)
  - phase: 01-foundation
    plan: 06
    provides: handlePath dispatcher (GET/HEAD case replaced in this plan)
provides:
  - web/webdav/get.go — handleGet (GET/HEAD via vfs.ServeFileContent)
  - web/webdav/get_test.go — 6 integration tests (file, HEAD, Range, collection 405, 404, 401)
  - web/webdav/handlers.go — dispatcher GET|HEAD case now calls handleGet
affects:
  - 01-09 (end-to-end integration test — GET is now a green path through the router)
  - Phase 2/3 (write verbs can reuse the same ServeFileContent delegation pattern where reads are needed post-write)

tech-stack:
  added: []
  patterns:
    - "Delegate to vfs.ServeFileContent rather than re-implementing Range/ETag/Content-Length. ServeFileContent wraps http.ServeContent, which handles HEAD method (headers only), Range header (206 + Content-Range), If-Modified-Since, If-None-Match, and the Content-Length/Last-Modified headers from doc.UpdatedAt + doc.ByteSize. Rolling our own would duplicate ~200 lines of stdlib logic that the VFS already exercises."
    - "GET/HEAD share a single case in handlePath. http.ServeContent branches internally on r.Method so a single handler serves both verbs with no extra code. The plan spec collapses them for exactly this reason."
    - "Collection GET returns 405 Method Not Allowed with Allow: OPTIONS, PROPFIND, HEAD — NOT an HTML directory listing. READ-10 decision from CONTEXT.md §WebDAV scope: Phase 1 is read-only and WebDAV clients never render HTML, so a nav page would be dead weight for OnlyOffice/Finder/Cyberduck."

key-files:
  created:
    - web/webdav/get.go
    - web/webdav/get_test.go
    - .planning/phases/01-foundation/01-08-SUMMARY.md
  modified:
    - web/webdav/handlers.go

key-decisions:
  - "Use middlewares.AllowVFS(c, permission.GET, fileDoc) for per-file scope enforcement, not a blanket package-level permission check. The OAuth token's permission set may legitimately narrow to a subset of the VFS (a share link, a scoped app token), and AllowVFS is the canonical cozy-stack primitive for that. Out-of-scope attempts emit an audit WARN (auditLog event 'get out-of-scope') carrying the normalized VFS path — intrusion-detection signal that some other token just tried to read a file it shouldn't know about."
  - "Return 405 (not 403) on GET of a collection. Two reasonable answers exist — 405 Method Not Allowed (method isn't valid on THIS resource type) and 403 Forbidden (access denied for this resource). RFC 7231 §6.5.5 ties 405 explicitly to 'the method … is not supported by the target resource' which is exactly our case. 405 also requires an Allow header, which is how clients discover which methods DO work on the collection — no such requirement on 403. Finder and gowebdav both handle 405-with-Allow correctly; 403 would leave them blind."
  - "Return sentinel 'forbidden' for traversal AND permission-scope failures. Both produce the same HTTP status (403) and the same XML error body, but they log different audit events ('get path rejected' vs 'get out-of-scope'). Collapsing the client-visible shape is deliberate — the attacker gets the same response whether their path was malformed or their token was under-scoped, so probing for one reveals nothing about the other."
  - "Pass nil as the version argument to ServeFileContent. Phase 1 is fully current-version read-only; WebDAV has no native concept of file versions and no client can ask for a historical revision through the WebDAV protocol. If Phase 4+ ever adds version-aware reads (e.g. via a Cozy-specific X-Cozy-Version header) this is the single line that changes."
  - "Skipped the REFACTOR pass. handleGet is 35 LoC including comments, has exactly one decision branch per error class, and gofmt produces empty output. The plan explicitly allows skipping when the handler is already compact — the if/err branches could only be collapsed by sacrificing readability, and there are no duplicate fragments to extract."

requirements-completed: [READ-08, READ-09, READ-10]

metrics:
  tasks_total: 2
  tasks_completed: 2
  duration: ~1.5min
  started: 2026-04-05T15:10:50Z
  completed: 2026-04-05T15:12:20Z
---

# Phase 01 Plan 08: WebDAV GET + HEAD Handlers Summary

**Implemented handleGet — GET/HEAD on files delegate to vfs.ServeFileContent (which handles Range, ETag, Content-Length, HEAD via http.ServeContent for free); GET on collections returns 405 Method Not Allowed with an Allow header advertising OPTIONS/PROPFIND/HEAD; traversal and out-of-scope accesses produce 403 + audit WARN; missing paths return 404. 6 integration tests cover file read, HEAD headers-only, byte Range (206), collection 405, 404, and unauthenticated 401.**

## Performance

- **Duration:** ~1.5 min
- **Started:** 2026-04-05T15:10:50Z
- **Completed:** 2026-04-05T15:12:20Z
- **Tasks:** 2 (RED + GREEN — REFACTOR skipped, handleGet already compact)
- **Files:** 2 created, 1 modified

## Accomplishments

- `handleGet` serves files via `vfs.ServeFileContent(inst.VFS(), fileDoc, nil, "", "", c.Request(), c.Response())` — one line for the entire happy path; http.ServeContent underneath handles Range, ETag, Last-Modified, Content-Length, and HEAD semantics.
- Collection GET/HEAD returns 405 with `Allow: OPTIONS, PROPFIND, HEAD`.
- Path traversal (via `davPathToVFSPath` / `ErrPathTraversal`) returns 403 and emits `auditLog(c, "get path rejected", rawParam)`.
- Per-file permission scope enforced via `middlewares.AllowVFS(c, permission.GET, fileDoc)`; failures return 403 and emit `auditLog(c, "get out-of-scope", vfsPath)`.
- 404 for `os.ErrNotExist`; upstream VFS errors propagate to the stack HTTPErrorHandler.
- `handlePath` GET|HEAD case in `web/webdav/handlers.go` now dispatches to `handleGet` (one-line case body replacement, no restructuring — the dispatcher plumbing from plan 01-06 stays stable).
- 6 integration tests, all green.

## Task Commits

1. **Task 1 — RED tests** — `975404c79` (test)
   - Created `web/webdav/get_test.go` with `TestGet_File_ReturnsContent`, `TestHead_File_NoBody`, `TestGet_File_RangeRequest`, `TestGet_Collection_Returns405`, `TestGet_Nonexistent_Returns404`, `TestGet_Unauthenticated_Returns401`.
   - Local `seedFile(t, inst, name, content)` helper wires `vfs.NewFileDoc` + `fs.CreateFile` + `io.Copy`. Pattern lifted from `model/vfs/vfs_test.go:76-96`.
   - Verified RED: all file/HEAD/Range/405/404 tests failed with `501 Not Implemented` from the plan 01-06 stub; the 401 test passed (auth runs before the dispatcher so it was already correct).
2. **Task 2 — GREEN implementation** — `accd13500` (feat)
   - Created `web/webdav/get.go` with the full `handleGet` implementation (35 LoC).
   - Edited `web/webdav/handlers.go` to replace the 501-stub `case http.MethodGet, http.MethodHead:` body with `return handleGet(c)` — targeted Edit, no dispatcher restructuring.
   - All 6 tests green in 1.56s; full `web/webdav` package regression green in 3.91s; `gofmt -l`, `go vet ./web/webdav/`, and `go build ./...` all clean.

## Files Created/Modified

- **Created** `web/webdav/get.go` — 58 lines including doc comment. Imports: `errors`, `net/http`, `os`, `model/permission`, `model/vfs`, `web/middlewares`, `echo/v4`.
- **Created** `web/webdav/get_test.go` — 115 lines, 6 test functions + local `seedFile` helper. Internal `package webdav` so tests can reference unexported helpers in future (not strictly needed here but matches the package convention).
- **Modified** `web/webdav/handlers.go` — 3-line diff: replaced the 501 `sendWebDAVError` stub body with `return handleGet(c)`, dropped the adjacent "Implemented in plan 01-08" comment (now lives in `get.go`'s package doc).

## Final API (`web/webdav/get.go`)

```go
// Unexported — bound by Routes via handlePath dispatcher
func handleGet(c echo.Context) error
```

### Contract

| Scenario                          | Status | Headers / body                                           |
| --------------------------------- | ------ | -------------------------------------------------------- |
| GET file                          | 200    | Body, Content-Length (exact), Etag, Last-Modified        |
| GET file with Range               | 206    | Content-Range, Content-Length (partial)                  |
| HEAD file                         | 200    | Same headers as GET, empty body                          |
| GET collection                    | 405    | Allow: OPTIONS, PROPFIND, HEAD + sendWebDAVError body    |
| GET missing path                  | 404    | sendWebDAVError "not-found" XML body                     |
| GET traversal                     | 403    | sendWebDAVError "forbidden" + audit WARN                 |
| GET out-of-scope                  | 403    | sendWebDAVError "forbidden" + audit WARN                 |
| Unauthenticated                   | 401    | WWW-Authenticate: Basic realm="Cozy" (from auth mw)      |

## Verification

```
$ COZY_COUCHDB_URL=http://admin:password@localhost:5984/ \
  go test ./web/webdav/ -run 'TestGet|TestHead' -count=1
ok  	github.com/cozy/cozy-stack/web/webdav	1.557s

$ COZY_COUCHDB_URL=http://admin:password@localhost:5984/ \
  go test ./web/webdav/ -count=1
ok  	github.com/cozy/cozy-stack/web/webdav	3.911s

$ gofmt -l web/webdav/get.go web/webdav/get_test.go web/webdav/handlers.go
(empty)

$ go vet ./web/webdav/
(empty)

$ go build ./...
(no output)
```

### Acceptance criteria (Task 1 — RED)

- [x] `web/webdav/get_test.go` exists with 6 test functions — verified
- [x] `grep -q 'TestGet_File_ReturnsContent\|TestGet_Collection_Returns405\|TestGet_File_RangeRequest' web/webdav/get_test.go` — verified
- [x] Tests currently fail (501 stub) — verified before commit
- [x] Commit `975404c79` matches `test(.*): add RED tests for GET` — verified

### Acceptance criteria (Task 2 — GREEN)

- [x] `go test ./web/webdav/ -run 'TestGet|TestHead' -count=1` exits 0 — verified
- [x] `go test ./web/webdav/ -count=1` exits 0 (no regressions) — verified
- [x] `grep -q 'ServeFileContent' web/webdav/get.go` — verified
- [x] `grep -q 'StatusMethodNotAllowed' web/webdav/get.go` — verified
- [x] `grep -q 'OPTIONS, PROPFIND, HEAD' web/webdav/get.go` — verified
- [x] Commit `accd13500` matches `feat(.*): GET.*GREEN` — verified

## Decisions Made

See `key-decisions` in frontmatter. The load-bearing one is the **delegate-everything-to-ServeFileContent** choice: http.ServeContent already implements HEAD (headers only, no body), Range (206 + Content-Range + partial body), If-Modified-Since, If-None-Match, and computes Content-Length from the seek length of the underlying reader. Re-implementing those four features in handleGet would (a) duplicate stdlib code, (b) silently diverge from the rest of cozy-stack's file-serving endpoints (`web/files/files.go` also delegates to ServeFileContent for the Files JSON:API downloads), and (c) create a maintenance burden for every future Go release that tightens http.ServeContent semantics. The one-line delegation keeps the handler aligned with the rest of the stack.

## Deviations from Plan

None — plan executed exactly as written.

The Task 2 action block in the plan contained the complete `handleGet` body verbatim, and it compiled + turned all 6 RED tests green on first run. No auto-fixes (Rules 1-3 did not trigger), no architectural decisions (Rule 4 did not trigger), no authentication gates.

## Issues Encountered

None. The plan's context note flagged that plan 01-07 runs in parallel on `handlers.go`, but at the point this plan executed plan 01-07 had not yet landed its edit — `git log --oneline -3` showed `680f22721` (01-06 docs commit) as the most recent predecessor, so no merge conflict to handle. The plan's own instruction was followed regardless: the edit used a targeted `Edit` replacement of the GET/HEAD case only, never rewriting the whole dispatcher.

## User Setup Required

None beyond the existing `COZY_COUCHDB_URL=http://admin:password@localhost:5984/` env var already documented in plan 01-05 and cleared in plan 01-06.

## Handoff to Downstream Plans

**Plan 01-09 (end-to-end integration test):**
- GET is now a fully green path through `newWebdavTestEnv(t, nil)` + the real router. A gowebdav-client-against-real-routes test can drive file reads, HEAD probes, and Range requests against a seeded tree with no extra scaffolding.
- The `seedFile` helper in `get_test.go` is local to that file; plan 01-09 can either re-export it to `testutil_test.go` or roll its own inline — Phase 1 does not need multi-file seeding yet.

**Plan 01-07 (PROPFIND handler — running in parallel):**
- Shares the same `handlers.go` dispatcher. If plan 01-07 lands after 01-08, its edit is a one-line replacement of the `case "PROPFIND":` branch — no conflict with this plan's GET|HEAD edit. If it lands before, a rebase is a trivial merge of two non-overlapping switch-case edits.

**Phase 2/3 (write verbs):**
- `handleGet`'s delegation pattern (path-validate → VFS lookup → permission check → delegate to model/vfs) is the template for MKCOL, PUT, COPY, MOVE, DELETE handlers. The path-validate + permission-check preamble is identical; only the final delegation call changes per verb.
- When PUT lands, a file created via PUT will be immediately readable via this handleGet against the same VFS instance — no special bridge required.

**Phase 4+ (versions, if ever added):**
- The `nil` version argument in `ServeFileContent` is the only line that changes if WebDAV gains a header-based version selector. `handleGet` stays otherwise untouched.

## Next Phase Readiness

- Plan 01-08 complete. Wave 4 GET/HEAD path ready.
- With plan 01-07 PROPFIND still open, the Phase 1 read-only surface is 2-of-3 plans done (route plumbing + error bodies + path mapper + auth + OPTIONS + GET/HEAD; PROPFIND and the end-to-end integration test remain).
- No blockers introduced.
- No new STATE.md todos.

---
*Phase: 01-foundation*
*Completed: 2026-04-05*

## Self-Check: PASSED

- `web/webdav/get.go` present and contains `ServeFileContent`, `StatusMethodNotAllowed`, `OPTIONS, PROPFIND, HEAD` — verified (6 matching lines).
- `web/webdav/get_test.go` present with 6 `TestGet_*` / `TestHead_*` functions — verified.
- `.planning/phases/01-foundation/01-08-SUMMARY.md` present — verified.
- Commit `975404c79` (`test(01-08): add RED tests for GET/HEAD/Range/collection-405`) present in `git log` — verified.
- Commit `accd13500` (`feat(01-08): GET/HEAD via vfs.ServeFileContent, collection to 405 — GREEN`) present in `git log` — verified.
- `go test ./web/webdav/ -run 'TestGet|TestHead' -count=1` exits 0 (6/6 pass) — verified.
- `go test ./web/webdav/ -count=1` full package exits 0 (no regression) — verified.
- `gofmt -l web/webdav/get.go web/webdav/get_test.go web/webdav/handlers.go` empty — verified.
- `go vet ./web/webdav/` clean — verified.
- `go build ./...` clean — verified.

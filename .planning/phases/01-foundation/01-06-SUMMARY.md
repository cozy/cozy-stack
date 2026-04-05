---
phase: 01-foundation
plan: 06
subsystem: webdav-routing
tags: [webdav, echo, routing, rfc4918, options, nextcloud, redirect, 308, route-01, route-02, route-04, route-05]

requires:
  - phase: 01-foundation
    plan: 03
    provides: davPathToVFSPath + ErrPathTraversal (dispatcher will consume in plan 07/08)
  - phase: 01-foundation
    plan: 04
    provides: sendWebDAVError (used by the handlePath 501 stub)
  - phase: 01-foundation
    plan: 05
    provides: resolveWebDAVAuth + newWebdavTestEnv + OPTIONS bypass contract
provides:
  - web/webdav.Routes(*echo.Group) — canonical WebDAV route registrar
  - web/webdav.NextcloudRedirect — exported 308 handler for /remote.php/webdav/*
  - web/webdav.handleOptions — RFC 4918 §10.1 discovery handler (DAV: 1, Allow, MS-Author-Via)
  - web/webdav.handlePath — 501-stub dispatcher to be filled by plans 01-07/08
  - /dav/files + /dav/files/* registered in web/routing.go SetupRoutes
  - /remote.php/webdav + /remote.php/webdav/* → 308 redirect with method preservation
  - newWebdavTestEnv(t, nil) now defaults to the real Routes (flipped from required-override)
affects:
  - 01-07 (PROPFIND handler replaces the PROPFIND branch of handlePath; uses newWebdavTestEnv(t, nil))
  - 01-08 (GET/HEAD handler replaces the GET/HEAD branch of handlePath)
  - 01-09 (end-to-end integration test can now drive a real router via newWebdavTestEnv(t, nil))
  - Phase 2/3 (MKCOL/PUT/MOVE/COPY/DELETE replace the default branch; route table already wired)

tech-stack:
  added: []
  patterns:
    - "OPTIONS is registered on the outer group (no auth); every other WebDAV method lives under a router.Group(\"\", resolveWebDAVAuth) subgroup. Single place to enforce the bypass, no conditional logic inside the middleware."
    - "handlePath is a method-switch dispatcher — each Phase 1/2/3 plan replaces one case. The plumbing stays stable, the cases grow. This avoids re-touching Routes (and therefore the route table) each time a new verb ships."
    - "NextcloudRedirect is registered at the Echo root (not inside the /dav group) because the source path /remote.php/webdav does not share the /dav prefix. Same middleware chain, different mount point."
    - "Integration tests hit /remote.php/webdav by registering the redirect routes directly on env.TS.Config.Handler.(*echo.Echo). This dodges the need to expose a second setup.GetTestServer call or mutate newWebdavTestEnv."
    - "httpexpect.DontFollowRedirects is the canonical way to observe 3xx responses in this repo (see web/auth/auth_test.go) — reused verbatim."

key-files:
  created:
    - web/webdav/options_test.go
    - .planning/phases/01-foundation/01-06-SUMMARY.md
  modified:
    - web/webdav/webdav.go
    - web/webdav/handlers.go
    - web/webdav/testutil_test.go
    - web/routing.go

key-decisions:
  - "NextcloudRedirect is exported from day one, not renamed from lowercase in Task 3. The plan text hinted at a two-step (lowercase in Task 1/2, then capitalise in Task 3 for routing.go). Two-step is pointless churn: tests in package webdav can reference either form, so starting exported eliminates a rename commit and a second test edit. Applied as Rule 3 (blocking triviality)."
  - "handlePath collapses every unimplemented verb to the same sendWebDAVError(501, 'not-implemented') call. Three switch cases (PROPFIND / GET|HEAD / default) are kept distinct even though they return the same response today, so the diff for plans 07 and 08 is a single case body swap with no restructuring."
  - "'not-implemented' is NOT a vocabulary token in RFC 4918's precondition/postcondition list. This is deliberate — the error XML body on a 501 is advisory for clients that do parse it, and 'not-implemented' maps 1:1 to the HTTP status name. Phase 1 clients (PROPFIND + GET/HEAD) will never actually hit this path since plans 07/08 replace those branches before end-to-end tests run. Phase 2/3 verbs land together, so the temporary 501 is surface-visible only during development."
  - "OPTIONS returns MS-Author-Via: DAV in addition to DAV: 1 + Allow. The Windows Mini-Redirector refuses to upgrade a connection to WebDAV without this header, per research §236-272. Harmless for every other client."
  - "handleOptions responds with c.NoContent(200) — no body. RFC 4918 §10.1 is silent on the body and every observed client ignores it. Omitting a body skips a Content-Length computation and keeps the handler allocation-free."
  - "Nextcloud redirect uses strings.Replace with n=1 (single occurrence). Matching only the first instance protects against bizarre paths like /remote.php/webdav/foo/remote.php/webdav — the inner literal would be a filename and must NOT be rewritten."
  - "newWebdavTestEnv's overrideRoutes parameter is now optional (nil defaults to Routes). The required-nil guard from plan 01-05 was a stop-gap for the pre-Routes era; flipping it now unblocks plans 07/08/09 and Phase 2/3 from having to write their own mount callbacks when they want the full router. Explicit registrars (mountAuthOnly, mountRealRoutes) still work for tests that mount extra routes or exercise middleware in isolation."
  - "The WebDAV route block in web/routing.go uses a deliberately narrower middleware chain than the JSON:API block (NeedInstance + CheckInstanceBlocked + CheckInstanceDeleting only — no LoadSession, no Accept, no TOS deadline). Rationale: WebDAV is token-only (no cookies), its XML content-type is non-negotiable, and TOS-deadline HTML cannot be rendered by a WebDAV client. Keeping the chain narrow matches what the `webdav` package internally expects (resolveWebDAVAuth handles its own token + 401)."

requirements-completed: [ROUTE-01, ROUTE-02, ROUTE-04, ROUTE-05]

metrics:
  tasks_total: 3
  tasks_completed: 3
  duration: ~4min
  started: 2026-04-05T14:55:46Z
  completed: 2026-04-05T14:59:51Z
---

# Phase 01 Plan 06: WebDAV Route Wiring + OPTIONS + handlePath Dispatcher Summary

**Shipped `webdav.Routes`, the `handleOptions` RFC 4918 §10.1 discovery handler, the `handlePath` 501-stub dispatcher that plans 07/08 will fill in, the `NextcloudRedirect` 308 handler, and registered all of them in `web/routing.go` under a narrow WebDAV-specific middleware chain — unlocking end-to-end integration tests against a real router for every downstream Phase 1/2/3 plan.**

## Performance

- **Duration:** ~4 min
- **Started:** 2026-04-05T14:55:46Z
- **Completed:** 2026-04-05T14:59:51Z
- **Tasks:** 3 + 1 helper refactor
- **Files:** 1 created, 4 modified

## Accomplishments

- `web/webdav.Routes(g *echo.Group)` is the single entry point callers use to wire WebDAV into any Echo router. It mounts OPTIONS without auth and every other WebDAV method under `resolveWebDAVAuth`.
- `handleOptions` answers discovery probes with `DAV: 1`, `Allow: OPTIONS, PROPFIND, GET, HEAD`, and `MS-Author-Via: DAV`. Zero VFS access, zero allocation, zero body bytes. Unauthenticated by design.
- `handlePath` is a 4-line method switch that returns `sendWebDAVError(501, "not-implemented")` for every verb — plans 01-07 (PROPFIND) and 01-08 (GET/HEAD) replace one switch case each, and Phase 2/3 verbs replace the default.
- `NextcloudRedirect` performs a 308 Permanent Redirect from `/remote.php/webdav/*` to `/dav/files/*`. 308 preserves the request method (critical for PROPFIND — 301/302 would silently downgrade to GET and break every non-GET Nextcloud client).
- Six integration tests (3× OPTIONS variations, 3× Nextcloud redirect variations including PROPFIND method preservation) pass in 1.44s against a real cozy test instance.
- `web/routing.go` SetupRoutes now mounts WebDAV at `/dav` with a narrower middleware chain (`NeedInstance` + `CheckInstanceBlocked` + `CheckInstanceDeleting`) — no LoadSession, no Accept negotiation, no TOS deadline check. Full cozy-stack build clean, go vet clean.
- `newWebdavTestEnv(t, nil)` now defaults to the real `Routes` — the required-override guard from plan 01-05 was a pre-Routes stop-gap; flipping it unblocks plans 01-07/08/09 and Phase 2/3 from having to hand-roll their own mount callbacks when they want the full router.

## Task Commits

1. **Task 1 — RED tests for OPTIONS + Nextcloud 308 redirect** — `f3b07465a` (test)
   - 6 new `TestOptions_*` / `TestNextcloudRedirect_*` functions
   - `mountRealRoutes` helper (passthrough to `Routes`) + `registerNextcloudRedirect` helper (mounts the redirect on the underlying `*echo.Echo` served by httptest)
   - Compile-fails on `undefined: Routes` and `undefined: NextcloudRedirect`
2. **Task 2 — GREEN: Routes + handleOptions + handlePath + NextcloudRedirect** — `b71d86087` (feat)
   - `webdav.go` rewritten: `Routes`, `davAllowHeader`, `NextcloudRedirect`
   - `handlers.go` rewritten: `handleOptions`, `handlePath` dispatcher
   - All 6 RED tests flip GREEN; full `web/webdav` package suite still exits 0 (~2.5s)
3. **Task 3 — Wire WebDAV routes in web/routing.go SetupRoutes** — `7c023c2ed` (feat)
   - New import `web/webdav`
   - New route block between the JSON:API block and the non-auth block, with its own `mwsWebDAV` chain
   - `webdav.Routes(router.Group("/dav", mwsWebDAV...))`
   - `router.Match(webdavRedirectMethods, "/remote.php/webdav[/*]", webdav.NextcloudRedirect, mwsWebDAV...)` on both root and wildcard paths
   - `go build ./...` clean; `go vet ./web/...` clean; web/webdav package still green
4. **Helper refactor — flip newWebdavTestEnv overrideRoutes to optional** — `a53950f65` (refactor)
   - `testutil_test.go`: nil `overrideRoutes` now defaults to `Routes`. Clears the matching STATE.md todo filed in plan 01-05.

## Files Created/Modified

- **Created** `web/webdav/options_test.go` — 117 lines, 6 integration tests + 2 helpers (`mountRealRoutes`, `registerNextcloudRedirect`)
- **Modified** `web/webdav/webdav.go` — was a 20-line package doc + method list stub; now 62 lines adding `davAllowHeader`, `Routes`, `NextcloudRedirect`
- **Modified** `web/webdav/handlers.go` — was a 1-line comment stub; now 41 lines with `handleOptions` + `handlePath`
- **Modified** `web/webdav/testutil_test.go` — 6 lines changed to make `overrideRoutes` default to `Routes` when nil
- **Modified** `web/routing.go` — 1 import + 26-line block adding the WebDAV group and the Nextcloud redirect

## Final API

**`web/webdav/webdav.go`**
```go
var webdavMethods = []string{OPTIONS, PROPFIND, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE}
const davAllowHeader = "OPTIONS, PROPFIND, GET, HEAD"

func Routes(router *echo.Group)                  // exported — called from web/routing.go
func NextcloudRedirect(c echo.Context) error     // exported — 308 handler
```

**`web/webdav/handlers.go`**
```go
func handleOptions(c echo.Context) error  // package-internal, bound by Routes
func handlePath(c echo.Context) error     // package-internal, bound by Routes
```

### Routes behaviour
1. `router.OPTIONS("/files", handleOptions)` + `router.OPTIONS("/files/*", handleOptions)` — bypasses `resolveWebDAVAuth`
2. `authed := router.Group("", resolveWebDAVAuth)` — subgroup enforcing auth on everything else
3. `authed.Match(webdavMethods[1:], "/files", handlePath)` + `authed.Match(webdavMethods[1:], "/files/*", handlePath)` — dispatches to the 501-stub

### handleOptions contract
- Sets headers: `DAV: 1`, `Allow: OPTIONS, PROPFIND, GET, HEAD`, `MS-Author-Via: DAV`
- Returns `200 No Content`
- Does NOT touch VFS, does NOT read request body, does NOT parse the wildcard param

### handlePath stubbed cases (plans 07/08 replace these)
| Method            | Current behaviour                                      | Replaced by |
| ----------------- | ------------------------------------------------------ | ----------- |
| `PROPFIND`        | `sendWebDAVError(501, "not-implemented")`              | Plan 01-07  |
| `GET` / `HEAD`    | `sendWebDAVError(501, "not-implemented")`              | Plan 01-08  |
| `PUT`/`DELETE`/`MKCOL`/`COPY`/`MOVE` | `sendWebDAVError(501, "not-implemented")` | Phase 2/3   |

### NextcloudRedirect contract
- Reads `c.Request().URL.Path`
- `strings.Replace(path, "/remote.php/webdav", "/dav/files", 1)` — single replacement only (protects against pathological filenames containing the literal)
- Appends `?RawQuery` if present
- Returns `c.Redirect(308, newPath)` — **NOT** 301 or 302

## Insertion Point in web/routing.go

```
SetupRoutes (line 175)
├── router.Use(timersMiddleware, secure, CORS)
├── Block: non-authentified HTML routes (/auth, /public, /.well-known)
├── Block: authentified JSON API routes (/files, /data, /contacts, …)
├── Block: WebDAV routes  ← NEW (lines 259-283)
│   ├── mwsWebDAV = [NeedInstance, CheckInstanceBlocked, CheckInstanceDeleting]
│   ├── webdav.Routes(router.Group("/dav", mwsWebDAV...))
│   ├── router.Match(all-verbs, "/remote.php/webdav", webdav.NextcloudRedirect, mwsWebDAV...)
│   └── router.Match(all-verbs, "/remote.php/webdav/*", webdav.NextcloudRedirect, mwsWebDAV...)
├── Block: other non-authentified routes (/connection_check, /status, /version)
└── setupRecover + HTTPErrorHandler
```

## Verification

```
$ COZY_COUCHDB_URL=http://admin:password@localhost:5984/ \
  go test ./web/webdav/ -run 'TestOptions|TestNextcloud' -count=1 -v
=== RUN   TestOptions_FilesRoot_NoAuth
--- PASS: TestOptions_FilesRoot_NoAuth (0.29s)
=== RUN   TestOptions_FilesSubpath_NoAuth
--- PASS: TestOptions_FilesSubpath_NoAuth (0.25s)
=== RUN   TestOptions_DoesNotCallVFS
--- PASS: TestOptions_DoesNotCallVFS (0.22s)
=== RUN   TestNextcloudRedirect_PreservesMethod
--- PASS: TestNextcloudRedirect_PreservesMethod (0.21s)
=== RUN   TestNextcloudRedirect_RootPath
--- PASS: TestNextcloudRedirect_RootPath (0.20s)
=== RUN   TestNextcloudRedirect_PropfindMethod
--- PASS: TestNextcloudRedirect_PropfindMethod (0.20s)
PASS
ok  	github.com/cozy/cozy-stack/web/webdav	1.440s

$ COZY_COUCHDB_URL=http://admin:password@localhost:5984/ \
  go test ./web/webdav/ -count=1
ok  	github.com/cozy/cozy-stack/web/webdav	2.533s

$ go build ./...
(no output)

$ go vet ./web/...
(no output)

$ gofmt -l web/webdav/ web/routing.go
(empty)
```

### Acceptance criteria

- [x] `options_test.go` exists with ≥5 `TestOptions_` / `TestNextcloud_` functions — 6 delivered
- [x] Task 1 compile-fails on `undefined: Routes` / `undefined: NextcloudRedirect` — verified before GREEN
- [x] `go test ./web/webdav/ -run 'TestOptions|TestNextcloud' -count=1` exits 0 — verified
- [x] `grep -q 'func Routes' web/webdav/webdav.go` — verified
- [x] `grep -q 'StatusPermanentRedirect' web/webdav/webdav.go` — verified
- [x] `grep -q 'DAV' web/webdav/handlers.go` (DAV: 1 header) — verified
- [x] `go build ./...` exits 0 — verified
- [x] `grep -q '"github.com/cozy/cozy-stack/web/webdav"' web/routing.go` — verified
- [x] `grep -q 'webdav.Routes' web/routing.go` — verified
- [x] `grep -q 'remote.php/webdav' web/routing.go` — 3 occurrences verified
- [x] All web/webdav tests still pass — verified
- [x] 3 atomic task commits + 1 refactor commit with required message patterns — `f3b07465a`, `b71d86087`, `7c023c2ed`, `a53950f65`

## Deviations from Plan

### 1. [Rule 3 — Blocking triviality] NextcloudRedirect exported from day one

- **Found during:** Task 1 implementation.
- **Issue:** The plan's Task 2 text used lowercase `nextcloudRedirect` as a package-internal symbol, then Task 3 explicitly renamed it to `NextcloudRedirect` for the `web/routing.go` call. This would have introduced a rename-only commit and an edit to the Task 1 test file.
- **Fix:** Exported `NextcloudRedirect` on first write. Tests in `package webdav` reference it directly — lowercase vs exported makes no difference to internal tests, but avoids a cross-package rename round trip.
- **Files modified:** `web/webdav/webdav.go`, `web/webdav/options_test.go`
- **Verification:** Both GREEN test run and `go build ./...` pass.
- **Committed in:** `b71d86087` (Task 2).

### 2. [Rule 3 — Blocking triviality] MS-Author-Via header added in handleOptions

- **Found during:** Task 2 implementation (cross-referenced against research §236-272 and §666-686).
- **Issue:** The plan text listed DAV and Allow headers in `handleOptions` but omitted `MS-Author-Via: DAV`, which research identifies as required for Windows Mini-Redirector compatibility. Forgetting it would cause every Windows client to silently refuse the connection.
- **Fix:** Added `h.Set("MS-Author-Via", "DAV")` in `handleOptions`. Harmless for non-Windows clients.
- **Files modified:** `web/webdav/handlers.go`
- **Verification:** OPTIONS tests still green; header is a passive addition.
- **Committed in:** `b71d86087` (Task 2).

### 3. [Follow-on — optional feature] Flipped newWebdavTestEnv overrideRoutes to optional

- **Found during:** Post-Task 3 (explicitly called out in the execute-phase context note and STATE.md open todos).
- **Issue:** Plan 01-05 made `overrideRoutes` required with a `t.Fatal` because `Routes` did not yet exist. Now that plan 01-06 defines it, keeping the guard would force every future test (01-07/08/09 + Phase 2/3) to write its own mount callback just to get the real router.
- **Fix:** Nil `overrideRoutes` defaults to `Routes`. Existing callers (`auth_test.go:mountAuthOnly`, `options_test.go:mountRealRoutes`) still work — they pass explicit registrars.
- **Files modified:** `web/webdav/testutil_test.go` (6 lines)
- **Verification:** Full `web/webdav` package green after the flip.
- **Committed in:** `a53950f65` (separate refactor commit).

---

**Total deviations:** 3 (2 blocking-trivialities baked into Task 2, 1 follow-on refactor clearing a STATE.md todo).
**Impact on plan:** None negative. All three are improvements agreed by the context note and/or research. No scope creep, no architectural change.

## Issues Encountered

- `httpexpect` API for not following redirects is `WithRedirectPolicy(httpexpect.DontFollowRedirects)`, not the integer `0` I initially wrote. Fixed by grepping `web/auth/auth_test.go` for the canonical usage — resolved in-file before committing the RED tests.

## User Setup Required

None beyond the existing `COZY_COUCHDB_URL=http://admin:password@localhost:5984/` env var documented in plan 01-05.

## Handoff to Downstream Plans

**Plan 01-07 (PROPFIND handler — GREEN):**
- Replace the `case "PROPFIND":` branch in `handlePath` with the real handler body.
- Use `newWebdavTestEnv(t, nil)` directly — the default Routes is wired and OPTIONS/redirect are already green.
- `c.Get("permission")` is guaranteed populated by `resolveWebDAVAuth` → `ForcePermission` on every non-OPTIONS request.

**Plan 01-08 (GET/HEAD handler — GREEN):**
- Replace the `case http.MethodGet, http.MethodHead:` branch.
- Same testing pattern: `newWebdavTestEnv(t, nil)` + token + `env.E.GET("/dav/files/...")`.

**Plan 01-09 (end-to-end integration test):**
- The real router is now reachable via `newWebdavTestEnv(t, nil)` so a full gowebdav-client-against-real-routes test is possible. OPTIONS + Nextcloud redirect are already proven.

**Phase 2/3 (write verbs):**
- Replace the `default` branch of `handlePath` with a nested switch or extract into per-verb dispatchers.
- Add each new verb's `Allow` entry to `davAllowHeader` in `webdav.go` when it lands.
- Routes wiring in `web/routing.go` needs no change — it already covers every method in `webdavMethods`.

## Next Phase Readiness

- Wave 3 plan 01-06 complete. Route plumbing ready for handler plans 01-07/08/09.
- No blockers introduced.
- One STATE.md todo cleared (`overrideRoutes` flip). Two still open from plan 01-05:
  - Re-introduce `TestAuth_401IsNotLogged` once logger gains a test-capture seam.
  - Document / Makefile-ise `COZY_COUCHDB_URL`.

---
*Phase: 01-foundation*
*Completed: 2026-04-05*

## Self-Check: PASSED

- `web/webdav/options_test.go` present with 6 `TestOptions_*` / `TestNextcloudRedirect_*` functions — verified.
- `web/webdav/webdav.go` present with `func Routes`, `func NextcloudRedirect`, `davAllowHeader`, `StatusPermanentRedirect` — verified.
- `web/webdav/handlers.go` present with `handleOptions` (DAV: 1 header) + `handlePath` dispatcher — verified.
- `web/webdav/testutil_test.go` `overrideRoutes` defaults to `Routes` when nil — verified.
- `web/routing.go` imports `web/webdav`, calls `webdav.Routes`, and Match'es `/remote.php/webdav[/*]` to `webdav.NextcloudRedirect` (3× `remote.php/webdav` grep hits) — verified.
- `.planning/phases/01-foundation/01-06-SUMMARY.md` present — verified.
- Commits `f3b07465a` (test), `b71d86087` (feat GREEN), `7c023c2ed` (feat routing), `a53950f65` (refactor testutil) all present in `git log` — verified.
- `go test ./web/webdav/ -run 'TestOptions|TestNextcloud' -count=1` exits 0 (6/6 pass) — verified with `COZY_COUCHDB_URL=http://admin:password@localhost:5984/`.
- `go test ./web/webdav/ -count=1` full package exits 0 — verified.
- `go build ./...` clean — verified.
- `go vet ./web/...` clean — verified.
- `gofmt -l web/webdav/ web/routing.go` empty — verified.

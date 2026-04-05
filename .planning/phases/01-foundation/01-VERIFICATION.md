---
phase: 01-foundation
verified: 2026-04-05T18:00:00Z
status: passed
score: 28/28 requirements verified, 5/5 ROADMAP success criteria verified
caveat: "pre-existing test-harness race (pkg/config/config + model/stack/model/job AntivirusTrigger) fails `go test -race` — OUT OF SCOPE for Phase 1, filed as FOLLOWUP-01 in STATE.md, approved by user"
human_verification_deferred:
  - test: "Real OnlyOffice mobile client against a live Cozy instance"
    expected: "Browse + read works end-to-end"
    why_human: "Requires physical device + TLS — deferred to Phase 3 per ROADMAP"
  - test: "iOS Files app against a live Cozy instance"
    expected: "Browse + read works"
    why_human: "Requires iOS device — deferred to Phase 3"
  - test: "macOS Finder read-only browse"
    expected: "Finder opens mount, lists files, downloads"
    why_human: "Requires Finder instance — deferred to Phase 3"
---

# Phase 1: Foundation — Verification Report

**Phase Goal:** Any WebDAV client can connect to a Cozy instance, authenticate, browse the directory tree, and download files. All security guards and correctness invariants (ETag, date format, XML namespace, path traversal prevention, Content-Length policy) are baked in before any write operation is attempted.

**Verified:** 2026-04-05
**Status:** passed (with documented `-race` caveat)
**Re-verification:** No — initial verification

---

## Methodology

Goal-backward verification driven by ROADMAP's 5 Success Criteria plus cross-reference against all 28 Phase-1 requirement IDs in REQUIREMENTS.md. Evidence is grounded in the actual source tree (`web/webdav/`, `web/routing.go`), not in plan/summary claims.

Test suite was re-executed during verification:

```
COZY_COUCHDB_URL=... go test ./web/webdav/... -count=1 -timeout 5m
ok  github.com/cozy/cozy-stack/web/webdav  6.504s
```

`go build ./web/webdav/...` is clean. No TODO/FIXME/PLACEHOLDER markers exist anywhere in the package.

---

## Success Criteria (ROADMAP)

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | gowebdav client with Bearer token can browse /dav/files/, PROPFIND Depth:0/1 return 207 with all 9 live properties, `xmlns:D="DAV:"` prefix | VERIFIED | `gowebdav_integration_test.go:42-81` `SuccessCriterion1_BrowseWithBearerToken` (green under -count=1). XML shape verified by `xml.go:32-117` (9 Prop fields, D: prefixed). Unit tests `xml_test.go` cover namespace, date format, ETag. |
| 2 | Unauthenticated non-OPTIONS → 401 `WWW-Authenticate: Basic realm="Cozy"`; OPTIONS succeeds without auth with DAV:1 + full Allow list | VERIFIED | `gowebdav_integration_test.go:88-107`. Auth middleware at `auth.go:22-48` (OPTIONS bypass + 401 path). `webdav.go:37-45` registers OPTIONS OUTSIDE auth group. `handlers.go:17-23` emits `DAV:1`, `Allow: OPTIONS, PROPFIND, GET, HEAD`, `MS-Author-Via: DAV`. |
| 3 | PROPFIND Depth:infinity → 403; `../`, `%2e%2e`, null bytes, system dirs rejected before VFS | VERIFIED | `gowebdav_integration_test.go:113-139`. `propfind.go:57-70` Depth:infinity handled BEFORE path mapping; path mapper at `path_mapper.go:35-56` rejects nulls, residual `%`, traversal, anchors under `/files` prefix. |
| 4 | GET streams with Content-Length, ETag, Last-Modified; Range works; GET on collection → 405 | VERIFIED | `gowebdav_integration_test.go:146-191`. `get.go:27-60` delegates to `vfs.ServeFileContent` (handles Range/HEAD/ETag via stdlib `http.ServeContent`); collection branch returns 405 with `Allow: OPTIONS, PROPFIND, HEAD`. |
| 5 | `/remote.php/webdav/*` → 308 to `/dav/files/*`; subsequent request succeeds | VERIFIED | `gowebdav_integration_test.go:197-254`. `webdav.go:54-61` `NextcloudRedirect` uses `StatusPermanentRedirect` (308, preserves method). Wired at `web/routing.go:275-282` for all WebDAV verbs. |

**Score: 5/5 success criteria VERIFIED.**

---

## Required Artifacts

| Artifact | Exists | Substantive | Wired | Status |
|----------|--------|-------------|-------|--------|
| `web/webdav/webdav.go` (Routes, NextcloudRedirect) | ✓ (61 LoC) | ✓ | ✓ (imported by `web/routing.go:52,270,280`) | VERIFIED |
| `web/webdav/auth.go` (resolveWebDAVAuth, 401, auditLog) | ✓ (80 LoC) | ✓ | ✓ (used in `webdav.go:41`) | VERIFIED |
| `web/webdav/handlers.go` (handleOptions, handlePath) | ✓ (40 LoC) | ✓ | ✓ | VERIFIED |
| `web/webdav/path_mapper.go` (davPathToVFSPath, ErrPathTraversal) | ✓ (66 LoC) | ✓ | ✓ (used in propfind.go:66, get.go:29) | VERIFIED |
| `web/webdav/xml.go` (Multistatus, Response, Prop, marshalMultistatus, buildETag, buildLastModified, buildCreationDate, parsePropFind) | ✓ (179 LoC) | ✓ | ✓ | VERIFIED |
| `web/webdav/errors.go` (buildErrorXML, sendWebDAVError) | ✓ (43 LoC) | ✓ | ✓ (used by every non-2xx handler) | VERIFIED |
| `web/webdav/propfind.go` (handlePropfind, streamChildren, buildResponseForDir/File) | ✓ (233 LoC) | ✓ | ✓ (dispatched from handlers.go:33) | VERIFIED |
| `web/webdav/get.go` (handleGet) | ✓ (60 LoC) | ✓ | ✓ (dispatched from handlers.go:35) | VERIFIED |
| `web/routing.go` wiring | ✓ | ✓ | ✓ (import + Routes + NextcloudRedirect registration) | VERIFIED |
| `web/webdav/xml_test.go`, `path_mapper_test.go`, `auth_test.go`, `errors_test.go`, `propfind_test.go`, `get_test.go`, `options_test.go`, `gowebdav_integration_test.go` | ✓ (8 test files, ~980 LoC) | ✓ | N/A | VERIFIED |

---

## Key Link Verification

| From | To | Via | Status | Evidence |
|------|----|----|--------|----------|
| `web/routing.go` | `webdav.Routes` | direct call on `/dav` group | WIRED | `web/routing.go:270` |
| `web/routing.go` | `webdav.NextcloudRedirect` | `router.Match` on `/remote.php/webdav[/*]` for all WebDAV verbs | WIRED | `web/routing.go:279-282` |
| `webdav.Routes` | `resolveWebDAVAuth` | echo sub-group wraps all non-OPTIONS methods | WIRED | `webdav.go:41-44` |
| `handlePath` | `handlePropfind` / `handleGet` | method switch | WIRED | `handlers.go:30-40` |
| `handlePropfind` | `vfs.DirOrFileByPath` + `DirIterator` (ByFetch=200) | `inst.VFS()` | WIRED | `propfind.go:74, 125` |
| `handleGet` | `vfs.ServeFileContent` | direct call | WIRED | `get.go:59` |
| `handlePropfind` / `handleGet` | `middlewares.AllowVFS(permission.GET, fetcher)` | permission scope check | WIRED | `propfind.go:89, get.go:52` |
| `resolveWebDAVAuth` | `middlewares.GetRequestToken` + `ParseJWT` + `ForcePermission` | standard Cozy primitives | WIRED | `auth.go:28-38` |
| All non-2xx responses | `sendWebDAVError` (Content-Length set before WriteHeader) | canonical helper | WIRED | propfind.go:59/61/69/77/91, get.go:32/39/49/54, errors.go:35-42 |
| Audit logs on traversal / out-of-scope / Depth:infinity | `auditLog` (WARN, inst.Logger().WithNamespace("webdav")) | structured fields | WIRED | 5 call sites in `propfind.go`, `get.go`; never called from 401 path |

All 10 key links VERIFIED.

---

## Requirements Coverage (28 Phase-1 IDs)

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| **ROUTE-01** | 01-06 | `/dav/files/` exposed | SATISFIED | `webdav.go:38-44`; wired in `web/routing.go:270` |
| **ROUTE-02** | 01-06 | `/remote.php/webdav/*` → 308 to `/dav/files/*` | SATISFIED | `webdav.go:54-61` (StatusPermanentRedirect); `web/routing.go:279-282`; E2E subtest 5 |
| **ROUTE-03** | 01-03 | Path normalisation, URL decoding, `path.Clean`, prefix assertion | SATISFIED | `path_mapper.go:35-56`; `path_mapper_test.go` covers normalisation, decoding, trailing slash |
| **ROUTE-04** | 01-06 | OPTIONS returns `DAV:1` + `Allow:` + no auth | SATISFIED | `handlers.go:17-23`; OPTIONS registered outside auth group (`webdav.go:38-39`); E2E subtest 2 |
| **ROUTE-05** | 01-03 | Only `/files/` exposed (never `/settings`, `/apps`, etc.) | SATISFIED | `path_mapper.go:45-49` anchors cleaned path under `/files` prefix; rejects anything else. `..%252fsettings` rejected in E2E subtest 3 |
| **AUTH-01** | 01-05 | OAuth Bearer via `middlewares.GetRequestToken` | SATISFIED | `auth.go:28` |
| **AUTH-02** | 01-05 | Token also accepted in Basic password field (username ignored) | SATISFIED | `middlewares.GetRequestToken` handles both conventions; E2E subtest 1 uses `gowebdav.NewClient(..., "", token)` (empty username, token as password) — works end-to-end |
| **AUTH-03** | 01-05 | 401 `WWW-Authenticate: Basic realm="Cozy"` on non-OPTIONS | SATISFIED | `auth.go:45-48`; E2E subtest 2 |
| **AUTH-04** | 01-05 | Token → existing Cozy permissions (no bypass) | SATISFIED | `auth.go:33-38` uses `ParseJWT` + `ForcePermission` — identical to other Cozy APIs |
| **AUTH-05** | 01-05/07/08 | Scope permission enforced on `/files/` subtree | SATISFIED | `propfind.go:89` and `get.go:52` call `middlewares.AllowVFS(permission.GET, fetcher)` |
| **READ-01** | 01-07 | PROPFIND Depth:0 on collection returns its properties | SATISFIED | `propfind.go:94-99` + `buildResponseForDir`; `propfind_test.go` Depth:0 case |
| **READ-02** | 01-07 | PROPFIND Depth:0 on file | SATISFIED | `propfind.go:96-97` + `buildResponseForFile` |
| **READ-03** | 01-07 | PROPFIND Depth:1 returns dir + immediate children | SATISFIED | `propfind.go:100-104` + `streamChildren`; E2E subtest 1 uses `ReadDir("/")` |
| **READ-04** | 01-07 | Depth:infinity → 403 | SATISFIED | `propfind.go:57-59`; E2E subtest 3 |
| **READ-05** | 01-02 | All 9 live properties with correct formats (RFC 1123 getlastmodified, md5 double-quoted getetag, ISO 8601 creationdate, empty supportedlock/lockdiscovery) | SATISFIED | `xml.go:56-89`; `buildLastModified` (`http.TimeFormat`), `buildETag` (double-quoted base64 md5), `buildCreationDate` (RFC 3339 UTC). `baseProps` + type-specific fields in `propfind.go:170-221` covers all 9. Unit tests in `xml_test.go` |
| **READ-06** | 01-02 | XML uses `D:` prefix (`xmlns:D="DAV:"`) | SATISFIED | `xml.go:167` writes root `<D:multistatus xmlns:D="DAV:">` by hand; struct tags use literal `D:` form; `TestXMLNamespacePrefix` |
| **READ-07** | 01-07 | Streaming PROPFIND (no full buffer, `DirIterator`) | SATISFIED | `propfind.go:124-143` uses `DirIterator(ByFetch=200)`; Response slice grows incrementally; only final XML body is buffered so Content-Length can be set |
| **READ-08** | 01-08 | GET file via `vfs.ServeFileContent` (Range, ETag, chunked) | SATISFIED | `get.go:59`; E2E subtest 4 verifies Range → 206 + Content-Range |
| **READ-09** | 01-08 | HEAD same headers, no body | SATISFIED | `vfs.ServeFileContent` delegates to `http.ServeContent` which handles HEAD natively; E2E subtest 4 HEAD assertions |
| **READ-10** | 01-08 | GET collection → 405 (decision: no HTML nav) | SATISFIED | `get.go:44-50`; Allow header set; E2E subtest 4 |
| **SEC-01** | 01-05 | All methods except OPTIONS require auth | SATISFIED | `webdav.go:41-44` registers non-OPTIONS under `authed` group with `resolveWebDAVAuth`; OPTIONS bypass also in `auth.go:24-26` as defence-in-depth |
| **SEC-02** | 01-03 | `path.Clean` + prefix assertion | SATISFIED | `path_mapper.go:45-49` |
| **SEC-03** | 01-07 | PROPFIND depth limits (infinity blocked, Depth:1 pagination via DirIterator) | SATISFIED | `propfind.go:57-59` (infinity) + `propfind.go:124-143` (ByFetch=200 iterator) |
| **SEC-04** | 01-05/07/08 | Audit logs on traversal + Depth:infinity + out-of-scope, NOT on 401 | SATISFIED | 5 `auditLog` call sites (all structural); `auth.go:57-80` doc forbids 401 use; `auth.go:44-48` `sendWebDAV401` has zero `auditLog` calls |
| **SEC-05** | 01-04/07 | Content-Length on every response (Finder strict) | SATISFIED | `errors.go:35-42` (error path), `propfind.go:113-114` (multistatus path), and `vfs.ServeFileContent` (file body path, via stdlib `http.ServeContent`) all set Content-Length before WriteHeader |
| **TEST-01** | 01-01/02 | Unit tests for XML written RED first | SATISFIED | `xml_test.go` (153 LoC) covers Multistatus marshal, D: prefix, 9 props, date format, ETag quoting, ResourceType discriminator |
| **TEST-02** | 01-01/03 | Unit tests for path mapping (traversal, edge cases) | SATISFIED | `path_mapper_test.go` (64 LoC) covers normalisation, encoded escapes, null byte, prefix escape |
| **TEST-04** | 01-09 | Integration tests for auth (Bearer, Basic, 401, scopes) — end-to-end gowebdav client | SATISFIED | `auth_test.go` (73 LoC) for unit-level; `gowebdav_integration_test.go:42-107` for the E2E criterion 1 + 2. Uses real `studio-b12/gowebdav` client. |

**Coverage: 28 / 28 Phase-1 requirement IDs SATISFIED.** No orphaned requirements — every ID declared in REQUIREMENTS.md's Phase-1 set is traceable to at least one `.go` artifact AND at least one passing test.

---

## Anti-Pattern Scan

| File | Pattern | Severity | Finding |
|------|---------|----------|---------|
| (all webdav *.go files) | `TODO\|FIXME\|XXX\|HACK\|PLACEHOLDER` | — | NONE FOUND |
| (all webdav *.go files) | `return null\|placeholder\|coming soon` | — | NONE FOUND |
| `handlers.go` default branch | `sendWebDAVError(501, "not-implemented")` | INFO | Intentional: Phase 2/3 write methods stub for now. Matches plan 01-06 design. Not a stub for Phase 1 read methods. |

**No blocker or warning anti-patterns.**

---

## Known Caveat (Not a Gap — Pre-Existing, Out of Scope)

**`go test ./web/webdav/... -race -count=1` FAILS** with ~6 `WARNING: DATA RACE` reports between `pkg/config/config.UseViper` (test N setup) and `config.FsURL` (read by the `AntivirusTrigger` goroutine launched by `stack.Start` in test N-1).

**Assessment (per FOLLOWUP-01 in STATE.md + 01-VALIDATION.md Gap 1):**
- Root cause is entirely in `pkg/config/config`, `model/job/trigger_antivirus.go`, `model/stack/main.go`, `tests/testutils/test_utils.go` — no WebDAV code is implicated.
- Reproducible on `master` without any Phase 1 code when any package stacks multiple `testutils.NewSetup`/`GetTestInstance` runs under `-race`.
- User decision (2026-04-05 plan 01-09 checkpoint): ship Phase 1 with explicit caveat, file as separate non-WebDAV hardening task.
- Correctness impact on Phase 1 production code: **NONE**. Every functional and security criterion passes with `-count=1`.

**Disposition in this verification:** Documented but NOT counted as a Phase 1 gap. Tracking handled by STATE.md `FOLLOWUP-01` (provisional slot `01.1-race-harness`, possibly Phase 2 Task 0).

---

## Human Verification (Deferred to Phase 3)

Per `01-VALIDATION.md §Manual-Only Verifications`, three scenarios are intentionally deferred:

1. **OnlyOffice mobile** against live Cozy — requires physical device + TLS. Deferred to Phase 3 per ROADMAP.
2. **iOS Files app** against live Cozy — requires iOS device. Deferred to Phase 3.
3. **macOS Finder** read-only browse — requires Finder instance. Deferred to Phase 3.

These are **not gaps**: the ROADMAP explicitly defers them to Phase 3 (litmus + real clients), and every in-scope Phase 1 behaviour is covered by `TestE2E_GowebdavClient` with a real `studio-b12/gowebdav` client.

---

## Summary

Phase 1 achieves its stated goal. All 5 ROADMAP success criteria are verified end-to-end by `TestE2E_GowebdavClient` (passing under `go test ./web/webdav/... -count=1`). All 28 Phase-1 requirement IDs map to concrete, substantive, and wired code AND to at least one passing test. Key security invariants (path traversal, Depth:infinity, auth isolation, Content-Length policy, audit logging exclusion on 401) are structurally enforced, not just tested. The `-race` caveat is pre-existing, out of scope, and user-approved.

**Verdict:** Phase 1 PASSED. Ready to proceed to Phase 2 planning. The `-race` race remains as FOLLOWUP-01 — strongly recommend addressing it as Phase 2 Task 0 before Phase 2 tests begin stacking fresh `-race` runs.

---

*Verified: 2026-04-05*
*Verifier: Claude (gsd-verifier)*

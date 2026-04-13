# Project Research Summary

**Project:** cozy-stack WebDAV v1.2 — Robustness Beyond Litmus
**Domain:** WebDAV server robustness testing and correctness hardening on a Go/Swift/CouchDB stack
**Researched:** 2026-04-12
**Confidence:** HIGH

## Executive Summary

v1.1 shipped a fully litmus-compliant WebDAV server (63/63) with streaming PUT via `io.Copy` and single-range GET via `http.ServeContent`. v1.2 is not a feature release — it is a robustness and evidence release: prove the existing implementation handles multi-GB transfers, connection drops, and concurrent writes correctly, then automate that proof in CI. The research finding is that the cozy-stack VFS layer is already designed correctly for all target scenarios (streaming PUT, interrupted PUT cleanup, atomic CouchDB commits), so v1.2 is primarily a testing and evidence-gathering effort with minimal production code changes.

The recommended approach is to write targeted tests in layers: first establish streaming proof infrastructure (memory measurement helpers, deterministic fixture generation), then interrupted PUT isolation, then byte-range and concurrency edge cases, and finally wire CI litmus automation. Total estimated production code changes are small (~30 LOC: a Content-Range PUT rejection guard and a CouchDB 409 to HTTP 409 mapping fix). All test infrastructure can be built from stdlib — zero new Go module dependencies are required.

The key risk is measurement methodology, not correctness. Go's runtime arena retention makes naive RSS-based streaming proof meaningless: `HeapAlloc` must be sampled concurrently during the in-flight transfer, not post-hoc. Concurrency tests that use `time.Sleep` for synchronisation will produce flaky CI. Both are well-understood patterns with established solutions documented in the pitfalls research.

---

## Key Findings

### Recommended Stack

The v1.1 server stack — custom Echo handlers, `encoding/xml`, `net/http.ServeContent`, `studio-b12/gowebdav` for test clients, `gavv/httpexpect` for raw HTTP assertions — remains unchanged for v1.2. All robustness testing tooling is stdlib.

**Core technologies (unchanged from v1.1):**
- `encoding/xml` (stdlib): RFC 4918 XML marshalling — zero dependency, correct namespace handling
- `net/http.ServeContent` (stdlib): handles all range GET logic including multi-range `multipart/byteranges` since Go 1.1
- `studio-b12/gowebdav` v0.12.0: integration test WebDAV client — already in go.mod
- `gavv/httpexpect/v2`: raw HTTP assertions for edge cases beyond gowebdav's reach
- `owncloud/litmus` Docker image: CI litmus runner — preferred over `apt install litmus` (apt package is v0.13 from 2014, severely stale)

**New for v1.2 (all stdlib, no go.mod additions):**
- `io.Pipe`: interrupted PUT simulation — synchronous, error-propagating, in-process
- `runtime.ReadMemStats` + `runtime/metrics`: streaming memory bound assertions
- `mime`, `mime/multipart`: multi-range response parsing in tests
- `golang.org/x/perf/cmd/benchstat`: CLI tool for benchmark comparison (tool-install only, not a module dep)

### Expected Features

**Must have (v1.2 core — P1):**
- FOLLOWUP-01 race harness fix: pre-existing data race in `pkg/config`/`model/stack`/`model/job` blocks `-race` usage — must be resolved first to unblock clean test runs (~50–100 LOC in test harness)
- Content-Range PUT rejection (400): ~10 LOC guard in `put.go`; RFC 7231 §4.3.4 explicitly requires this; v1.1 currently leaves this ambiguous
- Interrupted PUT test: verify no partial file or orphaned CouchDB doc after connection drop; zero production code change expected (VFS `Close()` cleanup is already correct by design)
- Large-file streaming benchmark: 1 GB PUT with `HeapAlloc` ceiling assertion; converts "we stream" from claim to measured evidence
- Multi-range GET test: assert `Range: bytes=0-9,20-29` returns `206 multipart/byteranges`; no production code needed
- Concurrent write test + CouchDB 409 fix: two goroutines PUT same path; assert deterministic outcome; ~20 LOC to map CouchDB MVCC conflicts from 500 to 409
- CI litmus integration: new `.github/workflows/webdav-litmus.yml` using `owncloud/litmus` Docker image; closes automation debt deferred from v1.1

**Should have (v1.2 P2, requires device/CI availability):**
- iOS Files formal sign-off: manual checklist on physical iOS device; zero code; ~1–2 hours; requires HTTPS staging endpoint

**Defer (v2+):**
- Nextcloud chunked upload protocol: proprietary, ~500+ LOC + cleanup job, only helps Nextcloud-dialect clients; single streaming PUT already works for all in-scope clients
- WebDAV LOCK / Class 2: requires VFS lock subsystem redesign; explicitly out of scope
- PROPPATCH CouchDB persistence: dead properties currently in-memory only; passes litmus, low user impact

### Architecture Approach

The system is a thin Echo handler layer (`web/webdav/`) that dispatches to `vfs.VFS` — no business logic in handlers. All streaming, atomicity, and concurrency guarantees live in the VFS layer (`model/vfs/vfsswift/impl_v3.go`). The critical architectural finding: `swiftFileCreationV3.Close()` already implements transactional cleanup (partial Swift object deleted + CouchDB doc deleted on error) — interrupted PUT correctness is guaranteed by design, just untested. The test harness (`testutil_test.go`) provides an `httptest.Server` backed by `aferoVFS` (in-process, no Swift), with CouchDB required for metadata.

**Major components and v1.2 integration points:**
1. `web/webdav/put.go` — PUT handler; `io.Copy` at line 104 streams directly; error propagation to `file.Close()` is correct; add Content-Range guard near line 80
2. `model/vfs/vfsswift/impl_v3.go` — `swiftFileCreationV3.Close()` at line 865; transactional cleanup defer already exists; concurrent create race resolved by second `DirChildExists` check at line 946
3. `model/vfs/file.go` — `ServeFileContent` at line 251; delegates all range/ETag logic to `http.ServeContent`; correct for single and multi-range with zero additional code
4. `web/webdav/testutil_test.go` — test harness; extend for large bodies (`io.LimitReader`), connection drops (`env.TS.CloseClientConnections()`), and concurrent clients (two `gowebdav.NewClient` calls)
5. `.github/workflows/webdav-litmus.yml` — new CI job; model after `system-tests.yml`; use `owncloud/litmus` Docker image with `--network host`

### Critical Pitfalls

1. **Body-accumulating test helpers defeat streaming validation** — `httpexpect`'s `Body().Raw()` calls `io.ReadAll` internally; use `io.TeeReader` into `sha256.New()` + `io.Discard` for large-file response verification; never allocate the full body as `[]byte` for files > 1 MB

2. **Post-hoc RSS measurement proves nothing** — Go arenas do not release to OS immediately after GC; sample `HeapAlloc` (not RSS) concurrently during the in-flight transfer; alternatively use a `GOMEMLIMIT` subprocess that would OOM if the body is buffered

3. **Sleep-based concurrency synchronisation is always wrong** — use `sync.WaitGroup` + channels; add `goleak.VerifyNone(t)` to catch goroutine leaks between tests; run concurrency tests with `-count=10 -race`

4. **CouchDB startup race in CI** — Docker `healthy` status does not mean CouchDB is accepting queries; add an explicit `curl` poll loop (30 retries × 1s on `/_up`) before running any tests

5. **Binary test fixtures in git** — generate programmatically via `io.LimitReader(rand.Reader, N)` in `t.TempDir()`; one committed 1 GB fixture makes every checkout impractical

---

## Implications for Roadmap

All dependencies flow from instrumentation → proof → edge cases → CI. The recommended build order from ARCHITECTURE.md maps cleanly to phases.

### Phase 1: Prerequisites and Instrumentation
**Rationale:** The race condition in `pkg/config`/`model/stack`/`model/job` causes non-deterministic test failures that would corrupt memory measurements in all subsequent phases. The memory measurement helper and fixture generation pattern must be locked in before any large-file test is written.
**Delivers:** Clean `-race` CI, reusable `HeapAlloc` sampling helper, fixture generation convention established
**Addresses:** FOLLOWUP-01 race fix, streaming measurement methodology
**Avoids:** Pitfall 2 (post-hoc RSS), Pitfall 9 (git fixture bloat)

### Phase 2: Streaming Proof (Large Files)
**Rationale:** Proves or disproves the central "we stream" claim. If middleware buffering is found, this phase surfaces it and determines whether a production fix is needed — must come before interrupted PUT which depends on the streaming path being clean.
**Delivers:** `TestPut_LargeFile_Streaming` and `TestGet_LargeFile` with `HeapAlloc` ceiling assertions; streaming converted from assertion to evidence
**Addresses:** Large-file streaming benchmark, large-file GET test
**Avoids:** Pitfall 1 (body accumulation), Pitfall 2 (RSS measurement)
**Research flag:** Echo middleware body-buffering must be verified empirically in this phase; outcome determines whether any production code change is needed

### Phase 3: Interrupted PUT and Defensive Guards
**Rationale:** VFS cleanup correctness is designed in; this phase writes the test that verifies it under real disconnect conditions, and adds the Content-Range rejection guard as a small standalone fix.
**Delivers:** `TestPut_Interrupted_NoOrphan` (new file + overwrite paths), Content-Range PUT `400` guard (~10 LOC in `put.go`)
**Addresses:** Interrupted PUT test, Content-Range PUT rejection
**Avoids:** Pitfall 3 (partial file committed, overwrite rollback deletes original)

### Phase 4: Byte-Range Edge Cases
**Rationale:** Multi-range GET is already handled by `http.ServeContent`; this phase documents and guards the behavior, including RFC 7233 edge cases that are commonly missed (416, full-file range → 200, `If-Range`).
**Delivers:** `TestGet_MultiRange`, edge case tests for 416 / 200 / `If-Range` precondition
**Addresses:** Multi-range GET test
**Avoids:** Pitfall 6 (wrong 206 vs 200, missing 416 for out-of-bounds range)

### Phase 5: Concurrency Hardening
**Rationale:** Most complex test; requires Phases 1–3 stable to avoid flakiness from unrelated races. The CouchDB 409 mapping fix is a latent correctness bug that should ship regardless.
**Delivers:** `TestPut_Concurrent_SamePath` with proper channel synchronisation and `goleak.VerifyNone(t)`, CouchDB 409 → HTTP 409 mapping fix (~20 LOC)
**Addresses:** Concurrent write test, CouchDB 409 fix
**Avoids:** Pitfall 4 (sleep-based sync), Pitfall 5 (goroutine leaks between tests)

### Phase 6: CI Litmus Automation
**Rationale:** Validates the combined state of all prior phases; litmus 63/63 must hold after all changes. Can be drafted in parallel with Phases 2–5 but should merge last.
**Delivers:** `.github/workflows/webdav-litmus.yml` using `owncloud/litmus` Docker image; automated compliance regression detection on every PR
**Addresses:** CI litmus integration
**Avoids:** Pitfall 8 (CouchDB startup race via poll loop; litmus version drift via Docker instead of apt)

### Phase 7: iOS Sign-Off (conditional on device availability)
**Rationale:** Requires physical iOS device and HTTPS staging endpoint; zero code changes; can be deferred if neither is available.
**Delivers:** Manual validation checklist completed against FileBrowser Pro / Keynote WebDAV browser
**Addresses:** iOS Files formal sign-off
**Avoids:** Pitfall 10 (URL session cache masking bugs; only happy-path testing)

### Phase Ordering Rationale

- Phase 1 first: race fix is a prerequisite for reliable memory measurement; measurement helper must exist before any large-file test to prevent Pitfalls 1 and 9
- Phases 2–5 follow code dependencies: streaming proof before interrupted PUT (same io.Copy path), interrupted PUT before concurrency (both touch `Close()`)
- Phase 6 last: validates combined state; 63/63 litmus baseline must hold through all changes
- Phase 7 independent: can slot anywhere after Phase 3 given a device and staging endpoint

### Research Flags

Phases needing deeper investigation during execution:
- **Phase 2:** Echo middleware body-buffering — no external research needed, but requires reading Echo middleware source and running a diagnostic profiling test; outcome determines whether a production fix is needed
- **Phase 6:** `owncloud/litmus` Docker image + `--network host` in GitHub Actions — the workflow outline in STACK.md is a recommendation, not a tested config; a trial CI run will be needed to validate Docker networking

Phases with standard well-documented patterns (no additional research needed):
- **Phase 1:** Standard Go race fix methodology; `runtime.ReadMemStats` is documented API
- **Phase 3:** `io.Pipe` simulation fully documented with code examples in STACK.md
- **Phase 4:** Fully delegated to `http.ServeContent`; test pattern is mechanical
- **Phase 5:** Concurrency patterns fully documented in PITFALLS.md and ARCHITECTURE.md with code examples

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All tooling from direct source inspection of go.mod and existing harness; zero new dependencies reduces risk |
| Features | HIGH | Scope tightly bounded by existing code; table stakes verified against RFC 7231/7233 and major implementations |
| Architecture | HIGH | All findings from direct code inspection; integration points are precise with line numbers; no inference from docs alone |
| Pitfalls | HIGH (Go specifics) / MEDIUM (iOS) | Go runtime behavior from official docs; iOS URL session cache from community reports only |

**Overall confidence:** HIGH

### Gaps to Address

- **Echo middleware body-buffering (Phase 2):** Not verified empirically. If any middleware calls `io.ReadAll` on the request body before the PUT handler runs, the streaming proof fails and a production fix is needed. This is the single highest-priority unknown.
- **ETag inconsistency between PROPFIND and GET:** PROPFIND returns `doc.DocRev` (CouchDB `_rev`); GET returns `base64(doc.MD5Sum)`. Pre-existing, out of scope for v1.2, but needs a code comment before iOS sign-off to prevent future confusion.
- **CouchDB 409 exact trigger conditions (Phase 5):** The exact conditions under which CouchDB MVCC returns a conflict error to the VFS layer are inferred from code structure, not directly tested. The Phase 5 test must directly trigger the conflict path to validate the fix, not rely on inference.
- **iOS 17+ behavioral changes (Phase 7):** iOS 17 changed `User-Agent` and added `If-None-Match` on COPY. If sign-off is attempted, an iOS 17+ device is required, not an older device.

---

## Sources

### Primary (HIGH confidence — official docs and first-party code)
- cozy-stack `web/webdav/put.go`, `get.go`, `handlers.go` — handler logic and io.Copy location
- cozy-stack `model/vfs/vfsswift/impl_v3.go` — Close() deferred cleanup (line 865), CreateFile locking (line 188), concurrent create race (line 946)
- cozy-stack `model/vfs/file.go` — ServeFileContent → http.ServeContent delegation (line 251)
- cozy-stack `web/webdav/testutil_test.go`, `gowebdav_integration_test.go` — test harness capabilities
- cozy-stack `.github/workflows/go-tests.yml`, `system-tests.yml` — CI patterns
- cozy-stack `scripts/webdav-litmus.sh` — existing litmus orchestration
- Go stdlib `runtime.MemStats` documentation — HeapAlloc vs HeapInuse vs RSS distinction
- RFC 7231 §4.3.4 — Content-Range on PUT must return 400
- RFC 7233 §4 — 206, 416, If-Range semantics
- golang/go issue #3784 — multi-range ServeContent fixed Go 1.1+
- sabre.io/dav/large-files/ — streaming architecture, 15 GB claim (official docs)
- Nextcloud Developer Docs — chunked upload protocol specification (official docs)

### Secondary (MEDIUM confidence — community sources, issue trackers)
- owncloud-ci/litmus GitHub repo — Docker image env vars, maintenance status (last update May 2024)
- cs3org/reva issue #86 — WebDAV PUT atomicity patterns
- owncloud/core issue #1051 — Content-Range PUT explicitly refused, ecosystem precedent
- rclone forum — WebDAV 16 GB OOM root cause (NTLMSSP buffering, not the WebDAV layer)
- go.uber.org/goleak — goroutine leak detection API

### Tertiary (LOW confidence — single source or inference)
- iOS 17 WebDAV behavioral changes — community reports, not Apple documentation
- CouchDB 409 error propagation path through VFS layers — inferred from code structure, not directly tested

---
*Research completed: 2026-04-12*
*Ready for roadmap: yes*

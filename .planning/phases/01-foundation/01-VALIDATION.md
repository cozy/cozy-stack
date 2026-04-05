---
phase: 1
slug: foundation
status: verified-except-race
nyquist_compliant: false
wave_0_complete: true
created: 2026-04-05
finalized: 2026-04-05
approval: pending-race-decision
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` (stdlib) + `testify` (already in go.mod) |
| **Config file** | `go.mod` (tests alongside code in `web/webdav/*_test.go`) |
| **Quick run command** | `go test ./web/webdav/... -count=1` |
| **Full suite command** | `go test ./web/webdav/... ./pkg/webdav/... -race -count=1` |
| **Estimated runtime** | ~30-60 seconds (unit + integration with test cozy instance) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./web/webdav/... -count=1 -run <relevant test>` (targeted, <5s)
- **After every plan wave:** Run full suite with `-race`
- **Before `/gsd:verify-work`:** Full suite must be green, including gowebdav integration tests
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

Finalized 2026-04-05. Every Phase 1 plan (01-01 through 01-09) is mapped to its actual automated verify command against the real test tree.

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | Status |
|---------|------|------|-------------|-----------|-------------------|--------|
| 01-01 | Scaffold + gowebdav + stubs | 0 | TEST-01, TEST-02 | scaffold | `go build ./web/webdav/...` | ✅ green |
| 01-02 | XML multistatus + D: prefix + dates + ETag | 1 | READ-01, READ-02, READ-04, READ-05, READ-06 | unit | `go test ./web/webdav/ -run 'TestMultistatus\|TestGetLastModifiedFormat\|TestBuildETag\|TestResourceType\|TestXMLNamespacePrefix' -count=1` | ✅ green |
| 01-03 | davPathToVFSPath traversal prevention | 1 | ROUTE-03, SEC-02 | unit | `go test ./web/webdav/ -run TestDavPathToVFSPath -count=1` | ✅ green |
| 01-04 | §8.7 error XML builder + sendWebDAVError | 1 | SEC-05 (error Content-Length) | unit | `go test ./web/webdav/ -run 'TestBuildErrorXML\|TestSendWebDAVError' -count=1` | ✅ green |
| 01-05 | Auth middleware + test harness | 2 | AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, SEC-01, SEC-04 | integration | `go test ./web/webdav/ -run TestAuth -count=1` | ✅ green |
| 01-06 | Routes + OPTIONS + Nextcloud 308 + routing.go | 3 | ROUTE-01, ROUTE-02, ROUTE-04, ROUTE-05 | integration | `go test ./web/webdav/ -run 'TestOptions\|TestNextcloud' -count=1` | ✅ green |
| 01-07 | PROPFIND Depth 0/1/infinity + DirIterator | 4 | READ-01..07, SEC-03, SEC-04 | integration | `go test ./web/webdav/ -run TestPropfind -count=1` | ✅ green |
| 01-08 | GET/HEAD via ServeFileContent + Range + 405 | 4 | READ-08, READ-09, READ-10 | integration | `go test ./web/webdav/ -run 'TestGet\|TestHead' -count=1` | ✅ green |
| 01-09 | End-to-end gowebdav client + 5 success criteria | 5 | TEST-04, SEC-05 + ALL Phase 1 criteria | E2E | `go test ./web/webdav/ -run TestE2E_GowebdavClient -count=1` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

**Full package run:** `COZY_COUCHDB_URL=http://admin:password@localhost:5984/ go test ./web/webdav/... -count=1 -timeout 5m` — **PASSES** in ~7s.

**Build + vet:** `go build ./...` clean, `go vet ./web/webdav/...` clean.

**Race-enabled run (`-race`):** FAILS due to a pre-existing test-infrastructure race unrelated to WebDAV code. See **Outstanding Gaps** below.

**Coverage invariant:** All 28 Phase 1 REQ-IDs mapped and verified green by at least one test. AUTH-01..05, READ-01..10, ROUTE-01..05, SEC-01..05, TEST-01, TEST-02, TEST-04 all complete per REQUIREMENTS.md.

---

## Wave 0 Requirements

Wave 0 establishes the test scaffolding and the failing tests (RED phase of TDD). Nothing else can proceed until these land.

- [ ] `web/webdav/` directory created
- [ ] `github.com/studio-b12/gowebdav@v0.12.0` added to `go.mod` via `go get`
- [ ] `web/webdav/xml.go` — empty stub file for XML structs (will be TDD-filled)
- [ ] `web/webdav/xml_test.go` — RED tests for multistatus marshalling, namespace `D:` prefix, 9 properties, date format, ETag quoting
- [ ] `web/webdav/path.go` — empty stub for path mapping
- [ ] `web/webdav/path_test.go` — RED tests for path normalization, URL encoding, traversal rejection, prefix assertion
- [ ] `web/webdav/auth.go` — empty stub for auth middleware
- [ ] `web/webdav/auth_test.go` — RED tests for Bearer header, Basic Auth password field, 401 WWW-Authenticate realm
- [ ] `web/webdav/errors.go` — empty stub for RFC 4918 §8.7 error XML builder
- [ ] `web/webdav/errors_test.go` — RED tests for error XML format and precondition elements
- [ ] `web/webdav/handler_propfind.go` — empty stub
- [ ] `web/webdav/propfind_test.go` — RED tests with gowebdav client hitting a test instance
- [ ] `web/webdav/handler_get.go` — empty stub
- [ ] `web/webdav/get_test.go` — RED tests for GET file + HEAD + Range + collection→405
- [ ] `web/webdav/routes.go` — empty stub for Routes(group)
- [ ] `web/webdav/options_test.go` — RED tests for OPTIONS (no auth, `DAV: 1`, Allow list)
- [ ] `web/webdav/testutil_test.go` — helpers: setup test instance, authenticated gowebdav client, common fixtures
- [ ] `web/routing.go` — add import of `web/webdav` (commented until auth wave lands) and stub call

**TDD discipline:** Each test file in Wave 0 must be committed with `test(webdav): add RED tests for X` before any non-test file gains body. GREEN implementation comes in subsequent waves.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| OnlyOffice mobile scenario | Phase 1 success criteria | Requires real mobile app + TLS cert | Defer to Phase 3 (litmus + real clients) |
| iOS Files app scenario | Phase 1 success criteria | Requires real iPad/iPhone + TLS | Defer to Phase 3 |
| macOS Finder browse | Phase 1 success criteria | Requires real Finder instance | Defer to Phase 3 (expected read-only) |

**All Phase 1 behaviors have automated verification via gowebdav client against a test cozy instance.** Real-client scenarios are deferred to Phase 3 per ROADMAP.md.

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify commands (see Per-Task Verification Map)
- [x] Sampling continuity: every plan has at least one automated verify
- [x] Wave 0 covers all test scaffolding references
- [x] No watch-mode flags
- [x] Feedback latency < 60s (full suite ~7s, individual plans <2s each)
- [ ] `nyquist_compliant: true` — **BLOCKED on race gap decision**, see below

**Approval:** pending — race gap decision required.

---

## Outstanding Gaps

### Gap 1 — Pre-existing test-infrastructure race under `-race`

**Discovered:** Plan 01-09 Task 2 (final race-enabled sweep — the first time `-race` has been run on the webdav package suite).

**Symptom:** `go test ./web/webdav/... -race -count=1` FAILS with ~6 `WARNING: DATA RACE` reports. Every race is at the same address pair:

```
Write at config.UseViper (pkg/config/config/config.go:1009)
  ← config.UseTestFile (pkg/config/config/config.go:1387)
  ← web/webdav.newWebdavTestEnv (testutil_test.go:41)    ← test N

Previous read at config.FsURL (pkg/config/config/config.go:475)
  ← model/instance.(*Instance).MakeVFS (model/instance/instance.go:239)
  ← model/instance.(*InstanceService).Get (model/instance/service.go:50)
  ← model/job.(*AntivirusTrigger).pushJob (model/job/trigger_antivirus.go:102)
  ← model/job.(*memScheduler).StartScheduler (model/job/mem_scheduler.go:59)
  ← model/stack.Start (model/stack/main.go:104)
  ← testutils.(*TestSetup).GetTestInstance (tests/testutils/test_utils.go:178)    ← test N-1
```

**Root cause:** The cozy-stack test harness mutates `config.*` package globals via `config.UseTestFile` at the start of every test, but `testutils.GetTestInstance` launches `stack.Start`, which in turn spawns a `memScheduler` goroutine that owns an `AntivirusTrigger` that reads `config.FsURL()` on a long-lived timer. That goroutine outlives the test that started it and collides with the *next* test's viper mutation. The race is between test N's setup goroutine and test N-1's still-running antivirus scheduler.

**Scope assessment:** This race is **entirely outside the web/webdav package**. It is a latent bug in the stack-wide test fixture (`pkg/config/config/config.go` + `model/job/trigger_antivirus.go` + `model/stack/main.go` + `tests/testutils/test_utils.go`). It is exposed for the first time by Plan 01-09 only because 01-09 is the first plan in Phase 1 that runs the full suite with `-race`. All eight preceding Phase 1 plans ran without `-race` and their SUMMARY self-checks passed.

**Directly caused by Phase 1 code:** NO. Reproducible with every test in web/webdav/ that uses `newWebdavTestEnv` (i.e. the helper from 01-05 and every *_test.go that uses it), and reproducible after temporarily removing `gowebdav_integration_test.go` (the file added in 01-09 Task 1). The race exists on `master` for any package that stacks multiple `testutils.NewSetup` tests and runs them with `-race`.

**Fix surface:** Either (a) serialize the config globals behind a mutex in `pkg/config/config/config.go` (touches stack-wide config primitives), (b) stop the antivirus trigger in test cleanup (touches `tests/testutils/test_utils.go` and the job scheduler lifecycle), or (c) add a `t.Cleanup` hook in `newWebdavTestEnv` that stops the stack — none of which are Phase 1 WebDAV concerns and all of which belong to a separate hardening task on a non-webdav package.

**Correctness impact on Phase 1:** NONE. The race is in test setup code, not production code. The WebDAV code itself has zero data-race reports. Every functional test passes with `-count=1` (no `-race`). Every Phase 1 ROADMAP success criterion is verified green by `TestE2E_GowebdavClient` and its 5 subtests.

**Recommended disposition:**
- File a separate hardening task (non-Phase-1) to either stop the stack scheduler in test teardown OR add a mutex/atomic to `config.*` globals
- Document the limitation in Phase 1 sign-off: "Phase 1 is functionally complete and passes the full suite with `-count=1`. The `-race` invariant is blocked by a pre-existing stack-wide test-harness race with no WebDAV involvement."
- Ship Phase 1 as green (functional + security criteria all satisfied) with this caveat captured in STATE.md as an open todo
- OR fix the infrastructure race before merging Phase 1 — user's call

**Verification after disposition:** once the harness bug is fixed, re-run `go test ./web/webdav/... -race -count=1 -timeout 5m` and flip `nyquist_compliant: true` + check the box above.

---
phase: 1
slug: foundation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-05
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

Populated by planner. Each task maps to one or more REQ-IDs from Phase 1 and has an automated verify command.

Template rows (to be expanded by planner):

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 1-01-01 | 01 | 0 | TEST-01 | unit | `go test ./web/webdav/ -run TestXMLMultistatus` | ❌ W0 | ⬜ pending |
| 1-01-02 | 01 | 1 | READ-06 | unit | `go test ./web/webdav/ -run TestXMLNamespacePrefix` | ❌ W0 | ⬜ pending |
| 1-02-01 | 02 | 1 | ROUTE-03, SEC-02 | unit | `go test ./web/webdav/ -run TestPathMapping` | ❌ W0 | ⬜ pending |
| 1-03-01 | 03 | 2 | AUTH-01, AUTH-02 | integration | `go test ./web/webdav/ -run TestAuthMiddleware` | ❌ W0 | ⬜ pending |
| 1-04-01 | 04 | 3 | READ-01..05 | integration | `go test ./web/webdav/ -run TestPropfindDepth0` | ❌ W0 | ⬜ pending |
| 1-04-02 | 04 | 3 | READ-03, READ-07 | integration | `go test ./web/webdav/ -run TestPropfindStreaming` | ❌ W0 | ⬜ pending |
| 1-05-01 | 05 | 4 | READ-08, READ-09 | integration | `go test ./web/webdav/ -run TestGetFile` | ❌ W0 | ⬜ pending |
| 1-06-01 | 06 | 4 | ROUTE-01, ROUTE-04 | integration | `go test ./web/webdav/ -run TestOptionsRoute` | ❌ W0 | ⬜ pending |
| 1-07-01 | 07 | 5 | TEST-04, AUTH-03 | E2E | `go test ./web/webdav/ -run TestGowebdavClient` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

**Coverage invariant:** Every Phase 1 REQ-ID must appear in at least one row. Planner fills this in detail.

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

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references (test scaffolding for every *_test.go above)
- [ ] No watch-mode flags (no `-watch`, no `go test -run .` loops)
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter after planner fills in per-task map

**Approval:** pending

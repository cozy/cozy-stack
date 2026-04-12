---
phase: 02
slug: write-operations
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-06
approved: 2026-04-12
---

# Phase 02 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — Phase 1 established test harness |
| **Quick run command** | `COZY_COUCHDB_URL=http://admin:password@localhost:5984/ go test ./web/webdav/ -count=1 -timeout 5m` |
| **Full suite command** | `COZY_COUCHDB_URL=http://admin:password@localhost:5984/ go test ./web/webdav/ -count=1 -timeout 5m -v` |
| **Estimated runtime** | ~8 seconds |

---

## Sampling Rate

- **After every task commit:** Run quick run command
- **After every plan wave:** Run full suite command
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 02-XX-01 | TBD | TBD | WRITE-01 | integration | `go test ./web/webdav/ -run TestPut -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-02 | TBD | TBD | WRITE-02 | integration | `go test ./web/webdav/ -run TestPut -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-03 | TBD | TBD | WRITE-03 | integration | `go test ./web/webdav/ -run TestPutConditional -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-04 | TBD | TBD | WRITE-04 | integration | `go test ./web/webdav/ -run TestPutParent -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-05 | TBD | TBD | WRITE-05 | integration | `go test ./web/webdav/ -run TestDelete -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-06 | TBD | TBD | WRITE-06 | integration | `go test ./web/webdav/ -run TestDeleteDir -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-07 | TBD | TBD | WRITE-07 | integration | `go test ./web/webdav/ -run TestMkcol -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-08 | TBD | TBD | WRITE-08 | integration | `go test ./web/webdav/ -run TestMkcolParent -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-09 | TBD | TBD | WRITE-09 | integration | `go test ./web/webdav/ -run TestMkcolExists -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-10 | TBD | TBD | MOVE-01 | integration | `go test ./web/webdav/ -run TestMove -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-11 | TBD | TBD | MOVE-02 | integration | `go test ./web/webdav/ -run TestMoveDir -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-12 | TBD | TBD | MOVE-03 | integration | `go test ./web/webdav/ -run TestMoveOverwrite -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-13 | TBD | TBD | MOVE-04 | integration | `go test ./web/webdav/ -run TestMoveOverwrite -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-14 | TBD | TBD | MOVE-05 | integration | `go test ./web/webdav/ -run TestMoveTraversal -count=1` | ❌ W0 | ⬜ pending |
| 02-XX-15 | TBD | TBD | TEST-03 | integration | `go test ./web/webdav/ -run TestE2E -count=1` | ✅ exists | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `web/webdav/put_test.go` — RED tests for PUT create, overwrite, streaming, conditional, 409
- [ ] `web/webdav/delete_test.go` — RED tests for DELETE file, dir, trash items
- [ ] `web/webdav/mkcol_test.go` — RED tests for MKCOL create, exists, missing parent
- [ ] `web/webdav/move_test.go` — RED tests for MOVE rename, reparent, overwrite, traversal

*Existing infrastructure covers framework and fixtures (testutil_test.go from Phase 1).*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| OnlyOffice mobile open→edit→save | TEST-03 (partial) | Requires real OnlyOffice mobile app | Connect OnlyOffice to test instance, open doc, edit, save, verify content persisted |

*All other behaviors have automated verification via gowebdav integration tests.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references — all four test files landed with plan 02-01..02-04
- [x] No watch-mode flags
- [x] Feedback latency < 10s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved (retrospective, 2026-04-12 during milestone v1.1 audit)

**Notes on retrospective approval:**

This VALIDATION.md remained in `draft` state throughout Phase 2 execution. Phase 2 verification nonetheless passed (5/5 ROADMAP success criteria, 15/15 requirements) because:

1. **Test harness inherited from Phase 1** — `testutil_test.go` was already established; Phase 2 reused it without modification, so the Wave-0 infrastructure bootstrap was effectively done by Phase 1.
2. **All planned test files exist and are green:**
   - `web/webdav/put_test.go` ✓
   - `web/webdav/delete_test.go` ✓
   - `web/webdav/mkcol_test.go` ✓
   - `web/webdav/move_test.go` ✓
   - `web/webdav/write_integration_test.go` (E2E gowebdav, TEST-03) ✓
3. **All 15 requirements (WRITE-01..09, MOVE-01..05, TEST-03) are traceable to concrete tests** — verified by the 02-VERIFICATION.md requirements table.

The draft status reflected a process omission (wave-0 retrospective loop not formally closed mid-execution), not a substantive quality gap. Promoted to `approved` during the v1.1 milestone audit (`.planning/v1.1-MILESTONE-AUDIT.md`) after cross-referencing VERIFICATION + SUMMARY + REQUIREMENTS traceability.

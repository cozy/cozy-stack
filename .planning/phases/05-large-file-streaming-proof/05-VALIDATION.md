---
phase: 5
slug: large-file-streaming-proof
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-14
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — stdlib |
| **Quick run command** | `go test ./web/webdav/ -run TestPut_LargeFile_Streaming -timeout 10m` |
| **Full suite command** | `go test ./web/webdav/ -timeout 15m` |
| **Estimated runtime** | ~120 seconds (includes 1 GB PUT + 1 GB GET) |

---

## Sampling Rate

- **After every task commit:** Run `{quick run command}` (matching test only)
- **After every plan wave:** Run `go test ./web/webdav/ -timeout 15m`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 180 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 05-01-01 | 01 | 1 | LARGE-01 | integration | `go test ./web/webdav/ -run TestPut_LargeFile_Streaming -timeout 10m` | ❌ W0 | ⬜ pending |
| 05-01-02 | 01 | 1 | LARGE-02 | integration | `go test ./web/webdav/ -run TestGet_LargeFile -timeout 10m` | ❌ W0 | ⬜ pending |
| 05-01-03 | 01 | 1 | LARGE-01,02 | code review | `! grep -E 'io\.ReadAll\|Body\(\)\.Raw\(\)' web/webdav/large_test.go` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `web/webdav/large_test.go` — new test file for TestPut_LargeFile_Streaming and TestGet_LargeFile
- [ ] `web/webdav/testhelpers_test.go` — add `runtime.GC()` call before first sample in `measurePeakHeap`

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| No accumulating buffer on large body path | LARGE-01, LARGE-02 | Static code review — grep confirms but intent requires human verification | `grep -nE "io\.ReadAll\|Body\(\)\.Raw\(\)\|bytes\.Buffer" web/webdav/large_test.go` returns no matches on body path |
| No 1 GB fixture committed to repo | LARGE-01, LARGE-02 | git history check | `git log --all --diff-filter=A --name-only \| grep -iE '(1gb\|large\.bin\|fixture.*bin)'` returns nothing |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 180s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending

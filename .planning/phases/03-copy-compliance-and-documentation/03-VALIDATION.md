---
phase: 3
slug: copy-compliance-and-documentation
status: approved
nyquist_compliant: true
wave_0_complete: false
created: 2026-04-11
approved: 2026-04-11
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

Derived from `03-RESEARCH.md §8 Validation Architecture`.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + `github.com/stretchr/testify v1.11.1` |
| **Config file** | `cozy.test.yaml` (via `config.UseTestFile(t)`) |
| **Quick run command** | `go test -p 1 -timeout 5m -run TestCopy ./web/webdav/...` |
| **Full suite command** | `go test -p 1 -timeout 5m ./web/webdav/...` |
| **Compliance command** | `scripts/webdav-litmus.sh` (or `make test-litmus`) |
| **Estimated runtime** | Quick: ~5s · Full `./web/webdav/...`: ~30s · Litmus × 2 routes: ~90s |

---

## Sampling Rate

- **After every task commit:** Run quick command — scoped to `TestCopy*` to keep feedback < 10s
- **After every plan wave:** Run full suite `go test -p 1 -timeout 5m ./web/webdav/...`
- **Before `/gsd:verify-work`:** Full suite green + `scripts/webdav-litmus.sh` passes on BOTH routes (`/dav/files/` AND `/remote.php/webdav/`)
- **Max feedback latency:** 10 seconds for unit layer, 90s for compliance layer

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 03-01-01 | 01-copy-file | 1 | COPY-01, COPY-03 | unit | `go test -run TestCopy_File ./web/webdav/...` | ❌ W0 | ⬜ pending |
| 03-01-02 | 01-copy-file | 1 | COPY-01 | unit | `go test -run TestCopy_File_Notes ./web/webdav/...` | ❌ W0 | ⬜ pending |
| 03-02-01 | 02-copy-dir | 2 | COPY-02 | unit | `go test -run TestCopy_Dir ./web/webdav/...` | ❌ W0 | ⬜ pending |
| 03-02-02 | 02-copy-dir | 2 | COPY-02 | unit | `go test -run TestCopy_Dir_Depth ./web/webdav/...` | ❌ W0 | ⬜ pending |
| 03-02-03 | 02-copy-dir | 2 | COPY-02 | unit | `go test -run TestCopy_Dir_207 ./web/webdav/...` | ❌ W0 | ⬜ pending |
| 03-03-01 | 03-e2e-gowebdav | 3 | TEST-05 | integration | `go test -run TestE2E_GowebdavClient/SuccessCriterion6 ./web/webdav/...` | ❌ W0 | ⬜ pending |
| 03-04-01 | 04-litmus-script | 3 | TEST-06 | external | `scripts/webdav-litmus.sh --dry-run` | ❌ W0 | ⬜ pending |
| 03-05-01 | 05-litmus-basic | 4 | TEST-06 | external | `LITMUS_TESTS="basic" scripts/webdav-litmus.sh` | ❌ W0 | ⬜ pending |
| 03-05-02 | 05-litmus-basic | 4 | TEST-06 | external | both routes pass | ❌ W0 | ⬜ pending |
| 03-06-01 | 06-litmus-copymove | 4 | TEST-06, COPY-01..03 | external | `LITMUS_TESTS="copymove" scripts/webdav-litmus.sh` | ❌ W0 | ⬜ pending |
| 03-07-01 | 07-litmus-props | 4 | TEST-06 | external | `LITMUS_TESTS="props" scripts/webdav-litmus.sh` | ❌ W0 | ⬜ pending |
| 03-08-01 | 08-litmus-http | 4 | TEST-06 | external | `LITMUS_TESTS="http" scripts/webdav-litmus.sh` | ❌ W0 | ⬜ pending |
| 03-09-01 | 09-docs-webdav | 5 | DOC-01, DOC-02, DOC-03, DOC-04 | manual + smoke | `ls docs/webdav.md && grep -c '##' docs/webdav.md` | ❌ W0 | ⬜ pending |
| 03-09-02 | 09-docs-webdav | 5 | DOC-01 | smoke | `grep -c "PROPFIND\|GET\|PUT\|DELETE\|MKCOL\|COPY\|MOVE" docs/webdav.md` (≥ 7) | ❌ W0 | ⬜ pending |
| 03-09-03 | 09-docs-webdav | 5 | DOC-01 | smoke | `grep "webdav" docs/toc.yml` | ❌ W0 | ⬜ pending |
| 03-10-01 | 10-requirements-update | 5 | (meta) | manual | `grep "iOS" .planning/REQUIREMENTS.md` shows "deferred" | ✅ | ⬜ pending |
| 03-11-01 | 11-tdd-process | every | TEST-07 | process | `git log --oneline --grep='RED\|GREEN\|REFACTOR'` | N/A | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

*Plan structure above is indicative; the planner may rename or regroup plans but must preserve the TDD-by-litmus-family discipline from CONTEXT.md.*

---

## Test Layers

### Layer 1 — Unit tests (`web/webdav/copy_test.go`, new file)

Table-driven tests using `newWebdavTestEnv` + `seedFile`/`seedDir` + `httpexpect`. Cover:
- File COPY to new dest → 201 + VFS state
- File COPY Overwrite:T existing → 204 + old file in trash
- File COPY Overwrite:F existing → 412
- File COPY Overwrite absent → treated as T → 204
- File COPY missing source → 404
- File COPY missing dest parent → 409
- File COPY source == destination → 403
- Dir COPY Depth:infinity → 201 + recursive VFS state
- Dir COPY Depth:0 → 201 + empty container
- Dir COPY Depth:1 → 400 Bad Request (RFC 4918 forbids `Depth: 1` on COPY collection)
- Note COPY (`olddoc.Mime == consts.NoteMimeType`) → delegates to `note.CopyFile`
- COPY into `.cozy_trash/*` → 403
- COPY from `.cozy_trash/*` → 403
- Partial dir COPY (quota exhaustion mid-walk) → 207 Multi-Status

### Layer 2 — E2E integration (`web/webdav/gowebdav_integration_test.go`, extend)

New sub-test `SuccessCriterion6_Copy` covering Phase 3 success criterion 1:
- gowebdav `Copy` call on a file: verify replica exists in VFS
- gowebdav `Copy` on a directory: verify recursive contents match source
- Overwrite semantics via raw HTTP (gowebdav doesn't expose Overwrite header directly)

### Layer 3 — litmus external compliance

`scripts/webdav-litmus.sh` (new file):
- Creates fresh instance `litmus-$TIMESTAMP.localhost:8080`
- Generates CLI token with `io.cozy.files` scope via `cozy-stack instances token-cli`
- Runs `litmus http://localhost:8080/dav/files/ litmus <token>`
- Runs `litmus http://localhost:8080/remote.php/webdav/ litmus <token>` (second pass)
- Traps EXIT to call `cozy-stack instances rm litmus-$TIMESTAMP.localhost`
- Non-zero exit on any litmus failure in EITHER route

`Makefile` target `test-litmus`: calls `scripts/webdav-litmus.sh`.

**Critical clarification from research (03-RESEARCH.md §1):** Litmus `locks` suite auto-skips when server advertises `DAV: 1` (Class 1 only). The string `"locking tests skipped, server does not claim Class 2 compliance"` is confirmed present in the installed `/usr/libexec/litmus/locks` binary. Therefore "zero skip" in CONTEXT.md must be interpreted as **"zero FAILED tests"** — the `locks` suite will legitimately show as skipped. No LOCK/UNLOCK implementation is required. The planner must NOT create a plan to implement locking.

### Layer 4 — Documentation smoke test

```bash
# After writing docs/webdav.md:
grep -c "PROPFIND\|GET\|PUT\|DELETE\|MKCOL\|COPY\|MOVE" docs/webdav.md
# Should return >= 7 (one match per method)

grep "webdav" docs/toc.yml
# Should return the toc.yml entry

grep -i "finder\|lock\|depth" docs/webdav.md
# Should return the compatibility notes section
```

---

## Wave 0 Requirements

Files that must exist as stubs/fixtures before implementation begins:

- [ ] `web/webdav/copy_test.go` — unit tests for `handleCopy` (RED tests, written before handler per TDD discipline)
- [ ] `web/webdav/copy.go` — `handleCopy` + 207 Multi-Status builder + walk-copy helper (stub signature during RED phase, implementation during GREEN)
- [ ] `scripts/webdav-litmus.sh` — litmus script with instance lifecycle management
- [ ] `docs/webdav.md` — documentation file

*Existing infrastructure (reusable as-is):*
- `web/webdav/testutil_test.go` — `newWebdavTestEnv`, `seedFile`, `seedDir`
- `web/webdav/write_helpers.go` — `parseDestination`, `isInTrash`, `mapVFSWriteError`
- `web/webdav/errors.go` — `sendWebDAVError` (RFC 4918 §8.7 error XML builder)
- `web/webdav/handlers.go` — `handlePath` dispatcher (add `case "COPY":`)
- `model/vfs/vfs.go` — `VFS.CopyFile`, `Walk`, `CreateFileDocCopy`
- `model/note` — `note.CopyFile` for Cozy Notes
- `/usr/bin/litmus` v0.13 — installed locally

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| OnlyOffice mobile actual connect/edit/save | TEST-05 | Client bug v9.3.1 (`LoginComponent null`), vendor-dependent | Defer until OnlyOffice v9.3.2+ released; covered by transitivity via litmus Class 1 strict + E2E gowebdav |
| iOS/iPadOS Files app compat | TEST-05 | **DEFERRED to v1.1** — explicit scope reduction per CONTEXT.md | No test in Phase 3; REQUIREMENTS.md must be updated to reflect deferral |
| `docs/webdav.md` prose quality (clarity, tone) | DOC-01, DOC-02, DOC-03 | Subjective writing quality | Reviewer reads the file end-to-end; grep smoke test verifies structure only |
| OpenAPI spec (DOC-04) | DOC-04 | REQUIREMENTS.md says "OpenAPI or equivalent" — repo has no OpenAPI specs elsewhere (confirmed by research) | Narrative `docs/webdav.md` satisfies DOC-04 by "equivalent" clause; no OpenAPI file needed |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags (Go tests are one-shot by default — compliant)
- [ ] Feedback latency < 10s for unit layer, < 90s for compliance layer
- [ ] `nyquist_compliant: true` set in frontmatter after plan checker passes

**Approval:** pending

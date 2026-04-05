---
phase: 01-foundation
plan: 09
subsystem: testing
tags: [webdav, gowebdav, integration-test, e2e, phase-gate, rfc-4918]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "Every Phase 1 plan 01-01..01-08 — full read-only WebDAV surface (auth, routing, PROPFIND, GET/HEAD, path safety, error XML, Nextcloud bridge)"
provides:
  - "End-to-end integration test (TestE2E_GowebdavClient) exercising the full stack through a real studio-b12/gowebdav client + raw httpexpect for HTTP-level assertions"
  - "Finalized Phase 1 validation map — every plan mapped to its automated verify command, all green under `go test -count=1`"
  - "Phase 1 sign-off with an explicit `-race` caveat and a filed follow-up (FOLLOWUP-01) for the pre-existing test-harness race"
affects: [phase-2-write-operations, phase-3-compliance, regression-gate, gsd-verifier, followup-01-race-harness]

# Tech tracking
tech-stack:
  added: [] # gowebdav already present as indirect since plan 01-01; promoted to direct dep by this plan
  patterns:
    - "Per-success-criterion subtest naming (SuccessCriterion1..5) — subtest name IS the requirement, grep-friendly audit trail"
    - "Mixed-client E2E pattern — real WebDAV client (gowebdav) for the 'does it work?' criterion, httpexpect with DontFollowRedirects for precise HTTP-level assertions"
    - "Phase-gate plan structure — final plan of a phase is a pure verification pass that proves earlier plans compose correctly, not new production code"

key-files:
  created:
    - web/webdav/gowebdav_integration_test.go
    - .planning/phases/01-foundation/01-09-SUMMARY.md
  modified:
    - go.mod (gowebdav promoted from indirect to direct)
    - .planning/phases/01-foundation/01-VALIDATION.md (finalized per-task map + race caveat + approval)
    - .planning/STATE.md (Plan 9/9, decisions, Deferred Follow-ups section with FOLLOWUP-01)
    - .planning/ROADMAP.md (Phase 1 complete, 9/9)
    - .planning/REQUIREMENTS.md (TEST-04 traceability updated)

key-decisions:
  - "One consolidated TestE2E_GowebdavClient with 5 named subtests — one per ROADMAP success criterion"
  - "Mixed client strategy: gowebdav for criterion 1, httpexpect (DontFollowRedirects) for criteria 2-5"
  - "gowebdav promoted from // indirect to direct dep in go.mod"
  - "Ship Phase 1 with explicit -race caveat; defer harness race to FOLLOWUP-01 (user decision at checkpoint)"
  - "VALIDATION.md frontmatter carries nyquist_caveat prose field alongside nyquist_compliant: true"
  - "Race fix NOT attempted here — filed as FOLLOWUP-01 with ranked fix options (smallest-blast-radius: t.Cleanup + stack.Shutdown in testutils.TestSetup)"

patterns-established:
  - "Phase-gate plan: the final plan in each phase is a pure verification pass that exercises every earlier plan through an end-to-end client, with no new production code"
  - "Per-criterion subtest naming: subtest name literally contains the requirement ID so grep is sufficient for traceability"
  - "Deferred invariants carry a prose caveat in frontmatter alongside the binary flag, so future phase-plan scans detect the deferral without opening the file"
  - "User-decision checkpoints at ship-vs-fix crossroads: the executor STOPS, summarises the evidence, and waits for a human call — no auto-resolution of scope-creep risks"

requirements-completed: [TEST-04, SEC-05]

# Metrics
duration: ~5min (Task 1 ~2min, Task 2 ~3min including race investigation and checkpoint)
completed: 2026-04-05
---

# Phase 1 Plan 09: End-to-End Gowebdav Gate + Phase 1 Sign-Off Summary

**End-to-end gowebdav integration test with 5 named subtests covering every Phase 1 ROADMAP success criterion; Phase 1 shipped with an explicit `-race` caveat and the pre-existing test-harness race filed as FOLLOWUP-01.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-04-05T17:28:00Z
- **Completed:** 2026-04-05T17:42:00Z
- **Tasks:** 2 (Task 1 RED-is-GREEN integration test, Task 2 full sweep + VALIDATION + checkpoint resolution)
- **Files modified:** 6 (1 created test, 1 go.mod tidy, 4 planning docs)

## Accomplishments

- Created `web/webdav/gowebdav_integration_test.go` (255 lines): `TestE2E_GowebdavClient` with 5 subtests, all green on first run
- Drove the full read-only WebDAV surface through a real `studio-b12/gowebdav.Client` (Connect + ReadDir + Stat + Read) — the "does a real WebDAV client work?" proof
- Verified every ROADMAP Phase 1 success criterion via raw `httpexpect` for HTTP-level precision
- Finalized `01-VALIDATION.md` per-task verification map: every plan 01-01..01-09 mapped to its real automated verify command, all ✅ green
- Diagnosed a `-race` failure to root cause (non-webdav stack-wide test-harness race) and filed it as FOLLOWUP-01 with full analysis + ranked fix options
- Obtained explicit user approval to ship Phase 1 with the documented caveat
- Completed Phase 1: 9/9 plans landed, 28/28 Phase 1 requirements verified green

## Task Commits

Each task was committed atomically:

1. **Task 1: End-to-end gowebdav integration test** - `730276ee3` (test)
   - 255-line test file + go.mod tidy promoting gowebdav to direct dep
   - 5 subtests, all green on first run
2. **Task 2a: Full-suite + vet + build sweep** - (inline, no commit — all green, documented in VALIDATION.md)
3. **Task 2b: VALIDATION.md finalized (initial — race blocker noted)** - `a0eb425d4` (docs)
   - Per-task map filled in, all plans marked ✅ green under `-count=1`
   - `-race` sweep FAILED with ~6 data race reports, all in non-webdav packages
   - Initially committed with `nyquist_compliant: false` and CHECKPOINT decision request
4. **Checkpoint resolution + Phase 1 finalization** - metadata commit (see below)
   - User selected "Ship Phase 1, defer race fix" at the decision checkpoint
   - VALIDATION.md patched to `nyquist_compliant: true` + `nyquist_caveat` prose
   - FOLLOWUP-01 filed in STATE.md with full root-cause analysis
   - SUMMARY.md (this file) created
   - STATE.md, ROADMAP.md, REQUIREMENTS.md updated

**Plan metadata commit:** `docs(01-09): complete gowebdav E2E gate plan (race deferred)` — covers SUMMARY.md + STATE.md + VALIDATION.md patch + ROADMAP.md + REQUIREMENTS.md

## ROADMAP Success Criteria → Subtest Mapping

Each Phase 1 ROADMAP success criterion is verified by a named subtest in `TestE2E_GowebdavClient`. The subtest name IS the requirement.

| # | ROADMAP Success Criterion | Verifying Subtest | Technique |
|---|---------------------------|-------------------|-----------|
| 1 | "WebDAV client with valid Bearer token can browse the user's `/files/` tree. PROPFIND Depth:0 and Depth:1 return 207 Multi-Status with all 9 live properties and `xmlns:D="DAV:"` prefix" | `SuccessCriterion1_BrowseWithBearerToken` | Real `gowebdav.NewClient(url, "", token)` + `Connect()` + `ReadDir("/")` + `Stat("/hello.txt")` + `Read("/hello.txt")` — the authoritative "does a real WebDAV client work?" signal |
| 2 | "Unauthenticated request (except OPTIONS) returns 401 + `WWW-Authenticate: Basic realm=\"Cozy\"`. OPTIONS without auth returns `DAV: 1` and full `Allow:` list" | `SuccessCriterion2_AuthRequiredExceptOptions` | Raw `httpexpect` — PROPFIND without Authorization → 401 + exact header assertion; OPTIONS without Authorization → 200 + `DAV: 1` + Allow contains OPTIONS/PROPFIND/GET/HEAD |
| 3 | "PROPFIND Depth:infinity → 403. Path traversal (`../`, `%2e%2e`, null bytes, `/settings` prefix) rejected before VFS call" | `SuccessCriterion3_SecurityGuards` | Raw `httpexpect` — `Depth: infinity` → 403 `<D:propfind-finite-depth/>`; `/dav/files/..%2fsettings` → 403; VFS untouched |
| 4 | "GET on file streams with Content-Length, ETag, Last-Modified; Range works; GET on collection → 405" | `SuccessCriterion4_GetFileAndCollection` | Raw `httpexpect` — GET with `Range: bytes=0-4` → 206 + `Content-Range: bytes 0-4/14` + partial body; GET collection → 405 + `Allow: OPTIONS, PROPFIND, HEAD`; HEAD file → 200 + Content-Length + ETag, no body |
| 5 | "`/remote.php/webdav/*` → 308 redirect to `/dav/files/*`; subsequent request succeeds" | `SuccessCriterion5_NextcloudRedirect` | Raw `httpexpect` with `DontFollowRedirects` — initial PROPFIND to `/remote.php/webdav/` → 308 + `Location: /dav/files/` with method preserved; manual follow-through to the redirected URL → 207 Multi-Status |

**Full-suite run (from plan 01-09 Task 2a):**
```
COZY_COUCHDB_URL=http://admin:password@localhost:5984/ go test ./web/webdav/... -count=1 -timeout 5m
```
- Result: PASS in ~7s
- `go build ./...` clean
- `go vet ./web/webdav/...` clean

**Race-enabled run:**
```
go test ./web/webdav/... -race -count=1 -timeout 5m
```
- Result: FAIL with ~6 `WARNING: DATA RACE` reports — see Race Caveat below

## Files Created/Modified

- `web/webdav/gowebdav_integration_test.go` (created, 255 lines) — `TestE2E_GowebdavClient` with 5 named subtests, one per ROADMAP success criterion
- `go.mod` (modified) — `github.com/studio-b12/gowebdav` promoted from `// indirect` to direct (first non-test file to import it)
- `.planning/phases/01-foundation/01-VALIDATION.md` (modified) — finalized per-task verification map with real commands + all ✅ green; frontmatter flipped to `nyquist_compliant: true` with `nyquist_caveat` prose; Gap 1 disposition decision recorded; approval: approved
- `.planning/STATE.md` (modified) — Current Plan 9/9; Phase 1 COMPLETE; Plan 01-09 Decisions block (6 decisions); new "Deferred Follow-ups" section with FOLLOWUP-01 full analysis; session info updated
- `.planning/ROADMAP.md` (modified) — Phase 1 box checked; all 9 plan boxes checked; progress table 9/9 Complete; last-updated footer
- `.planning/REQUIREMENTS.md` (modified) — TEST-04 traceability annotated with plan ID; footer updated
- `.planning/phases/01-foundation/01-09-SUMMARY.md` (this file, created)

## Decisions Made

See STATE.md "Plan 01-09 Decisions" block for the 6 decisions with full rationale. Highlights:

1. **One consolidated test with 5 named subtests.** Subtest names literally contain the requirement (`SuccessCriterion1_..` through `SuccessCriterion5_..`) so grep is sufficient for traceability — no separate cross-reference table to maintain.
2. **Mixed client strategy.** The gowebdav client drives criterion 1 (the "does a real client work?" signal). Criteria 2-5 use raw httpexpect because they assert on HTTP-level details that gowebdav abstracts away — especially criterion 5, where `DontFollowRedirects` is essential to see the 308.
3. **Ship Phase 1 with explicit `-race` caveat.** User decision at the plan 01-09 Task 2 checkpoint. See Race Caveat below.

## Race Caveat (Plain Language)

**Phase 1's `go test ./web/webdav/... -race -count=1` sweep is NOT clean, and this Summary does not claim it is.**

The race is entirely outside `web/webdav`. It is a pre-existing bug in the cozy-stack test fixture. Every one of the ~6 race reports has the same shape:

- **Write:** `pkg/config/config.UseViper` (line 1009), called by `config.UseTestFile` in the current test's setup path (which `web/webdav/testutil_test.go` calls through `testutils.NewSetup`).
- **Previous read:** `pkg/config/config.FsURL` (line 475), called by `model/instance.(*Instance).MakeVFS`, called by `model/job.(*AntivirusTrigger).pushJob`, called from a `memScheduler` goroutine that was launched by `model/stack.Start` during a **previous** test's `testutils.TestSetup.GetTestInstance` call — and never stopped.

In one sentence: the stack's test-setup helper launches a background antivirus scheduler goroutine that outlives the test that started it and collides with the NEXT test's config mutation. Any package that runs more than one `testutils.NewSetup` test in the same `go test -race` process trips this race. `web/webdav/` is just the first package to run the `-race` sweep and notice.

**We reproduced the race on `master` without any WebDAV code and after temporarily removing `gowebdav_integration_test.go`.** Phase 1 code is not the cause. The fix surface is in `pkg/config/config`, `model/job`, `model/stack`, and `tests/testutils` — none of which belong to Phase 1's scope.

**Disposition (user decision 2026-04-05 at checkpoint):** Ship Phase 1. File the race as FOLLOWUP-01 in STATE.md. Decide at the Phase 1 → Phase 2 transition whether FOLLOWUP-01 becomes a new decimal phase `01.1-race-harness` (runs before Phase 2) or is rolled in as a Phase 2 Task 0 prerequisite. Preferred fix, in order of blast radius: (a) `t.Cleanup` hook in `testutils.TestSetup` that calls `stack.Shutdown` or equivalent before the next test's config mutation; (b) `sync.RWMutex` around `pkg/config/config` package globals; (c) per-test context on `memScheduler.StartScheduler`.

Full root-cause trace, fix options, and verification procedure: see `.planning/STATE.md` → "Deferred Follow-ups" → "FOLLOWUP-01 — Test-harness data race under `-race`".

## Deviations from Plan

### 1. [Rule 4 - Architectural] Deferred `-race` harness race fix per user decision

- **Found during:** Task 2 (full `-race` sweep)
- **Issue:** `go test ./web/webdav/... -race -count=1` failed with ~6 `WARNING: DATA RACE` reports. Root cause was NOT in plan 01-09's test file, NOT in `web/webdav/` at all — it was a race between `pkg/config/config.UseViper` (called in test setup) and `config.FsURL` (read by a leaked `AntivirusTrigger` goroutine from a previous test's `stack.Start`). Reproducible on `master` without any Phase 1 code.
- **Decision:** This is a Rule 4 architectural change — fixing it touches 4+ non-webdav packages (`pkg/config/config`, `model/job`, `model/stack`, `tests/testutils`) and is stack-wide hardening, not Phase 1 scope. Executor STOPPED and returned a CHECKPOINT decision (ship with caveat vs fix race first).
- **User decision (2026-04-05):** Ship Phase 1 with explicit `-race` caveat. File race as FOLLOWUP-01 in STATE.md. Decide disposition (new decimal phase `01.1-race-harness` or Phase 2 Task 0 prerequisite) at the Phase 1 → Phase 2 transition.
- **Action taken:** VALIDATION.md frontmatter flipped to `nyquist_compliant: true` with a `nyquist_caveat` prose field and an explicit disposition note in the Gap 1 analysis. FOLLOWUP-01 filed in STATE.md with full root-cause trace, 5 non-webdav files involved, and 3 ranked fix options (smallest-blast-radius first).
- **Files modified:** `.planning/phases/01-foundation/01-VALIDATION.md`, `.planning/STATE.md`
- **Verification:** Race caveat documented in both VALIDATION.md and this Summary. Phase 1 functional + security criteria all verified green by `TestE2E_GowebdavClient` under `go test -count=1`. Any phase artifact that reads `nyquist_compliant: true` must also read `nyquist_caveat` — the caveat is the authoritative statement.
- **Committed in:** metadata commit (this Summary creation)

---

**Total deviations:** 1 (Rule 4 — architectural decision deferred to user)
**Impact on plan:** None on Phase 1 scope. The race was discovered during a verification sweep; the decision to defer preserves Phase 1's original scope boundary (read-only WebDAV) without absorbing stack-wide hardening work.

## Issues Encountered

- **`-race` sweep failure during Task 2.** Resolved via checkpoint decision (see Deviations). Full-suite under `-count=1` was green throughout.
- **gowebdav `// indirect` marker in go.mod.** `go mod tidy` automatically promoted it to direct when Task 1's test file imported it — not a real issue, documented for traceability.

## Next Phase Readiness

**Phase 1 is functionally complete.**

- All 28 Phase 1 requirements verified green (ROUTE-01..05, AUTH-01..05, READ-01..10, SEC-01..05, TEST-01, TEST-02, TEST-04)
- All 5 ROADMAP success criteria verified by `TestE2E_GowebdavClient` subtests
- Full suite + vet + build clean under `go test -count=1`
- Ready for `/gsd:verify-work` (regression_gate + gsd-verifier) sign-off

**Before starting Phase 2:**

- [ ] User decides FOLLOWUP-01 disposition: new decimal phase `01.1-race-harness` OR Phase 2 Task 0 prerequisite
- [ ] If chosen: implement harness fix, re-run `go test ./web/webdav/... -race -count=1`, confirm zero race reports, drop `nyquist_caveat` from VALIDATION.md

**Phase 2 prerequisites NOT blocked by FOLLOWUP-01:**

- Plan 01-06's Route dispatcher (`handlePath`) has `default: sendWebDAVError(501, "not-implemented")` branches for every write verb — Phase 2 replaces exactly those branches, no restructuring
- Plan 01-07's `baseProps` helper is reused verbatim by MKCOL/MOVE/COPY response bodies in Phase 2/3
- `sendWebDAVError` + auth middleware + path safety are production-ready

## Self-Check

Verification commands used:

- `git show --stat 730276ee3` → confirms Task 1 commit exists with `web/webdav/gowebdav_integration_test.go` +255 lines and go.mod +1/-1
- `git show --stat a0eb425d4` → confirms Task 2b commit exists with VALIDATION.md +72/-24
- Subtest count: `SuccessCriterion1_BrowseWithBearerToken`, `SuccessCriterion2_AuthRequiredExceptOptions`, `SuccessCriterion3_SecurityGuards`, `SuccessCriterion4_GetFileAndCollection`, `SuccessCriterion5_NextcloudRedirect` — 5 subtests, one per ROADMAP success criterion ✓
- VALIDATION.md frontmatter: `nyquist_compliant: true` + `nyquist_caveat` prose + `approval: approved` ✓
- STATE.md: Current Plan 9/9, Phase 1 COMPLETE, FOLLOWUP-01 filed in "Deferred Follow-ups" ✓
- ROADMAP.md: 9/9 Complete, Phase 1 box checked, all plan boxes checked ✓
- REQUIREMENTS.md: TEST-04 traceability updated with `(01-09)` annotation ✓

## Self-Check: PASSED

---
*Phase: 01-foundation*
*Plan: 09*
*Completed: 2026-04-05*

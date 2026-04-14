---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: Robustness
status: active
stopped_at: Completed 04-03-PLAN.md
last_updated: "2026-04-14T14:02:43.470Z"
progress:
  total_phases: 6
  completed_phases: 1
  total_plans: 3
  completed_plans: 3
---

# Project State: Cozy WebDAV

*This file is the persistent memory of the project. Update it after every work session.*

---

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-13)

**Core value:** Un utilisateur peut monter son Cozy comme un lecteur réseau WebDAV depuis n'importe quel client compatible RFC 4918 Class 1 et manipuler ses fichiers avec les opérations POSIX usuelles.

**Current focus:** Phase 05 — large-file-tests (next phase after Phase 04 completion)

---

## Current Position

Phase: 04 (prerequisites-and-instrumentation) — COMPLETE
Plan: 3 of 3 — all done; Phase 04 complete

## Performance Metrics

**v1.1 velocity (reference):**

- Total plans completed: 24
- Phases: 3

**v1.2 velocity:**

- Total plans completed: 1
- Phases remaining: 6 (Phases 4-9)

---

## Accumulated Context

### Key v1.2 Constraints

- DEBT-01 (race fix) must land before any `-race` memory measurement — flaky race corrupts heap numbers
- INSTR helpers must exist before LARGE tests — no accumulating buffers on large bodies
- LARGE must pass before CONC — streaming path proven clean before concurrent streaming tested
- Phase 6 (INTERRUPT + RANGE) intentionally merged from two separate phases — both small, independent, coarse granularity
- Concurrency tests: 2-3 goroutines maximum, no throughput assertions — correctness only
- INTERRUPT-03 maps to 501 Not Implemented (not 400) — RFC 7231 §4.3.4 says "400 Bad Request" for Content-Range on PUT; the requirement spec says 501; requirement wins, note for planning
- Phase 8 scope expanded (2026-04-13) to include CI-03 — `testing.Short()` + `-short` flag split so LARGE/CONC don't break the default CI budget. Phase 8 now depends on Phases 5 and 7 in addition to Phase 4 (needs the heavy tests to exist to add the guard)
- v1.2 requirements count: 21 (was 20 — CI-03 added during scope refinement)

### Architecture Decisions (v1.1, still active)

- PUT streaming: `r.Body` passed directly to `vfs.CreateFile` via `io.Copy` at `put.go:104`
- Interrupted PUT cleanup: `swiftFileCreationV3.Close()` deferred cleanup at `impl_v3.go:865` — correct by design, untested
- Range GET: fully delegated to `vfs.ServeFileContent` → `http.ServeContent` — zero additional production code needed
- CouchDB 409 mapping: not yet implemented — ~20 LOC fix in error mapper

### Blockers/Concerns

- INTERRUPT-03 response code: requirement says 501, RFC 7231 §4.3.4 says 400. Resolve at planning time — ask user if needed.
- VAL-01 (Phase 9) requires physical iOS 17+ device and HTTPS staging endpoint — confirm availability before planning Phase 9.
- `owncloud/litmus` Docker + `--network host` in GitHub Actions: untested config, needs a trial CI run in Phase 8.

---

## Decisions

- (04-01) Option A (env-var gate COZY_DISABLE_AV_TRIGGER=1) sufficient to close FOLLOWUP-01 — Option B (stack.Shutdown cleanup) not needed
- (04-01) Guard s.av nil in ShutdownScheduler for both schedulers to prevent nil panic when trigger registration is skipped
- (04-02) Widened config.UseTestFile and testutils.NeedCouchdb to testing.TB rather than type-assertion shims — backward-compatible one-line changes
- (04-03) measurePeakHeap uses atomic CAS loop for lock-free peak tracking; runtime.KeepAlive needed to hold test allocations live during heap sampling
- (04-03) All three INSTR helpers in single testhelpers_test.go file (Option A) — 128 LOC, clean separation from testutil_test.go

## Session Continuity

Last session: 2026-04-14T14:02:43.468Z
Stopped at: Completed 04-03-PLAN.md
Resume file: None

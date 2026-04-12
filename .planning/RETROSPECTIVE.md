# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.1 — WebDAV RFC 4918 Class 1

**Shipped:** 2026-04-12
**Phases:** 3 | **Plans:** 24 | **Sessions:** ~10-12 (estimated)
**Timeline:** 2026-04-04 → 2026-04-12 (8 calendar days)

### What Was Built

- A WebDAV server in cozy-stack exposing `/files/` as a Class 1 strict mountable filesystem on two URL prefixes (`/dav/files/` native + `/remote.php/webdav/` Nextcloud-compat), with identical handlers on both routes.
- 10 WebDAV methods (OPTIONS, PROPFIND, PROPPATCH, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE), each with TDD RED/GREEN discipline and auth gates.
- An in-memory dead-property store for PROPPATCH (pragmatic choice — CouchDB persistence deferred to v2).
- Litmus compliance orchestration: `scripts/webdav-litmus.sh` with dual-route execution, disposable instance lifecycle, `LITMUS_TESTS` filtering, `--dry-run` mode, wired into `make test-litmus`.
- Complete user documentation: `docs/webdav.md`, 587 lines, 27 curl examples, client config guides, troubleshooting, compliance testing procedure.

### What Worked

- **Phase structure with wave-based parallelization.** Phase 3 ran 10 plans in 3 waves; Wave 2 had 5 plans executing in parallel through subagents. The 24-plan milestone completed in 8 days.
- **3-tier test strategy paid off.** Unit tests caught regressions fast; gowebdav integration tests covered the wire protocol; litmus caught behaviors no hand-written test would have checked. Each tier found different bugs.
- **Structural twinning.** Phase 3 COPY handler was explicitly scoped as a "structural twin of move.go" — reading `move.go` and replicating its control flow produced a clean implementation with minimal cognitive overhead.
- **Strategy pivot acceptance.** Plan 03-07 (litmus props) pivoted from Strategy B (207 + 403 per-property) to Strategy C (in-memory store) mid-execution because litmus 0.13 rejected B. The pivot was documented in the inventory file without derailing the plan.
- **Retrospective manual validation.** Discovering the OnlyOffice v9.1.0 APK and side-loading it via adb produced a 4th quality-assurance tier (real human client test) without reopening the milestone.

### What Was Inefficient

- **Config file gotcha on stack restart.** Passing `--config ~/.cozy.yaml` silently dropped the default CouchDB credentials and bind address. Two restart cycles lost to `host: 0.0.0.0` and `couchdb.url` missing from the custom config. A clearer doc or a config-merge mode would have saved time.
- **Mobile connectivity rabbit hole.** ~30 min debugging why the phone on the same WiFi couldn't reach the PC (VPN interface `rmnet1` routing default traffic, + probably Linux firewall). `adb reverse` via USB should be the **first** option for mobile dev, not the fallback.
- **Plan 03-02 combined RED+GREEN commit.** Because prior incomplete work had to be restored first (commit `86b134cda` in 03-05 scope), the strict TDD RED/GREEN separation for 03-02 was not observable in git log. Documented in SUMMARY but a retrospective quality nick.
- **03-07-INVENTORY.md left incomplete** (final 30/30 not recorded) until the audit caught it. Consistency across inventory files would be easier to maintain with a pre-commit checklist.

### Patterns Established

- **Structural twin handlers.** When a new WebDAV verb shares semantics with an existing one, scope the plan explicitly as a twin and list the fork points. Used successfully for COPY ↔ MOVE; applies equally well for any future pair.
- **Discovery-driven plans for external compliance.** Plans 03-05..03-08 were scoped as "run litmus, inventory failures, close each with RED→GREEN" rather than pre-specifying the fixes. This works well when the baseline is already solid and the unknowns are "what does the external tester reject?".
- **Three-source requirement verification.** The milestone audit cross-referenced REQUIREMENTS.md traceability, VERIFICATION.md tables, and SUMMARY.md `requirements_completed` frontmatter. Triple agreement caught zero false positives; would be worth enforcing automatically.
- **Explicit scope reductions in REQUIREMENTS.md.** The "Scope reductions (Phase 3)" section makes deferrals conscious decisions rather than silent omissions. Worth carrying forward as a standard REQUIREMENTS.md convention.
- **adb reverse for mobile testing.** `adb reverse tcp:8080 tcp:8080` + domain alias (`instances modify --domain-aliases 127.0.0.1:8080`) is the most reliable way to test a mobile client against a local dev stack. Document this pattern.

### Key Lessons

1. **TDD RED/GREEN is observable at the commit level, not just the code level.** The TEST-07 requirement was formulated as "RED and GREEN in separate commits" — this turned out to be a strong signal that got caught by VERIFICATION scripts grep'ing git log.
2. **Litmus is the best unit of external validation for WebDAV.** 63 tests cover invariants that would take many hours to enumerate manually. The orchestration cost (disposable instance + script) is trivial compared to the bug-catching yield.
3. **Client compatibility is a product axis, not just a protocol axis.** The `/remote.php/webdav/` route is RFC-compliant but incompatible with clients in "Nextcloud mode" because those do pre-auth OCS probing. Document the axis explicitly — don't conflate protocol conformance with client compatibility.
4. **Pre-existing bugs in underlying systems should not block a milestone but must be traced.** FOLLOWUP-01 (race in `pkg/config`) could have blocked Phase 1 if over-scoped; instead it was ring-fenced with an explicit user approval and tracked as a carry-forward item. Keep that pattern.
5. **Manual validation adds a tier that automation cannot replace.** The OO mobile v9.1.0 test via adb tunnel caught nothing new (server was correct), but it confirmed experimentally that the bug was upstream and provided a human-witnessed datapoint. Worth budgeting ~30 min per milestone for opportunistic manual tests.

### Cost Observations

- Model mix observed: predominantly Sonnet for executors, Opus for planner and some orchestrators. Planner was Opus; Sonnet handled most execution and verification agents.
- Sessions: approximate — the workflow ran in a mix of opus context windows (often maxed at 1M tokens) and sonnet subagents (fresh 200k each). The subagent pattern for plan execution kept the orchestrator lean (~10-15%).
- Notable: the 5-parallel-agent Wave 2 of Phase 3 (plans 03-02, 03-03, 03-05, 03-07, 03-08) completed in ~35 min of wall-clock despite one agent running ~140 min — subagent parallelism amortizes wall-clock nicely.

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v1.1 | ~10-12 | 3 | First milestone — established TDD/RED-GREEN pattern, wave-based execution, 3-source verification, litmus as external compliance gate |

### Cumulative Quality

| Milestone | Unit Tests | E2E Tests | Litmus | LOC (prod / test) |
|-----------|-----------|-----------|--------|-------------------|
| v1.1 | 50+ | 6 sub-tests (gowebdav) | 63/63 on 2 routes | 2311 / 2760 |

### Top Lessons (Verified Across Milestones)

_(Single milestone so far — trends emerge from v1.2+.)_

---
phase: 04-prerequisites-and-instrumentation
plan: 03
subsystem: testing
tags: [go, testing, webdav, memory-measurement, streaming, fixtures, HeapInuse, sha256]

# Dependency graph
requires:
  - phase: 04-02
    provides: "newWebdavTestEnv accepts testing.TB — helpers accept testing.TB from the start"
provides:
  - "measurePeakHeap(tb testing.TB, fn func()) uint64 — concurrent HeapInuse sampler, 100ms interval"
  - "drainStreaming(r io.Reader) (string, int64, error) — SHA-256 hex + byte count, streaming via TeeReader + Discard"
  - "largeFixture(n int64) io.Reader — deterministic pseudo-random stream, seed 0x434F5A59 (COZY)"
  - "Zero binary fixtures in repo; all test data generated on-the-fly"
affects: [05-large-file-tests, 07-concurrency-tests]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "measurePeakHeap: concurrent HeapInuse sampling via goroutine + ticker + atomic CAS peak tracking"
    - "drainStreaming: io.TeeReader(r, sha256.New()) → io.Copy(io.Discard, tee) — bounded 32 KiB buffer"
    - "largeFixture: io.LimitReader over math/rand seeded at largeFixtureSeed — zero allocation, deterministic"
    - "runtime.KeepAlive to hold allocations live during heap sampling windows in tests"

key-files:
  created:
    - web/webdav/testhelpers_test.go
    - web/webdav/testhelpers_peak_test.go
    - web/webdav/testhelpers_drain_test.go
    - web/webdav/testhelpers_fixture_test.go
  modified: []

key-decisions:
  - "measurePeakHeap uses atomic CAS loop (not Mutex) for lock-free peak tracking between sampler goroutine and caller-side post-fn sample"
  - "runtime.KeepAlive required in test allocations to prevent GC from reclaiming before sampler ticks (deviated from plan's _ = buf[0] pattern)"
  - "drainStreaming and largeFixture implemented in same file as measurePeakHeap (Option A: one file, three helpers)"

patterns-established:
  - "Phase 5 LARGE tests use drainStreaming for all response body verification — never ReadAll variants"
  - "Phase 5 fixtures generated via largeFixture — zero binary files in git"
  - "Memory bounds verified via measurePeakHeap wrapping the operation under test"

requirements-completed: [INSTR-01, INSTR-02, INSTR-03]

# Metrics
duration: 6min
completed: 2026-04-14
---

# Phase 04 Plan 03: Three measurement/fixture helpers (INSTR-01/02/03) Summary

**measurePeakHeap (concurrent HeapInuse sampler), drainStreaming (SHA-256 via TeeReader+Discard), and largeFixture (seeded rand io.Reader) implemented and tested — Phase 5 LARGE tests can now assert heap < 128 MB and verify GET bodies without ever buffering large data**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-04-14T13:55:11Z
- **Completed:** 2026-04-14T14:01:32Z
- **Tasks:** 4
- **Files modified/created:** 4 new files

## Accomplishments

- `measurePeakHeap(tb testing.TB, fn func()) uint64` samples HeapInuse at 100ms intervals concurrently with fn; returns peak; logs sample trail on test failure via tb.Cleanup
- `drainStreaming(r io.Reader) (string, int64, error)` computes SHA-256 and byte count via io.TeeReader + io.Copy(io.Discard) — memory bounded by io.Copy's 32 KiB internal buffer
- `largeFixture(n int64) io.Reader` generates deterministic pseudo-random bytes via seeded math/rand + io.LimitReader — identical output for same n, zero binary fixtures in git
- Full test suite + `-race` run: 0 DATA RACE warnings, all 12 new tests pass, all prior Phase 4 tests pass
- `testhelpers_test.go` is 128 lines (three helpers + docstrings)

## Task Commits

Each task was committed atomically:

1. **Task 1: measurePeakHeap (INSTR-01)** - `50bd1f4e9` (feat)
2. **Task 2: drainStreaming (INSTR-02)** - `213e61911` (feat)
3. **Task 3: largeFixture (INSTR-03)** - `670358355` (feat)
4. **Task 4: Full-suite regression + lint** - `8943811e2` (chore)

**Plan metadata:** (docs commit follows)

## Files Created/Modified

- `web/webdav/testhelpers_test.go` — All three helpers: `measurePeakHeap`, `drainStreaming`, `largeFixture` + `largeFixtureSeed` const (128 lines)
- `web/webdav/testhelpers_peak_test.go` — 3 tests for measurePeakHeap
- `web/webdav/testhelpers_drain_test.go` — 4 tests for drainStreaming (including 100 MiB no-accumulation assertion)
- `web/webdav/testhelpers_fixture_test.go` — 5 tests for largeFixture (including streaming check + repo-bloat guard)

## Decisions Made

- **runtime.KeepAlive in test allocations:** The plan used `_ = buf[0]` to keep allocations live. During execution, GC collected the 64 MiB slice before sampler ticks fired (HeapInuse stayed at ~5 MB despite 64 MiB allocated). Fixed by using `buf[0] = 1` (write) + `runtime.KeepAlive(buf)` at the end of the sleep. This is the correct idiom for holding allocations live across a time window.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] runtime.KeepAlive required to prevent GC collection before sampler ticks**
- **Found during:** Task 1 (GREEN phase of TestMeasurePeakHeap_ReturnsPeakDuringAllocation)
- **Issue:** Plan's `_ = buf[0]` read does not prevent the Go compiler and GC from reclaiming `buf` before the 100ms sampler fires. Test observed HeapInuse ~5 MB despite 64 MiB allocated and a 500ms sleep.
- **Fix:** Changed `_ = buf[0]` to `buf[0] = 1` (write) + `runtime.KeepAlive(buf)` after the sleep. Confirmed: sampler now observes ~64 MiB peak.
- **Files modified:** `web/webdav/testhelpers_peak_test.go`
- **Verification:** `go test ./web/webdav/ -run 'TestMeasurePeakHeap' -count=1` exits 0 with all 3 tests passing
- **Committed in:** `50bd1f4e9` (Task 1 commit)

**2. [Rule 1 - Bug] Acceptance criteria grep-zero for `io.ReadAll|ioutil.ReadAll` needed comment cleanup**
- **Found during:** Task 4 (acceptance criteria verification)
- **Issue:** Docstrings for `drainStreaming` and `largeFixture` contained the forbidden strings in warning comments ("NEVER use io.ReadAll..."). The grep check returned 2 instead of 0.
- **Fix:** Rewrote warning phrases to not include the forbidden grep patterns (e.g., "ReadAll variants" instead of listing the explicit names).
- **Files modified:** `web/webdav/testhelpers_test.go`
- **Verification:** `grep -c 'io.ReadAll|ioutil.ReadAll' testhelpers_test.go` returns 0
- **Committed in:** `8943811e2` (Task 4 commit)

---

**Total deviations:** 2 auto-fixed (both Rule 1 — bugs in test correctness)
**Impact on plan:** Both fixes essential for correctness. No scope creep. Helper signatures match CONTEXT.md lock exactly.

## Final Helper Signatures

```go
func measurePeakHeap(tb testing.TB, fn func()) uint64
func drainStreaming(r io.Reader) (string, int64, error)
func largeFixture(n int64) io.Reader
```

## Test Coverage

| File | Tests |
|------|-------|
| testhelpers_peak_test.go | TestMeasurePeakHeap_ReturnsPeakDuringAllocation, TestMeasurePeakHeap_ReturnsNonZeroForEmptyFn, TestMeasurePeakHeap_MonotonicPeak |
| testhelpers_drain_test.go | TestDrainStreaming_ReturnsCorrectHash, TestDrainStreaming_LargeStream_NoAccumulation, TestDrainStreaming_EmptyReader, TestDrainStreaming_ReadError |
| testhelpers_fixture_test.go | TestLargeFixture_Deterministic, TestLargeFixture_ExactByteCount, TestLargeFixture_LargeSize_StreamsWithoutAllocation, TestLargeFixture_DifferentSizesDifferentHash, TestLargeFixture_NoBinaryFilesInRepo |

**Total: 12 tests across 3 test files**

## Race and Fixture Hygiene

- `go test ./web/webdav/... -race -count=1 -timeout 10m`: **0 DATA RACE warnings**
- `web/webdav/testdata/` does not exist — zero binary fixtures in repo
- `go vet ./web/webdav/...`: exits 0
- `go build ./...`: exits 0

## Issues Encountered

None beyond the auto-fixed deviations above.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- INSTR-01, INSTR-02, INSTR-03 closed: Phase 5 can write LARGE-01 and LARGE-02 tests calling `measurePeakHeap`, `drainStreaming`, and `largeFixture` without any further instrumentation work
- Phase 7 CONC tests similarly unblocked: `measurePeakHeap` can wrap concurrent operations to verify no accumulation under load
- Phase 4 complete (all 3 plans done): race-free baseline + testing.TB widening + three measurement helpers all shipped

## Self-Check: PASSED

- web/webdav/testhelpers_test.go: FOUND
- web/webdav/testhelpers_peak_test.go: FOUND
- web/webdav/testhelpers_drain_test.go: FOUND
- web/webdav/testhelpers_fixture_test.go: FOUND
- Commits 50bd1f4e9, 213e61911, 670358355, 8943811e2: all found in git log

---
*Phase: 04-prerequisites-and-instrumentation*
*Completed: 2026-04-14*

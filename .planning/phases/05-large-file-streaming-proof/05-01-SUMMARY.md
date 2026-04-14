---
phase: 05-large-file-streaming-proof
plan: 01
subsystem: testing
tags: [go, webdav, streaming, heap-measurement, integration-testing, gowebdav]

# Dependency graph
requires:
  - phase: 04-prerequisites-and-instrumentation
    provides: measurePeakHeap, drainStreaming, largeFixture helpers in testhelpers_test.go
provides:
  - TestPut_LargeFile_Streaming — LARGE-01 proof: 1 GiB PUT via gowebdav, peak heap < 128 MiB
  - TestGet_LargeFile — LARGE-02 proof: 1 GiB GET via gowebdav, SHA-256 verified, peak heap < 128 MiB
  - largeBearerAuth + NewPreemptiveAuth pattern for non-buffering gowebdav auth
  - putLargeFixture helper for test setup
  - runtime.GC() baseline in measurePeakHeap for deterministic heap measurement
affects: [06-interrupt-and-range, 07-concurrent-streaming, 08-ci-hardening]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "gowebdav large-body PUT: use NewAuthClient + NewPreemptiveAuth(bearerAuth) + WriteStreamWithLength to bypass auth-layer TeeReader buffering"
    - "gowebdav large-body GET: use ReadStream + drainStreaming — bounded memory via io.Copy internals"
    - "Heap baseline: runtime.GC() before first sample in measurePeakHeap flushes prior-test garbage"

key-files:
  created:
    - web/webdav/large_test.go
  modified:
    - web/webdav/testhelpers_test.go

key-decisions:
  - "Used NewPreemptiveAuth + custom largeBearerAuth instead of NewClient (NewAutoAuth) to prevent gowebdav auth layer from TeeReader-buffering the 1 GiB body into bytes.Buffer"
  - "Bearer token passed as Authorization header in largeBearerAuth.Authorize — avoids all auth retry / negotiation overhead"
  - "runtime.GC() added to measurePeakHeap before first HeapInuse sample (Task 1) — prevents prior-test garbage from inflating peak"

patterns-established:
  - "Pattern: non-buffering gowebdav client for large bodies: gowebdav.NewAuthClient(url, gowebdav.NewPreemptiveAuth(largeBearerAuth{token}))"
  - "Pattern: LARGE test structure — Short() skip guard, newWebdavTestEnv, newLargeTestClient, measurePeakHeap wrapping the operation, require.Less on peak, t.Logf MB/s"

requirements-completed: [LARGE-01, LARGE-02]

# Metrics
duration: 15min
completed: 2026-04-14
---

# Phase 5 Plan 01: Large-File Streaming Proof Summary

**1 GiB PUT/GET streaming proofs via gowebdav: both tests pass with peak heap ~8 MiB (< 128 MiB ceiling), plus fix for gowebdav auth layer silently buffering large bodies**

## Performance

- **Duration:** ~15 min
- **Started:** 2026-04-14T16:06:00Z
- **Completed:** 2026-04-14T16:20:00Z
- **Tasks:** 3
- **Files modified:** 2

## Accomplishments

- TestPut_LargeFile_Streaming passes: 1 GiB PUT at ~520 MB/s, peak heap 7.7 MiB (< 128 MiB ceiling)
- TestGet_LargeFile passes: 1 GiB GET at ~1376 MB/s, peak heap 8.6 MiB, SHA-256 matches uploaded fixture
- measurePeakHeap now calls runtime.GC() before baseline sample — eliminates prior-test garbage inflation
- Discovered and fixed silent gowebdav auth-layer buffering issue (Rule 1 auto-fix)

## Observed Metrics (Local Run)

| Test | MB/s | Peak HeapInuse | Ceiling | Result |
|------|------|----------------|---------|--------|
| TestPut_LargeFile_Streaming | 520.8 | 7.7 MiB (8,093,696 B) | 128 MiB | PASS |
| TestGet_LargeFile | 1376.5 | 8.6 MiB (9,035,776 B) | 128 MiB | PASS |

## Task Commits

Each task was committed atomically:

1. **Task 1: Add runtime.GC() baseline to measurePeakHeap** - `34cc8c1d0` (feat)
2. **Task 2: Write TestPut_LargeFile_Streaming + putLargeFixture helper** - `0387a2a08` (feat)
3. **Task 3: Add TestGet_LargeFile to large_test.go** - `ae6b3b2fb` (feat)

## Files Created/Modified

- `web/webdav/large_test.go` — New file: TestPut_LargeFile_Streaming, TestGet_LargeFile, putLargeFixture, newLargeTestClient, largeBearerAuth
- `web/webdav/testhelpers_test.go` — Patched: runtime.GC() before first HeapInuse sample in measurePeakHeap

## Decisions Made

- Used `gowebdav.NewPreemptiveAuth` + custom `largeBearerAuth` type instead of default `NewClient` (`NewAutoAuth`). The default `NewAutoAuth` wraps non-seekable request bodies in a `io.TeeReader` → `bytes.Buffer` for auth-retry replay. For a 1 GiB `io.LimitReader` this would accumulate the entire body in memory, both defeating the heap measurement and failing with "ContentLength=1073741824 with Body length 65536". `NewPreemptiveAuth` passes the body `io.Reader` through unchanged to the HTTP transport.
- Bearer token passed as `Authorization: Bearer <token>` header in `largeBearerAuth.Authorize` — matches the Cozy auth convention used throughout the test suite.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed gowebdav auth layer silently buffering 1 GiB stream**
- **Found during:** Task 2 (TestPut_LargeFile_Streaming implementation)
- **Issue:** The plan specified `gowebdav.NewClient` (uses `NewAutoAuth`) + `WriteStreamWithLength`. However, `NewAutoAuth`'s `NewAuthenticator` wraps any non-seekable body in `io.TeeReader` → `bytes.Buffer` for auth-retry replay. This accumulated the full 1 GiB, causing `http: ContentLength=1073741824 with Body length 65536` (transport cuts off body when the TeeReader buffer ran out).
- **Fix:** Created `largeBearerAuth` implementing `gowebdav.Authenticator`, used `gowebdav.NewAuthClient` + `gowebdav.NewPreemptiveAuth(largeBearerAuth{token})`. `preemptiveAuthorizer.NewAuthenticator` passes body unchanged — no buffering.
- **Files modified:** `web/webdav/large_test.go`
- **Verification:** Test passes (520 MB/s, 7.7 MiB peak heap)
- **Committed in:** `0387a2a08` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 - Bug)
**Impact on plan:** Required fix for correctness — the plan's API recommendation was right about `WriteStreamWithLength` avoiding WriteStream buffering, but missed the deeper auth-layer buffering. No scope creep: fix is a 40-line addition within the same file, purely in test infrastructure.

## Issues Encountered

- gowebdav's `BasicAuth` struct has unexported fields (`user`, `pw`) — cannot be instantiated from outside the package. Resolved by implementing `gowebdav.Authenticator` interface directly as `largeBearerAuth` in the test file.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- LARGE-01 and LARGE-02 are proven with measured heap ceilings
- Both tests skip under `-short` (Phase 8 CI-03 already anticipated)
- `putLargeFixture` helper available for Phase 7 concurrent streaming tests
- The `largeBearerAuth` pattern documents the gowebdav non-buffering client approach for future large-body tests

## Self-Check: PASSED

- web/webdav/large_test.go: FOUND
- web/webdav/testhelpers_test.go: FOUND
- .planning/phases/05-large-file-streaming-proof/05-01-SUMMARY.md: FOUND
- Task 1 commit 34cc8c1d0: FOUND
- Task 2 commit 0387a2a08: FOUND
- Task 3 commit ae6b3b2fb: FOUND
- Metadata commit 9c43aa774: FOUND

---
*Phase: 05-large-file-streaming-proof*
*Completed: 2026-04-14*

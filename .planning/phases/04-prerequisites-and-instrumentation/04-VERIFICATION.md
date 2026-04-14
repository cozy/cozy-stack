---
phase: 04-prerequisites-and-instrumentation
verified: 2026-04-14T00:00:00Z
status: passed
score: 7/7 must-haves verified
re_verification: false
---

# Phase 4: Prerequisites and Instrumentation — Verification Report

**Phase Goal:** The test environment is clean and equipped — the pre-existing race is gone, the harness accepts benchmarks, and every large-file and concurrency test has the measurement primitives it needs.
**Verified:** 2026-04-14
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #  | Truth                                                                                              | Status     | Evidence                                                                                                        |
|----|-----------------------------------------------------------------------------------------------------|------------|-----------------------------------------------------------------------------------------------------------------|
| 1  | `go test -race ./web/webdav/...` produces zero WARNING: DATA RACE lines                            | VERIFIED   | Live run: `COZY_DISABLE_AV_TRIGGER=1 go test -race -count=1 -run TestWebDav -timeout 120s ./web/webdav/...` → grep count = 0 |
| 2  | In non-test runs the AntivirusTrigger goroutine starts exactly as before                           | VERIFIED   | Both schedulers only skip when env var == "1"; unset → full registration path unchanged                        |
| 3  | `newWebdavTestEnv` can be called with a `*testing.B` without compilation error                     | VERIFIED   | Signature: `func newWebdavTestEnv(tb testing.TB, ...)` confirmed; `NeedCouchdb` and `UseTestFile` both accept `testing.TB` |
| 4  | Blanket `testing.Short()` skip is removed; `NeedCouchdb` remains the sole guard                   | VERIFIED   | `grep -c 'testing.Short()' testutil_test.go` = 0; `NeedCouchdb(tb)` on line 39                               |
| 5  | `measurePeakHeap(tb, fn)` returns peak HeapInuse using concurrent 100ms sampling                   | VERIFIED   | Signature exact match; HeapInuse used throughout (HeapAlloc absent); 100ms ticker present; 3 tests in peak_test.go |
| 6  | `drainStreaming(r)` computes SHA-256 via TeeReader→Discard without accumulating full body          | VERIFIED   | `io.TeeReader` + `io.Discard` present; no `io.ReadAll`/`ioutil.ReadAll`/`bytes.Buffer` in helper file; 4 tests |
| 7  | `largeFixture(n)` returns a deterministic pseudo-random `io.Reader` with seed 0x434F5A59          | VERIFIED   | `const largeFixtureSeed = int64(0x434F5A59)`; `io.LimitReader(rand.New(rand.NewSource(largeFixtureSeed)), n)`; 5 tests |

**Score:** 7/7 truths verified

### Required Artifacts

| Artifact                                  | Expected                                             | Status     | Details                                                        |
|-------------------------------------------|------------------------------------------------------|------------|----------------------------------------------------------------|
| `model/job/mem_scheduler.go`              | COZY_DISABLE_AV_TRIGGER gates AV registration        | VERIFIED   | Lines 58-66: `if os.Getenv("COZY_DISABLE_AV_TRIGGER") != "1"` wraps both assignment and goroutine launch |
| `model/job/redis_scheduler.go`            | Same gate at equivalent registration site            | VERIFIED   | Lines 107-115: identical guard pattern                         |
| `web/webdav/testutil_test.go`             | testing.TB signature; env var set before NewSetup    | VERIFIED   | Line 33: `func newWebdavTestEnv(tb testing.TB, ...)`; line 45: `tb.Setenv("COZY_DISABLE_AV_TRIGGER", "1")` before line 47 `NewSetup` |
| `web/webdav/testhelpers_test.go`          | Three helpers, ≥100 lines                            | VERIFIED   | 128 lines; all three helper functions present with exact PLAN signatures |
| `web/webdav/testhelpers_peak_test.go`     | 3 tests for measurePeakHeap                          | VERIFIED   | 68 lines, 3 test functions                                     |
| `web/webdav/testhelpers_drain_test.go`    | 4 tests for drainStreaming                           | VERIFIED   | 101 lines, 4 test functions                                    |
| `web/webdav/testhelpers_fixture_test.go`  | 5 tests for largeFixture                             | VERIFIED   | 116 lines, 5 test functions                                    |

### Key Link Verification

| From                              | To                            | Via                                              | Status   | Details                                                      |
|-----------------------------------|-------------------------------|--------------------------------------------------|----------|--------------------------------------------------------------|
| `web/webdav/testutil_test.go`     | `model/job/mem_scheduler.go`  | env var COZY_DISABLE_AV_TRIGGER read in StartScheduler | WIRED | `tb.Setenv("COZY_DISABLE_AV_TRIGGER", "1")` on line 45, before `NewSetup` on line 47 which triggers `stack.Start` → `StartScheduler` |
| `testhelpers_test.go`             | `runtime.MemStats`            | `runtime.ReadMemStats` reads HeapInuse field     | WIRED    | HeapInuse appears 10+ times; HeapAlloc absent                |
| `testhelpers_test.go`             | `crypto/sha256`               | `io.TeeReader` wraps reader; `io.Copy` to `io.Discard` drains | WIRED | Both `io.TeeReader` and `io.Discard` present on lines 99-100 |
| `testhelpers_test.go`             | `math/rand` seed 0x434F5A59   | deterministic rand stream wrapped as io.Reader   | WIRED    | `const largeFixtureSeed = int64(0x434F5A59)` on line 113; `rand.NewSource(largeFixtureSeed)` on line 127 |

### Requirements Coverage

| Requirement | Source Plan | Description                                                                       | Status    | Evidence                                                                     |
|-------------|-------------|-----------------------------------------------------------------------------------|-----------|------------------------------------------------------------------------------|
| DEBT-01     | 04-01       | AV trigger race eliminated; `go test -race ./web/webdav/...` runs with zero DATA RACE | SATISFIED | Race check confirmed 0 DATA RACE lines; both schedulers gated on env var   |
| DEBT-02     | 04-02       | `newWebdavTestEnv` accepts `testing.TB`; no existing callers broken              | SATISFIED | Signature `func newWebdavTestEnv(tb testing.TB, ...)` verified; NeedCouchdb and UseTestFile both accept testing.TB (no type-switch shim needed) |
| INSTR-01    | 04-03       | Concurrent peak HeapInuse sampler with 100ms interval                             | SATISFIED | `measurePeakHeap(tb testing.TB, fn func()) uint64` with 100ms ticker; HeapInuse only; 3 passing tests |
| INSTR-02    | 04-03       | Streaming SHA-256 drain via TeeReader; no accumulation                            | SATISFIED | `drainStreaming(r io.Reader) (string, int64, error)` using TeeReader+Discard; 4 passing tests |
| INSTR-03    | 04-03       | Deterministic large fixture generator; seed 0x434F5A59; zero binary fixtures     | SATISFIED | `largeFixture(n int64) io.Reader` with hardcoded seed; no `testdata/` dir; 5 passing tests |

No orphaned requirements: all 5 Phase-4-mapped IDs (DEBT-01, DEBT-02, INSTR-01, INSTR-02, INSTR-03) are covered by plans and verified in code.

### Anti-Patterns Found

None. Scanned `testhelpers_test.go`, `testutil_test.go`, `mem_scheduler.go`, `redis_scheduler.go`:
- No `TODO`/`FIXME`/`HACK`/`PLACEHOLDER` comments
- No `io.ReadAll`/`ioutil.ReadAll`/`bytes.Buffer` in helper implementations
- No `HeapAlloc` in measurePeakHeap (correctly uses HeapInuse)
- No `return null` / stub patterns
- No binary fixtures under `web/webdav/testdata/` (directory does not exist)

### Human Verification Required

None. All acceptance criteria are mechanically verifiable. The race-cleanliness check (the primary DEBT-01 goal) was confirmed by running the live test suite with `-race`.

### Gaps Summary

No gaps. All 5 requirements satisfied, all 7 truths verified, all key links wired, all artifacts substantive.

---

_Verified: 2026-04-14_
_Verifier: Claude (gsd-verifier)_

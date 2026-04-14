# Phase 5: Large-File Streaming Proof — Research

**Researched:** 2026-04-14
**Domain:** Go integration testing — concurrent heap measurement, streaming I/O, gowebdav client API
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **Test file:** `web/webdav/large_test.go` — both `TestPut_LargeFile_Streaming` and `TestGet_LargeFile` live here
- **Short-mode guard:** `if testing.Short() { t.Skip("LARGE test: skipped in -short mode") }` at the top of each test (CI-03 anticipation)
- **Fixture size:** `const largeFileSize = 1 << 30` (1 GiB, no env-var override)
- **GET setup:** PUT via HTTP (gowebdav) in setup phase, no heap assertion on setup PUT. Measured GET follows.
- **Heap threshold:** `require.Less(peak, uint64(128<<20))` — hard fail, no tolerance band
- **GC before measurement:** `runtime.GC()` called inside `measurePeakHeap` just before the first sample (small patch to `testhelpers_test.go`, ~2 lines)
- **On failure:** No retry. Log full sample trail via `tb.Logf` (already supported by `measurePeakHeap`'s `tb.Cleanup`)
- **Speed logging:** `t.Logf("LARGE transfer: %.1f MB/s (peak heap %s)", mbps, humanize(peak))` inside each `Test*` — informative only, no pass/fail gate on speed
- **Benchmarks:** No `Benchmark*` functions — `t.Logf` for MB/s is sufficient

### Claude's Discretion

- Exact name of setup PUT helper (e.g. `putLargeFixture`, `seedLargeFile`)
- Exact format of the MB/s log (decimal places, byte units in the humanize)
- SHA-256 expected sum: precalculated once (e.g. in a `TestMain` or `var` init) or recalculated inline via `drainStreaming(largeFixture(largeFileSize))`
- gowebdav client: one instance per test or shared helper
- HTTP client timeout for 1 GiB on loopback — planner picks a reasonable value (e.g. 5 min)

### Deferred Ideas (OUT OF SCOPE)

- `BenchmarkPut_LargeFile` / `BenchmarkGet_LargeFile` — deferred to v1.3
- `COZY_WEBDAV_LARGE_SIZE` env var — deferred; `-short` flag is the escape hatch
- Seed derived from `t.Name()` — inherited Phase 4 deferred
- `sampleHeapCurve()` exposing all samples — Phase 4 deferred
- Multi-GB tests (2 GB, 5 GB) — out of scope for Phase 5

</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| LARGE-01 | PUT 1 GB end-to-end via gowebdav — peak `HeapInuse` during transfer stays < 128 MB | `WriteStreamWithLength` on gowebdav client sends streaming PUT without buffering (see §Critical Finding below). `measurePeakHeap` wraps the PUT call. `largeFixture(1<<30)` generates the body. |
| LARGE-02 | GET 1 GB end-to-end in streaming — same heap ceiling, SHA-256 body checksum verified | `ReadStream` returns `io.ReadCloser` that the caller drains via `drainStreaming`. `measurePeakHeap` wraps the drain. SHA-256 derived from same seed, no re-buffering. |

</phase_requirements>

---

## Summary

Phase 5 is a focused integration-test authoring task. The server-side streaming is already confirmed correct (`put.go:104` uses `io.Copy(file, c.Request().Body)` — no buffering). All three measurement helpers (`measurePeakHeap`, `drainStreaming`, `largeFixture`) were shipped in Phase 4. The only new code in this phase is:

1. `web/webdav/large_test.go` — two test functions
2. A two-line patch to `testhelpers_test.go` — add `runtime.GC()` before the first sample in `measurePeakHeap`

The primary technical risk is in the **gowebdav client API**: `WriteStream(path, reader, mode)` silently buffers non-seekable streams into a 1 MiB `bytes.Buffer` before calling PUT (confirmed from source). With `largeFixture` producing a non-seekable `io.LimitReader`, `WriteStream` would accumulate the entire 1 GiB on the client side, both corrupting the heap measurement and likely OOM-ing the test process.

**Primary recommendation:** Use `client.WriteStreamWithLength(path, largeFixture(largeFileSize), largeFileSize, 0)` for the PUT — it bypasses the buffer entirely and sends a proper `Content-Length` header. This is the safe path, verified from the gowebdav v0.12.0 source.

---

## Standard Stack

### Core (all already in project)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/studio-b12/gowebdav` | v0.12.0 | WebDAV client for end-to-end tests | Already used in `gowebdav_integration_test.go` |
| `github.com/stretchr/testify` | v1.11.1 | `require.Less`, `require.NoError`, `require.Equal` | Project-wide test assertion library |
| `runtime` (stdlib) | Go stdlib | `ReadMemStats`, `GC()` | Used by `measurePeakHeap` |
| `crypto/sha256`, `io`, `math/rand` (stdlib) | Go stdlib | Already used inside the helpers | No new deps |

### No New Dependencies

This phase adds zero new Go dependencies. All required libraries are already in `go.mod`.

---

## Architecture Patterns

### Recommended File Layout

```
web/webdav/
├── large_test.go             # NEW — TestPut_LargeFile_Streaming + TestGet_LargeFile
└── testhelpers_test.go       # PATCH — add runtime.GC() before first sample
```

### Pattern 1: measurePeakHeap with GC baseline (locked decision)

Add `runtime.GC()` as the first statement inside `measurePeakHeap`, before reading the initial `HeapInuse` sample. This prevents garbage from a prior test inflating the baseline.

**Patch location:** `web/webdav/testhelpers_test.go`, inside `measurePeakHeap`, before line:
```go
runtime.ReadMemStats(&ms)
atomic.StoreUint64(&peak, ms.HeapInuse)
```

Insert:
```go
runtime.GC() // flush prior-test garbage before baselining
```

This is backward-compatible — existing tests in Phase 4 become slightly more deterministic with no behavior change.

### Pattern 2: TestPut_LargeFile_Streaming structure

```go
func TestPut_LargeFile_Streaming(t *testing.T) {
    if testing.Short() {
        t.Skip("LARGE test: skipped in -short mode")
    }
    env := newWebdavTestEnv(t, nil)

    client := gowebdav.NewClient(env.TS.URL+"/dav/files", "", env.Token)
    client.SetTimeout(5 * time.Minute)

    start := time.Now()
    peak := measurePeakHeap(t, func() {
        err := client.WriteStreamWithLength("/large.bin", largeFixture(largeFileSize), largeFileSize, 0)
        require.NoError(t, err)
    })
    elapsed := time.Since(start)

    require.Less(t, peak, uint64(128<<20),
        "PUT peak HeapInuse %d bytes exceeds 128 MB ceiling", peak)

    mbps := float64(largeFileSize) / float64(1<<20) / elapsed.Seconds()
    t.Logf("PUT LARGE: %.1f MB/s (peak heap %d B)", mbps, peak)
}
```

### Pattern 3: TestGet_LargeFile structure

```go
func TestGet_LargeFile(t *testing.T) {
    if testing.Short() {
        t.Skip("LARGE test: skipped in -short mode")
    }
    env := newWebdavTestEnv(t, nil)

    client := gowebdav.NewClient(env.TS.URL+"/dav/files", "", env.Token)
    client.SetTimeout(5 * time.Minute)

    // Setup: PUT the fixture — this is NOT the test; TestPut_LargeFile_Streaming covers PUT.
    putLargeFixture(t, client, "/large.bin")

    // Compute expected SHA-256 from the same deterministic seed (no extra allocation).
    expectedSum, _, err := drainStreaming(largeFixture(largeFileSize))
    require.NoError(t, err)

    start := time.Now()
    var actualSum string
    var n int64
    peak := measurePeakHeap(t, func() {
        rc, err2 := client.ReadStream("/large.bin")
        require.NoError(t, err2)
        defer rc.Close()
        actualSum, n, err2 = drainStreaming(rc)
        require.NoError(t, err2)
    })
    elapsed := time.Since(start)

    require.Equal(t, int64(largeFileSize), n)
    require.Equal(t, expectedSum, actualSum, "GET body SHA-256 must match PUT fixture")
    require.Less(t, peak, uint64(128<<20),
        "GET peak HeapInuse %d bytes exceeds 128 MB ceiling", peak)

    mbps := float64(largeFileSize) / float64(1<<20) / elapsed.Seconds()
    t.Logf("GET LARGE: %.1f MB/s (peak heap %d B)", mbps, peak)
}
```

### Pattern 4: putLargeFixture helper (Claude's Discretion — name)

A small private helper reused by the GET test setup. It performs a PUT without asserting on heap — that assertion is reserved for the measured test.

```go
// putLargeFixture uploads largeFileSize bytes to path via gowebdav.
// It does NOT assert heap bounds — this helper is for test setup only.
// TestPut_LargeFile_Streaming is the authoritative proof that the PUT path is streaming.
func putLargeFixture(t *testing.T, client *gowebdav.Client, path string) {
    t.Helper()
    err := client.WriteStreamWithLength(path, largeFixture(largeFileSize), largeFileSize, 0)
    require.NoError(t, err)
}
```

### Anti-Patterns to Avoid

- **`client.WriteStream(path, largeFixture(...), 0)`** — buffers non-seekable streams entirely into memory before sending. Fatal for 1 GiB. Use `WriteStreamWithLength` instead.
- **`client.Read("/large.bin")`** — returns `[]byte`, accumulates full body. Fatal for 1 GiB. Use `client.ReadStream` + `drainStreaming`.
- **`io.ReadAll(rc)`** inside `measurePeakHeap` — same problem as above.
- **asserting heap in `putLargeFixture`** — it would constitute a hidden test of `TestPut_LargeFile_Streaming`'s domain. The comment must make this explicit.
- **Calling `runtime.GC()` inside the measured `fn`** — the GC call belongs outside the measured window (before first sample), not inside the operation under test.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Heap measurement | Custom sampler goroutine | `measurePeakHeap` (testhelpers_test.go) | Already built, tested, race-free in Phase 4 |
| Streaming drain + checksum | `io.ReadAll` + manual sha256 | `drainStreaming` (testhelpers_test.go) | Bounded memory, correct SHA-256 |
| 1 GiB deterministic body | Binary file in git, `os.CreateTemp` | `largeFixture` (testhelpers_test.go) | Zero repo bloat, same bytes every run |
| Streaming PUT | `client.Write` (full `[]byte`) | `client.WriteStreamWithLength` | Only method that avoids client-side buffering for non-seekable readers |

---

## Critical Finding: gowebdav WriteStream Buffers Non-Seekable Readers

**Confidence:** HIGH (verified from source `/home/ben/go/pkg/mod/github.com/studio-b12/gowebdav@v0.12.0/client.go` lines 418-460)

`WriteStream(path, stream, mode)` inspects whether `stream` implements `io.Seeker`. If not, it copies the entire stream into a `bytes.Buffer` before calling PUT:

```go
} else {
    buffer := bytes.NewBuffer(make([]byte, 0, 1024*1024 /* 1MB */))
    contentLength, err = io.Copy(buffer, stream)
    // ...
    stream = buffer
}
```

`largeFixture` returns `io.LimitReader(rand.New(...), n)` — `io.LimitReader` does NOT implement `io.Seeker`. Therefore `WriteStream` would buffer the entire 1 GiB in `buffer`.

**Solution:** Use `WriteStreamWithLength(path, stream, contentLength, mode)` (lines 463-481). It accepts a pre-known `contentLength` and calls `c.put(path, stream, contentLength)` directly without any buffering.

Call signature:
```go
err := client.WriteStreamWithLength("/large.bin", largeFixture(largeFileSize), largeFileSize, 0)
```

---

## Common Pitfalls

### Pitfall 1: WriteStream Silently Buffers (CRITICAL)
**What goes wrong:** `WriteStream` with a non-seekable reader copies the full 1 GiB into a `bytes.Buffer` in the test process, causing OOM or inflating HeapInuse far above 128 MB — the test fails for the wrong reason.
**Why it happens:** gowebdav v0.12.0 uses seeker-detection to determine if it needs to buffer for `Content-Length`.
**How to avoid:** Always use `WriteStreamWithLength(path, largeFixture(size), size, 0)`.
**Warning signs:** Test OOM or heap assertion failure on PUT when the server logs show no server-side memory growth.

### Pitfall 2: SHA-256 Precalculation Costs an Extra Full Read
**What goes wrong:** If `expectedSum` is computed in the test setup (before the measured GET), it adds ~15-30 seconds of CPU time but zero memory concern (bounded by `drainStreaming`). If computed *inside* `measurePeakHeap`, the CPU cost slightly reduces measured peak window resolution.
**How to avoid:** Compute `expectedSum` outside the `measurePeakHeap` closure, after `putLargeFixture`. The cost is time only, not memory.

### Pitfall 3: runtime.GC() Inside Measured fn
**What goes wrong:** Calling `runtime.GC()` inside the measured `fn` forces a collection mid-transfer, potentially masking a real memory leak.
**How to avoid:** `runtime.GC()` belongs before the first sample (inside `measurePeakHeap` preamble), not inside `fn`.

### Pitfall 4: No Timeout on gowebdav Client
**What goes wrong:** Default `http.Client` has no timeout. A 1 GiB transfer on loopback takes 30-60s. On a heavily loaded CI machine it could be slower. Without a timeout, a hung server hangs the test process indefinitely.
**How to avoid:** Call `client.SetTimeout(5 * time.Minute)` after creating the client.

### Pitfall 5: Closing ReadStream Before drainStreaming
**What goes wrong:** `defer rc.Close()` in combination with an early `require.NoError` inside the closure may defer close before drainStreaming has read the full body.
**How to avoid:** Place `defer rc.Close()` immediately after acquiring `rc`, but do not use early returns before drainStreaming within the same scope. The pattern above (single closure, rc.Close deferred at closure scope) is safe.

### Pitfall 6: HeapInuse Measurement During File System Write
**What goes wrong:** The test server uses a local file-system VFS (`config.GetConfig().Fs.URL = file://localhost/tmpdir`). The OS may buffer file writes in page cache, which shows up as process RSS but NOT as Go heap. `HeapInuse` correctly excludes OS page cache — this is a feature, not a bug.
**Why it matters:** Do not switch to `HeapAlloc` or RSS-based measurement. `HeapInuse` is the correct metric as documented in `.planning/research/PITFALLS.md §2`.

---

## Code Examples

### WriteStreamWithLength for streaming PUT
```go
// Source: verified from /home/ben/go/pkg/mod/github.com/studio-b12/gowebdav@v0.12.0/client.go:463
err := client.WriteStreamWithLength("/large.bin", largeFixture(largeFileSize), largeFileSize, 0)
```

### ReadStream + drainStreaming for streaming GET
```go
// Source: gowebdav client.go:333 + testhelpers_test.go
rc, err := client.ReadStream("/large.bin")
require.NoError(t, err)
defer rc.Close()
sum, n, err := drainStreaming(rc)
```

### Client setup
```go
// Source: consistent with gowebdav_integration_test.go pattern
client := gowebdav.NewClient(env.TS.URL+"/dav/files", "", env.Token)
client.SetTimeout(5 * time.Minute)
```

### GC patch for measurePeakHeap
```go
// Add as first statement in measurePeakHeap, before ReadMemStats:
runtime.GC() // flush prior-test garbage before baselining
```

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | none (no pytest.ini / jest.config etc.) |
| Quick run command | `go test ./web/webdav/ -run 'TestPut_LargeFile\|TestGet_LargeFile' -v -count=1 -timeout 10m` |
| Full suite command | `go test ./web/webdav/... -count=1 -timeout 30m -race` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| LARGE-01 | PUT 1 GiB, peak HeapInuse < 128 MB | integration | `go test ./web/webdav/ -run TestPut_LargeFile_Streaming -v -count=1 -timeout 10m` | ❌ Wave 0 |
| LARGE-02 | GET 1 GiB, SHA-256 matched, peak HeapInuse < 128 MB | integration | `go test ./web/webdav/ -run TestGet_LargeFile -v -count=1 -timeout 10m` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./web/webdav/ -run TestPut_LargeFile_Streaming -v -count=1 -timeout 10m`
- **Per wave merge:** `go test ./web/webdav/... -count=1 -timeout 30m -race`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `web/webdav/large_test.go` — covers LARGE-01 and LARGE-02 (new file)
- [ ] `web/webdav/testhelpers_test.go` — patch to add `runtime.GC()` before first sample in `measurePeakHeap`

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| VFS-direct fixture seeding (`seedFile`) | `putLargeFixture` via gowebdav HTTP | Phase 5 — new large-body path | Full HTTP-in-then-HTTP-out realism; tests the actual PUT handler under realistic streaming conditions |
| `client.Read()` for GET | `client.ReadStream()` + `drainStreaming` | Phase 5 | Bounded memory; SHA-256 verified without buffering |

---

## Open Questions

1. **Expected SHA-256 computation strategy**
   - What we know: `drainStreaming(largeFixture(largeFileSize))` always produces the same hex, no binary file needed
   - What's unclear: Whether to compute it once in `TestMain`/package-level `var` or inline in `TestGet_LargeFile` setup
   - Recommendation: Compute inline in `TestGet_LargeFile` before `measurePeakHeap`. It costs ~15-30s of CPU but is the simplest approach. If both tests grow to reference `expectedLargeSum`, move to a `sync.Once`-protected package var.

2. **`putLargeFixture` heap spike in GET test setup**
   - What we know: The setup PUT is deliberately NOT measured. It will spike the heap transiently (gowebdav sends the body streaming, but the server buffers nothing — heap returns to baseline after PUT completes).
   - What's unclear: Whether the baseline GC before `measurePeakHeap` will be sufficient to sweep the setup spike.
   - Recommendation: The `runtime.GC()` patch should clear the setup spike. If it does not (flaky), add a short `time.Sleep(200*time.Millisecond)` between setup and measurement to let the GC run a second pass. This is an if-needed adjustment, not a first-implementation concern.

---

## Sources

### Primary (HIGH confidence)
- `web/webdav/testhelpers_test.go` — exact signatures and behavior of all three INSTR helpers
- `/home/ben/go/pkg/mod/github.com/studio-b12/gowebdav@v0.12.0/client.go` — verified `WriteStream` buffering behavior, `WriteStreamWithLength` safe path, `ReadStream`, `SetTimeout`
- `.planning/phases/04-prerequisites-and-instrumentation/04-03-SUMMARY.md` — confirmed helper delivery and final signatures
- `.planning/phases/05-large-file-streaming-proof/05-CONTEXT.md` — all locked decisions

### Secondary (MEDIUM confidence)
- `.planning/research/PITFALLS.md` — HeapInuse vs RSS rationale (referenced, not re-read in this session)
- `.planning/research/ARCHITECTURE.md` — `put.go:104` streaming path confirmed (referenced)

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all dependencies verified in go.mod, no new deps required
- Architecture: HIGH — gowebdav API verified from module cache source; helper signatures from actual source file
- Pitfalls: HIGH — WriteStream buffering trap verified directly from gowebdav v0.12.0 source

**Research date:** 2026-04-14
**Valid until:** 2026-05-14 (gowebdav v0.12.0 is pinned in go.mod; stable)

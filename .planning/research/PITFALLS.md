# Pitfalls Research

**Domain:** Adding robustness tests and fixes to an existing Go WebDAV server (large files, interrupted PUT, concurrency, byte-range GET)
**Researched:** 2026-04-12
**Confidence:** HIGH (Go runtime specifics from official docs + real test harness patterns), MEDIUM (iOS client behaviour from community reports), LOW (CouchDB/Swift atomicity corner cases — single source)

---

## Critical Pitfalls

### Pitfall 1 [MEMORY]: Body-Accumulating Test Helpers Defeat Streaming Validation

**Category:** Large-file tests / Memory

**What goes wrong:**
A test helper reads the full response body into a `[]byte` (via `io.ReadAll`, `httpexpect` `.Body().Raw()`, `ioutil.ReadAll`, or `bytes.Buffer`) to assert content equality. The test passes. But now the test process is holding the entire file in memory — identical to a buffering bug in the server — so any "streaming PUT uses only constant memory" assertion is immediately falsified by the test itself. Worse: if you allocate a 1 GB test fixture and also capture the full response body, the test process peaks at ≥2 GB before GC has a chance to run.

**Why it happens:**
The existing test helpers (`env.E.GET(...).Expect().Status(200)` from httpexpect) are ergonomic and work perfectly for small payloads. Developers copy-paste this pattern for large-file tests without realising `Body().Raw()` calls `io.ReadAll` internally.

**How to avoid:**
- For large-file GET verification, use a streaming hash comparison: write a helper that pipes `resp.Body` through `io.TeeReader` into a `sha256.New()`, draining into `io.Discard`. Compare the digest, never the bytes. Example pattern:
  ```go
  func assertBodyHash(t *testing.T, r io.Reader, wantHex string) {
      h := sha256.New()
      _, err := io.Copy(h, r)
      require.NoError(t, err)
      require.Equal(t, wantHex, hex.EncodeToString(h.Sum(nil)))
  }
  ```
- For PUT, generate the fixture body as an `io.Reader` (not `[]byte`) using `io.LimitReader(rand.Reader, size)` or a seeded deterministic `io.Reader` that produces verifiable output without storing it.
- Never use `WithBytes(bigSlice)` in httpexpect for files > 1 MB. Use `WithBody(reader)` instead.

**Warning signs:**
- Test process RSS peaks at `N * file_size` where N > 1.
- `go test -memprofile` shows `ioutil.ReadAll` or `bytes.(*Buffer).Write` in the heap profile top-10 during large file tests.
- CI OOM-kills during "large file" test suite but not during unit tests.

**Phase to address:** Wave 1 of the large-file phase. Establish the streaming helper pattern before writing any large-file test. Retrofitting is harder.

---

### Pitfall 2 [MEMORY]: Measuring RSS to Prove Streaming — Go Runtime Hoard

**Category:** Large-file streaming validation

**What goes wrong:**
The test measures peak memory via `runtime.ReadMemStats` or `ps -o rss` before and after a PUT, sees RSS drop below `file_size`, and concludes "streaming works." But Go's runtime allocates memory from the OS in arenas and does **not** release it back to the OS immediately after GC. `HeapInuse` and RSS diverge within seconds of a large allocation being freed. A buffering bug that briefly held 512 MB will show a pre/post RSS delta of near zero if measured after the fact.

The companion problem: Go's `io.Copy` uses a 32 KiB internal buffer by default. Any handler that only uses `io.Copy` and never accumulates beyond that is already streaming. But the VFS layer may accumulate (e.g., computing MD5 over the full body before writing to Swift), making `io.Copy` in the handler irrelevant.

**Why it happens:**
Developers know "io.Copy is streaming" and write tests that call `runtime.ReadMemStats` after the PUT returns. By then, GC has reclaimed the buffered data but the arena may still be resident. The measurement looks clean but proved nothing.

**How to avoid:**
- Prove streaming by **interception, not measurement**: instrument the code path (or the test handler) to assert that body bytes are piped through without accumulation. Specifically, use an `io.LimitReader` wrapper that panics if more than `N` bytes (e.g., 2× chunk size) are held in-flight at any instant. Alternatively, use a slow reader (throttled via `time.Sleep` per chunk) and confirm that memory stays flat across the throttled duration via `runtime.ReadMemStats` sampled in a goroutine.
- For a practical CI-safe approach: wrap the request body in a `countingReader` that tracks `maxConcurrentBytes` (using a channel or atomic), assert `maxConcurrentBytes < threshold` after the PUT. This is a white-box test but is far more reliable than RSS.
- If you need black-box validation: run a PUT of a file larger than available heap in a subprocess with `GOMEMLIMIT` set. If the subprocess survives, it streamed. (`GOMEMLIMIT` is available since Go 1.19, controls soft memory limit triggering more aggressive GC.)
- Document that `HeapInuse` is a better proxy than RSS but still not perfect: RSS includes stack, runtime metadata, and unreleased arenas. `HeapAlloc` (actively allocated bytes) is the most accurate metric for "is the body still held."

**Warning signs:**
- "Streaming test" that only calls `runtime.ReadMemStats` once, after the request completes.
- Test peak RSS < file size, but `HeapAlloc` was never sampled concurrently with the in-flight PUT.
- VFS layer calling `io.ReadAll` or `ioutil.ReadAll` on `file.Read()` anywhere in the call chain.

**Phase to address:** Large-file phase, Wave 1. Define the measurement methodology before writing the test.

---

### Pitfall 3 [INTERRUPTED-PUT]: Partial File Committed with Wrong ETag / Orphaned State

**Category:** Interrupted PUT / Correctness

**What goes wrong:**
The client sends a PUT and closes the TCP connection mid-stream (network drop, timeout, explicit abort). Three failure modes:

1. **Partial file committed**: The VFS `file.Close()` runs despite the partial body (e.g., the handler doesn't check `io.Copy` error before calling `Close()`), committing a truncated file with the correct name but wrong size and a stale ETag.
2. **Orphaned blob, no CouchDB doc**: The blob is written to Swift/FS but `file.Close()` returns an error that the handler swallows, so the CouchDB document is never created. The storage grows silently.
3. **Orphaned CouchDB doc, no blob**: The inverse — metadata committed but the blob write failed. Subsequent GET returns 404 or a corrupt read.
4. **Overwrite rollback deletes original**: The PUT was overwriting an existing file. The handler calls `fs.CreateFile(newdoc, olddoc)` — VFS moves `olddoc` to a temporary state. On abort, if rollback logic is missing or errors, the original file is gone even though the new file never committed.

The current `put.go` in v1.1 has the correct pattern (`io.Copy` error checked, `file.Close()` error merged), but this is the exact class of bug that interruption tests will surface if any VFS backend has a subtlety.

**Why it happens:**
`context.Context` cancellation propagates through `io.Copy` — when the client disconnects, `c.Request().Body.Read()` returns `io.ErrUnexpectedEOF` or `context.Canceled`. If the handler doesn't distinguish "body read error" from "commit error", it may attempt `file.Close()` in a partial state. The VFS's transactional guarantees depend on the backend: local-FS can leave a half-written temp file; Swift can leave a partial object.

**How to avoid:**
- In the PUT handler: if `io.Copy` returns a non-nil error, call `file.Close()` anyway (to trigger the VFS abort path) but **do not** return a 2xx. Return 500. The current `put.go` already merges errors correctly — preserve this pattern for any new chunked/range PUT code.
- Write an interruption test that:
  1. Starts a goroutine sending a slow body (throttled reader).
  2. Cancels the request context mid-way via `cancel()`.
  3. After the handler returns, asserts no file exists at the path (or the original file is intact if it was an overwrite).
  4. Queries CouchDB directly (via VFS) to confirm no orphaned document.
- For the overwrite case: assert that after an aborted PUT to an existing path, the original file is still readable and has its original ETag.

**Warning signs:**
- `file.Close()` called unconditionally in a `defer` without checking whether `io.Copy` succeeded (a defer-only pattern can mask the abort).
- VFS backend logs showing "wrote X bytes, expected Y" without a corresponding CouchDB rollback.
- Integration test that only checks "no file at new path" but not "original file still intact."

**Phase to address:** Interrupted PUT phase. The overwrite-rollback case specifically needs a dedicated test before the chunked-upload feature is added (it reuses the same overwrite path).

---

### Pitfall 4 [CONCURRENCY]: Sleep-Based Synchronisation in Concurrent Tests

**Category:** Concurrency tests / Flakiness

**What goes wrong:**
A concurrent-write test launches two goroutines that both PUT to the same path, then `time.Sleep(100 * time.Millisecond)` and asserts "exactly one of them got 204, the other got 412." On fast hardware this works. In CI on a slow disk image (CouchDB writes are slower, VFS has more latency), both goroutines may still be in flight when the sleep expires, making the assertion race. Alternatively: both finish before the sleep, but goroutine scheduling caused one to start before the other had a chance to read the ETag, and the result is non-deterministic.

**Why it happens:**
Developers reach for `time.Sleep` because channels/WaitGroups add boilerplate to test code, and the sleep "works locally." CI environments are slower and have higher variance.

**How to avoid:**
- Synchronise with channels or `sync.WaitGroup`, never with `time.Sleep`. Pattern:
  ```go
  results := make(chan int, 2)
  for i := 0; i < 2; i++ {
      go func() {
          resp := putRequest(...)
          results <- resp.StatusCode
      }()
  }
  a, b := <-results, <-results
  // exactly one 204 and one 412 (or both 204 if no ETag precondition)
  ```
- Use a `sync.Barrier` or `errgroup` to ensure both goroutines start simultaneously: send a "ready" signal from each, wait for both, then release. This maximises the race window.
- For ETag-based concurrency (If-Match on concurrent PUT), seed the file with a known ETag before the test so the first PUT wins and the second gets 412 deterministically.
- Add `-count=5` or `-count=10` to concurrency tests in CI to catch intermittent failures; `-race` flag is mandatory.

**Warning signs:**
- `time.Sleep` anywhere in a test that also launches goroutines.
- Test is marked `t.Skip("flaky")` without a linked issue.
- Test passes locally with `-count=1` but fails with `-count=10`.

**Phase to address:** Concurrency phase, Wave 1. Establish the channel-synchronisation helper before writing tests.

---

### Pitfall 5 [CONCURRENCY]: Goroutine Leaks Poisoning Subsequent Tests

**Category:** Concurrency tests / Test isolation

**What goes wrong:**
A concurrent PUT test launches goroutines that block on a slow VFS write. The test asserts the status codes and returns, but the goroutines haven't finished. Go's test runner moves to the next test. The leaked goroutines now complete their writes against the same CouchDB test instance, interfering with the next test's preconditions (e.g., a file that should not exist now appears).

The `t.Cleanup` hook runs after the test function returns but before the next test starts — however, it cannot cancel goroutines that don't respect a `context.Context`.

**Why it happens:**
Test goroutines are spawned without a cancellable context. When the test ends, there's no mechanism to stop them. The VFS write is not context-aware, so it completes regardless.

**How to avoid:**
- Always use `context.WithTimeout(context.Background(), testTimeout)` and pass it to HTTP requests. Set a generous but finite timeout (e.g., 5 seconds for concurrency tests).
- Use `goleak.VerifyNone(t)` (from `go.uber.org/goleak`) at the end of each concurrency test. This fails the test if goroutines are still running when the test ends, making leaks visible immediately rather than at the next test's unexpected failure.
- Alternatively, use `testify/suite` with a `TearDownTest` that calls `goleak.VerifyNone(s.T())`.

**Warning signs:**
- Test isolation failures: a test that creates file A and asserts it exists starts failing because a prior test's leaked goroutine already created file A.
- Non-deterministic test ordering (Go test runner doesn't guarantee order within a package) combined with leaked goroutines.
- `go test -v` showing goroutines still running after "--- PASS:" lines.

**Phase to address:** Concurrency phase, before the first concurrent-PUT test is written.

---

### Pitfall 6 [BYTE-RANGE]: Incorrect 206 vs 200 and Content-Range Header

**Category:** Byte-range GET / RFC 7233 compliance

**What goes wrong:**
RFC 7233 requires:
- A valid, unsatisfied range (bytes beyond file size): **416 Range Not Satisfiable** with `Content-Range: bytes */total`.
- A range covering the entire file: **200 OK** (not 206), because no "partial" content is being served.
- A valid partial range: **206 Partial Content** with `Content-Range: bytes first-last/total`.
- Multi-range: **206** with `Content-Type: multipart/byteranges; boundary=...`.

The current `get.go` delegates to `vfs.ServeFileContent`, which wraps `http.ServeContent`. Go's `http.ServeContent` handles single-range correctly (206 + Content-Range) but its multi-range support generates `multipart/byteranges` with correct MIME structure. The pitfall is **not** in the delegated path — it's in any **new** code that wraps or pre-processes Range headers before passing them to `ServeContent`, or in a custom range handler added for chunked upload progress.

A common bug: the handler parses the `Range` header, detects an out-of-bounds range, returns `200 OK` with the full body instead of `416`. This is wrong per RFC 7233 §4.4.

**How to avoid:**
- Do not write a custom Range handler. Trust `http.ServeContent` / `vfs.ServeFileContent`. If you must pre-process:
  - Parse `Range` with `http.ParseRange(header, fileSize)` — it returns `([]httpRange, error)`. If error, return 416.
  - If the result covers the entire file, suppress the Range header before calling ServeContent (it will then return 200).
- For the `If-Range` precondition: if the ETag in `If-Range` does not match the current ETag, the Range header must be **ignored** and the full file returned as 200. This is a correctness bug that `http.ServeContent` handles correctly only if you pass it the ETag via `modtime` + `name` — verify this is wired correctly.
- Write explicit tests for:
  - `Range: bytes=0-` on a 0-byte file → 200 (or 416 — RFC allows either; choose one and document it).
  - `Range: bytes=0-999` on a 100-byte file → 416.
  - `Range: bytes=0-99` on a 100-byte file → 200 (entire file).
  - `Range: bytes=0-49, 50-99` → 206 multipart.

**Warning signs:**
- GET handler intercepting or rewriting the `Range` header before passing to `ServeFileContent`.
- Test suite only covers `Range: bytes=0-N` where N < file size.
- Missing test for `Range` on a zero-byte file.
- iOS Files app shows stale content: this is often the client's URL session cache, but also frequently caused by wrong ETag on partial responses.

**Phase to address:** Byte-range GET phase, Wave 1. Test `http.ServeContent` delegation first, add custom range logic only if delegation is insufficient.

---

### Pitfall 7 [CHUNKED-UPLOAD]: Stale Session Leak and Race on Concurrent Chunk Writes

**Category:** Content-Range PUT / Chunked upload design

**What goes wrong:**
If chunked upload is implemented (upload session per file, identified by an upload ID):

1. **Stale session leak**: A client starts an upload, sends chunk 1, then disconnects. The partial blob and the session metadata (CouchDB doc?) are never cleaned up. After many such aborts, storage grows unboundedly.
2. **Concurrent chunk race**: Two clients both PUT chunk 5 of the same upload ID. Both succeed. Now chunk 5 is duplicated or corrupt.
3. **Sparse file creation**: The server accepts chunks out of order and writes them at their declared byte offsets without validating that earlier chunks arrived first. If the client sends chunk 3 before chunk 2, the server creates a sparse file with a zero-filled gap.
4. **Offset validation missing**: `Content-Range: bytes 0-999/5000` on chunk 1, then `Content-Range: bytes 2000-2999/5000` on chunk 2 — the server accepts a 1000-byte gap, creating a corrupt file.

**Why it happens:**
Chunked upload is stateful. Most WebDAV implementations (including cozy-stack v1.1) are stateless per-request. Adding upload sessions requires distributed state (CouchDB or in-memory), and the failure modes are non-obvious.

**How to avoid:**
- Before implementing chunked upload: decide whether it is in scope. RFC 4918 does not require it. Only implement if a specific client (iOS Files, rclone) demonstrably needs it.
- If implemented: use CouchDB as the session store (not in-memory — restarts kill in-memory state mid-upload). Each session doc tracks: upload ID, file path, total expected size, received byte ranges (as a sorted interval list), blob reference.
- Strict offset enforcement: reject any chunk whose `Content-Range` start offset != size of all previously received bytes. Return `409 Conflict`.
- Session cleanup: add a TTL to session docs (e.g., 24 hours). A background job or a lazy cleanup on session create deletes expired sessions and their blobs.
- Concurrent chunk protection: use CouchDB optimistic locking (rev-based) on the session doc to reject a second concurrent write to the same chunk.

**Warning signs:**
- The `Content-Range` header on PUT is parsed but the offset is not validated against received chunks.
- Upload sessions stored only in a Go `sync.Map` (lost on restart, no TTL).
- No test for "client sends chunk 2 before chunk 1" scenario.

**Phase to address:** Only if Content-Range PUT is scoped into v1.2. Gate behind an explicit design decision. If the decision is "not yet", add a 501 Not Implemented response for `Content-Range` PUT to avoid silent corruption.

---

### Pitfall 8 [CI]: CouchDB Container Startup Race and Litmus Package Drift

**Category:** CI integration

**What goes wrong:**
1. **CouchDB startup race**: The GitHub Actions job starts CouchDB as a service container and immediately runs tests. CouchDB takes 2-8 seconds to be ready. The first PROPFIND (or the test harness's `NeedCouchdb` check) times out, the job fails with a connection error, and the failure looks like a flaky test rather than an infrastructure race.
2. **litmus package drift**: `apt-get install litmus` on Ubuntu 22.04 installs litmus 0.13; Ubuntu 24.04 installs 0.14 (or a different patchlevel). Litmus 0.13 and 0.14 differ in how they handle `PROPPATCH` responses with mixed success/failure (207 vs 400). A suite that passes on 0.13 may fail on 0.14 for PROPPATCH edge cases, not because the server regressed but because the test changed.
3. **Parallel litmus runs**: If two CI jobs run litmus against the same CouchDB instance and the same cozy instance concurrently, litmus creates/deletes files in predictable paths (`/litmus/`, `/litmus2/`). Concurrent runs corrupt each other's state.

**How to avoid:**
- CouchDB startup: add a `wait-for-it.sh` step or a `healthcheck` loop in the CI job before starting tests:
  ```yaml
  - name: Wait for CouchDB
    run: |
      for i in $(seq 1 30); do
        curl -sf http://localhost:5984/ && break
        sleep 1
      done
  ```
  The existing `testutils.NeedCouchdb(t)` helper likely already retries, but the CI wait step catches cases where the connection is refused before Go test binary even starts.
- litmus version pinning: do not rely on `apt-get install litmus`. Instead, use the prebuilt binary approach: check the litmus version in the `make test-litmus` script with `litmus --version` and fail fast with a clear message if the wrong version is installed. Or: download a specific release tarball in CI and cache it.
- Parallel isolation: each CI job should use a unique cozy instance subdomain (already done via `testutils.NewSetup` which uses `t.Name()` as a seed). The litmus script should also create a unique test collection per run (e.g., `/litmus-$CI_JOB_ID/`).

**Warning signs:**
- CI failure log shows "connection refused" to CouchDB on port 5984, not a test assertion failure.
- litmus failure on `props: 30/30` locally but `props: 28/30` in CI — litmus version mismatch.
- Two CI jobs started from the same PR fail alternately rather than consistently — shared state.

**Phase to address:** CI litmus integration phase (deferred from v1.1). Address startup race first (it's the most common CI failure mode).

---

### Pitfall 9 [CI]: Test Fixture Size and Git Repo Bloat

**Category:** Large-file tests / Repository hygiene

**What goes wrong:**
A developer creates a test that needs a 100 MB file. The simplest approach: `testdata/large-file-100MB.bin` checked into git. After three rounds of this (different sizes for different tests), the repo gains 500 MB of binary fixtures that every checkout must download, git clone becomes slow, and CI checkout time spikes.

**Why it happens:**
Test fixtures checked into git are the path of least resistance when you need "a file with known content."

**How to avoid:**
- Never check binary test fixtures into git. Generate them programmatically:
  ```go
  func generateFixture(t *testing.T, size int64) (path string, sha256hex string) {
      f, err := os.CreateTemp(t.TempDir(), "fixture-*")
      require.NoError(t, err)
      t.Cleanup(func() { os.Remove(f.Name()) })
      h := sha256.New()
      r := io.TeeReader(io.LimitReader(rand.Reader, size), h)
      _, err = io.Copy(f, r)
      require.NoError(t, err)
      return f.Name(), hex.EncodeToString(h.Sum(nil))
  }
  ```
  This produces a deterministic-enough fixture (content is random but the hash is computed fresh each time) that never touches the repo.
- For reproducible fixtures (same bytes every run): use a seeded `rand.New(rand.NewSource(42))` reader.
- Add `testdata/*.bin` and `testdata/*.dat` to `.gitignore` as a safety net.

**Warning signs:**
- `testdata/` directory with files larger than a few KB.
- `git lfs` discussion without a decision — means large files are being contemplated for git storage.
- CI checkout step taking > 30 seconds.

**Phase to address:** Large-file phase, before any fixture is generated. Add a linting check or CI rule that rejects binary files > 100 KB in `testdata/`.

---

### Pitfall 10 [iOS-VALIDATION]: URL Session Cache and Subnet Isolation Masking Server Bugs

**Category:** iOS Files app manual validation

**What goes wrong:**
1. **URL session cache**: iOS's `NSURLSession` aggressively caches WebDAV responses (especially PROPFIND and GET). A bug is fixed on the server, but the iOS client continues to show stale content because its cache hasn't been invalidated. The tester sees "correct" behaviour and signs off, but a fresh mount (first connection) would show the old bug.
2. **Subnet isolation**: The iOS device is on a different Wi-Fi subnet or behind a VPN. The device reaches the cozy-stack dev server but the server's HTTPS certificate is for `localhost` or a `.local` domain that iOS rejects with `NSURLErrorDomain -1202` (SSL error), silently falling back to a read-only view. The tester sees files but cannot write.
3. **iOS 17+ DAV changes**: iOS 17 changed the default `User-Agent` string and added additional precondition headers (`If-None-Match` on COPY) not present in iOS 16. A server tested only against iOS 16 may fail the `If-None-Match` on COPY edge case on iOS 17+.

**How to avoid:**
- Before each iOS validation session: on the iOS device, go to Settings → General → Transfer or Reset iPhone → Reset → Reset Network Settings. This clears the URL session cache. Or: in the Files app, remove the server connection and re-add it.
- Use a proper TLS certificate (Let's Encrypt or mkcert) for the dev server domain, not a self-signed cert, and ensure the domain is reachable from the iOS device's network. The simplest setup: deploy on a public staging instance with a valid cert.
- Test iOS 17+ explicitly with an iOS 17 device if available. Focus on COPY, MOVE, and LOCK (even though LOCK is not implemented — iOS may send LOCK and expect 405 Method Not Allowed, not an unhandled panic).
- Test the unhappy paths explicitly: rename a file to an already-existing name (expect Overwrite behaviour), move a file to a directory that doesn't exist (expect 409), and upload a file that would exceed quota (expect 507).

**Warning signs:**
- iOS Files app shows the old file name after a server-side rename until the app is backgrounded and foregrounded.
- Tester reports "seems fine" but only tested read operations.
- iOS device on a different VLAN than the dev server.

**Phase to address:** iOS validation phase (last phase of v1.2, after robustness features are stable). Use a staging environment, not localhost.

---

## Technical Debt Patterns

Shortcuts that seem reasonable but create long-term problems.

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| `time.Sleep` for goroutine synchronisation in tests | No boilerplate | Flaky CI on slow hardware, false passes | Never — use `sync.WaitGroup` or channels |
| `io.ReadAll` in test body verification for large files | Simple equality assertion | OOM in CI, defeats streaming validation | Only for files < 1 MB |
| Binary fixtures in `testdata/` | Stable, reproducible | Repo bloat, slow checkout | Never for files > 100 KB |
| Measuring RSS after-the-fact to prove streaming | "Passes the smell test" | Proves nothing due to Go arena retention | Never — use concurrent sampling or GOMEMLIMIT subprocess |
| In-memory upload session store for chunked upload | Simple to implement | Lost on restart, no TTL, no persistence | Only for a prototype; must be replaced before shipping |
| `defer file.Close()` without checking `io.Copy` error | Idiomatic Go pattern | Commits partial file if body read fails | Never for PUT handler — error must be checked before Close |
| Skipping `If-Range` precondition for byte-range tests | Fewer test cases | Clients (iOS, rclone) send `If-Range`; silent regression possible | Never — add at least one `If-Range` test case |
| Running litmus against a shared CouchDB instance in CI | Simpler CI config | Parallel jobs corrupt each other's test state | Only for serial CI pipelines (never for parallel) |

---

## Integration Gotchas

Common mistakes when connecting to external services or test harnesses.

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| CouchDB in CI | Starting tests immediately after container `healthy` signal | Add an explicit HTTP poll loop (30 retries × 1s) on `/_up` endpoint before running tests |
| litmus apt package | Using `apt-get install litmus` without version pinning | Pin version via tarball download in CI; check `litmus --version` at script start |
| Swift/local VFS on test instance | Using the default shared temp dir across tests | Set `config.GetConfig().Fs.URL` to `t.TempDir()` per test (already done in `testutil_test.go`) |
| gowebdav client library | Using it for large-file tests (it may buffer internally) | Check gowebdav's `ReadStream` vs `Read` methods; use `ReadStream` for large files |
| httpexpect for large responses | `Body().Raw()` accumulates full response in memory | Use `Body().Reader()` (if available) or switch to `net/http` directly for large-file assertions |
| iOS Files app URL session cache | Testing after server changes without cache reset | Reset network settings on device or disconnect/reconnect server before each session |
| GitHub Actions services (CouchDB) | Assuming `services.couchdb.ports` means it's immediately ready | Always add a wait step; Docker `healthy` state ≠ CouchDB accepting queries |

---

## Performance Traps

Patterns that work at small scale but fail as test corpus grows.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Running all large-file tests sequentially without explicit GC | Memory grows across tests; later tests OOM | Call `runtime.GC()` + `debug.FreeOSMemory()` between large-file tests in `TestMain` | At 3+ large-file tests in sequence |
| SHA256 of full body loaded into memory for assertion | Memory spike = 2× file size | Use streaming hash (`io.TeeReader` → `sha256.New()` → `io.Discard`) | At any file > 100 MB |
| CouchDB poll loop without timeout | Concurrent test starvation if CouchDB is wedged | Set a hard timeout (30s) on the poll loop; fail fast, don't hang | Any time CouchDB is slow to start |
| `go test ./web/webdav/... -race` with leaked goroutines | False data races reported for subsequent tests | Use `goleak.VerifyNone(t)` per concurrency test | From the first concurrent test |
| Parallel litmus runs against same cozy instance | Litmus creates/deletes predictable paths; runs corrupt each other | Unique instance or unique base collection path per litmus invocation | Any parallel CI setup |

---

## Security Mistakes

Domain-specific security issues specific to the robustness phase.

| Mistake | Risk | Prevention |
|---------|------|------------|
| Accepting `Content-Range` PUT without strict offset validation | Sparse file creation; client can write arbitrary byte ranges → corrupt files | Reject any `Content-Range` PUT where offset ≠ total received bytes so far; return 409 |
| Not aborting VFS file on interrupted PUT | Partial file committed; ETag no longer reflects actual content | Ensure `io.Copy` error propagates to prevent `file.Close()` from committing partial content |
| Exposing upload session IDs that are guessable | One user can resume another user's upload by guessing UUID | Use `crypto/rand` UUIDs for session IDs; validate session ownership against the authenticated instance |
| Accepting `Range` headers without size bounds on download | A client sending `Range: bytes=0-999999999999` could cause the server to seek to an invalid offset | Rely on `http.ServeContent` / `vfs.ServeFileContent` which handles this correctly — do not write a custom range handler |

---

## "Looks Done But Isn't" Checklist

- [ ] **Streaming PUT test**: Asserts memory stays bounded — not just that the file was written correctly. Verify with concurrent `HeapAlloc` sampling, not post-hoc RSS.
- [ ] **Interrupted PUT test**: Asserts original file is intact after abort (overwrite path), not just that the new file doesn't exist.
- [ ] **Byte-range GET**: Has a test for `Range: bytes=0-N` where N >= file size → 416 (not silent truncation).
- [ ] **Byte-range GET**: Has a test for `Range: bytes=0-N` where N = file size - 1 (entire file as range) → 200 or 206 (per implementation decision, must be consistent).
- [ ] **Concurrency test**: Uses channel synchronisation, not `time.Sleep`. Run with `-count=10` and `-race` in CI.
- [ ] **Concurrency test**: Uses `goleak.VerifyNone(t)` to catch goroutine leaks before the next test.
- [ ] **CI litmus**: Startup wait loop for CouchDB is present. litmus version is pinned or checked.
- [ ] **iOS validation**: Tests include rename-to-existing-name, move-to-nonexistent-parent, and upload-exceeding-quota (not just happy paths).
- [ ] **Large-file fixtures**: Generated programmatically in `t.TempDir()`, never checked into git.
- [ ] **Content-Range PUT** (if in scope): Strict offset validation is present. Session TTL and cleanup are implemented. Returns 501 if not in scope.

---

## Recovery Strategies

When pitfalls occur despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Binary fixtures accidentally committed to git | HIGH | `git filter-branch` or `git filter-repo` to rewrite history, then force-push; notify all contributors to re-clone. Prevention is far cheaper. |
| Partial file committed after interrupted PUT | MEDIUM | Manually delete the orphaned CouchDB doc + Swift blob via the cozy admin API. Add a VFS integrity check script. |
| Goroutine leaks accumulating across test run | LOW | Add `goleak.VerifyNone(t)` to failing test; trace the leaking goroutine via `go test -v` goroutine dump. |
| Flaky concurrency test in CI | MEDIUM | Replace `time.Sleep` with channel sync; add `-count=10 -race` to reproduce; check for shared mutable state across goroutines. |
| litmus version mismatch breaking CI | LOW | Pin litmus version in CI; run `litmus --version` as a CI pre-check step. |
| iOS cache masking server bug that shipped | HIGH | The bug is in production. Ship a fix; instruct users to disconnect and reconnect the Files server. |
| CouchDB startup race causing false CI failures | LOW | Add wait loop to CI job; set `continue-on-error: false` on the wait step so the failure is explicit. |

---

## Pitfall-to-Phase Mapping

How v1.2 roadmap phases should address these pitfalls.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Body-accumulating test helpers (P1) | Large-file phase, Wave 1 | No `io.ReadAll` / `Body().Raw()` in large-file test files; code review check |
| RSS measurement trap (P2) | Large-file phase, Wave 1 | Streaming proof uses concurrent `HeapAlloc` sampling or `GOMEMLIMIT` subprocess |
| Interrupted PUT orphans/rollback (P3) | Interrupted PUT phase | Test asserts original file intact after abort; VFS doc count unchanged after failed PUT |
| Sleep-based concurrency sync (P4) | Concurrency phase, Wave 1 | `grep -r 'time\.Sleep' web/webdav/*_test.go` returns 0 hits in new test files |
| Goroutine leaks between tests (P5) | Concurrency phase, Wave 1 | `goleak.VerifyNone(t)` at end of each concurrent test |
| 206 vs 200 and Content-Range header (P6) | Byte-range GET phase | Explicit tests for each RFC 7233 edge case (full-file range → 200, out-of-bounds → 416) |
| Chunked upload session leaks and race (P7) | Content-Range PUT phase (if scoped) | Test: abort mid-upload → no orphaned storage. Gate: explicit design decision before any implementation. |
| CouchDB startup race in CI (P8) | CI litmus integration phase | CI job has wait loop; no "connection refused" in logs |
| litmus package drift (P8) | CI litmus integration phase | `litmus --version` check in script; version pinned in CI |
| Test fixture git bloat (P9) | Large-file phase, Wave 1 | `git ls-files -- testdata/ | xargs du -sh` shows no files > 100 KB |
| iOS cache / subnet masking (P10) | iOS validation phase | Validation checklist includes cache reset step; staging environment used |

---

## Sources

- Go `runtime.MemStats` documentation: https://pkg.go.dev/runtime#MemStats — distinction between `HeapAlloc`, `HeapInuse`, and OS-level RSS.
- RFC 7233 §4 (Range Requests): defines 206, 416, `If-Range` semantics. https://datatracker.ietf.org/doc/html/rfc7233
- RFC 4918 §9.7.1 (PUT): "A PUT that would result in the creation of a resource without an appropriate parent collection MUST fail with a 409 Conflict."
- Go `http.ServeContent` source (stdlib): handles single and multi-range, `If-Range`, and full-range → 200 promotion automatically.
- `go.uber.org/goleak`: goroutine leak detector for Go tests. https://github.com/uber-go/goleak
- GOMEMLIMIT (Go 1.19+): https://pkg.go.dev/runtime — soft memory limit for subprocess streaming validation.
- CVE-2023-39143 (PaperCut WebDAV path traversal): referenced for context on path handling; not directly applicable to v1.2 scope.
- cozy-stack `web/webdav/put.go` v1.1: current implementation correctly merges `io.Copy` error with `file.Close()` error — this pattern must be preserved in any new PUT variant.
- cozy-stack `web/webdav/testutil_test.go` v1.1: existing harness sets `t.TempDir()` per test for VFS isolation — large-file tests must reuse this, not introduce a shared temp directory.

---
*Pitfalls research for: WebDAV robustness testing and fixes on an existing Go WebDAV server*
*Researched: 2026-04-12*

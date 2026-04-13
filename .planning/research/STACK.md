# Stack Research

**Domain:** WebDAV server layer on top of an existing Go VFS abstraction
**Researched:** 2026-04-04 (v1.1) · Updated 2026-04-12 (v1.2 addendum)
**Confidence:** HIGH (server library analysis from source + official issue tracker; MEDIUM for emersion/go-webdav Overwrite specifics, unverified against raw source)

---

## v1.2 Addendum: Robustness Testing Stack

*What's new in v1.2 — stack decisions for large-file E2E, interrupted PUT, concurrency, multi-range, CI litmus, and benchmark recording. The v1.1 server stack (custom handlers, stdlib XML, gowebdav client, httpexpect) remains unchanged.*

---

### 1. Large File E2E Tests (5 GB uploads/downloads, streaming proof via memory measurement)

**Fixture strategy: generate on-the-fly with `io.LimitedReader` + `crypto/rand`-seeded `bytes.Repeat`.**

Do NOT check in binary fixtures. A 5 GB file in the repo is impractical in every dimension (clone time, CI disk, git LFS complexity). Instead, construct a synthetic `io.Reader` at test time:

```go
// 5 GB of repeating pattern — zero allocation aside from the 4 KB seed.
seed := bytes.Repeat([]byte("cozy-webdav-large-file-test\n"), 4096/28+1)
body := io.LimitReader(bytes.NewReader(seed), 5*1024*1024*1024)
```

For upload: pass `body` directly to `gowebdav.WriteStreamWithLength` or a raw `http.NewRequest("PUT", ...)`. For download: pipe server response through `io.Copy(io.Discard, resp.Body)` counting bytes.

**Memory measurement: `runtime.MemStats` + `runtime/metrics`, NOT `/proc/PID/status` RSS.**

The goal is to verify that a 5 GB PUT does not buffer the full body in heap. Use `runtime.ReadMemStats` before and after upload and assert that `HeapInuse` growth is bounded (e.g., stays under 128 MB above baseline). This is an in-process measurement; it does not require external tooling.

```go
var before, after runtime.MemStats
runtime.GC()
runtime.ReadMemStats(&before)
// ... run upload ...
runtime.GC()
runtime.ReadMemStats(&after)

heapGrowth := int64(after.HeapInuse) - int64(before.HeapInuse)
assert.Less(t, heapGrowth, int64(128*1024*1024), "PUT must not buffer >128MB in heap for 5GB upload")
```

`runtime/metrics` (Go 1.16+, stable in Go 1.25) can complement this for `/memory/classes/heap/live:bytes` and `/memory/classes/total:bytes`, but `runtime.ReadMemStats` is simpler and sufficient for a streaming bound test.

**No new library needed.** `runtime`, `io`, `bytes`, `crypto/rand` are all stdlib. `gowebdav` v0.12.0 (already in go.mod) provides `WriteStreamWithLength(path, reader, size, mode)` for the upload path.

**Large test gating:** Tag large-file tests with a custom build tag or `testing.Short()` skip so they do not run in normal CI. Run only in a dedicated "robustness" CI job or locally.

```go
func TestLargeFilePUT(t *testing.T) {
    if testing.Short() {
        t.Skip("large file test: skipped in short mode")
    }
    // ...
}
```

---

### 2. Interrupted PUT Tests (client connection drop mid-transfer)

**Use stdlib `net.Pipe` + a custom `io.Reader` that closes the pipe mid-stream. No external library needed.**

The pattern: create a `net.Pipe()`, write the first N bytes of the PUT body through the write end, then `wc.Close()` to simulate a dropped connection. The server handler receives `io.ErrUnexpectedEOF` or `io.EOF` mid-read.

```go
func TestInterruptedPUT(t *testing.T) {
    env := newWebdavTestEnv(t, nil)

    pr, pw := io.Pipe()

    // Write 1 MB then close the writer mid-stream.
    go func() {
        chunk := bytes.Repeat([]byte("x"), 1024*1024)
        pw.Write(chunk)
        pw.CloseWithError(errors.New("simulated client disconnect"))
    }()

    req, _ := http.NewRequest("PUT", env.TS.URL+"/dav/files/partial.bin", pr)
    req.Header.Set("Authorization", "Bearer "+env.Token)
    req.ContentLength = 100 * 1024 * 1024 // 100MB declared, only 1MB sent
    resp, err := http.DefaultClient.Do(req)
    // Server must either return an error response or close cleanly.
    // Assert the VFS does not contain a partially-written zombie file.
    // ...
}
```

`io.Pipe` is the right primitive: it is synchronous (no buffering), error-propagating (CloseWithError surfaces on the read end), and in-process (no TCP socket needed). This integrates directly with the existing `newWebdavTestEnv`/`httptest.Server` harness.

**Alternative considered: `http.Hijacker`.** Hijacking the server-side connection mid-response is more complex and interacts poorly with Echo's response writer. The pipe-on-the-client-side approach is cleaner because it simulates the client dropping the connection, which is the real-world scenario.

**`Shopify/toxiproxy` is NOT needed.** Toxiproxy is appropriate for testing network conditions (latency, packet loss, bandwidth limits) in integration environments where the server is a separate process. For in-process httptest server tests, `io.Pipe` with controlled writes is simpler, faster, deterministic, and adds zero dependencies.

**No new library needed.** `io.Pipe`, `io.Reader`, `net/http` are stdlib.

---

### 3. Concurrent PUT/PROPFIND/MOVE Tests (multiple goroutines hitting the same path)

**Use `sync.WaitGroup` + `t.Parallel()` goroutine fan-out with the `-race` flag. No new library needed.**

Standard Go pattern for exercising concurrency in HTTP handlers:

```go
func TestConcurrentPUT(t *testing.T) {
    env := newWebdavTestEnv(t, nil)
    const workers = 10
    var wg sync.WaitGroup
    wg.Add(workers)
    for i := 0; i < workers; i++ {
        go func(n int) {
            defer wg.Done()
            path := fmt.Sprintf("/dav/files/concurrent-%d.txt", n)
            // Each worker PUTs a unique file — no shared-path contention yet.
            // For shared-path contention, all workers target the same path.
        }(i)
    }
    wg.Wait()
    // Assert: VFS state is self-consistent. No partial files. Correct ETag.
}
```

Run with `go test -race ./web/webdav/...` to catch data races in handler code. This is the standard Go race detector — no external library needed.

**`testing/synctest` (Go 1.25, now stable):** The `testing/synctest` package graduated from experimental (Go 1.24) to stable in Go 1.25 (which is the `go` directive in `go.mod`). It is useful for testing time-dependent concurrent behavior (e.g., timeout races, lock expiry). For concurrent HTTP tests against a live `httptest.Server`, `sync.WaitGroup` is simpler and more appropriate. `testing/synctest` is an option for unit-level concurrency tests inside handlers (e.g., testing the dead-property store under concurrent PROPPATCH), but not the primary tool for E2E goroutine fans.

**No new library needed.** `sync`, `testing` are stdlib.

---

### 4. Byte-Range GET Multi-Range (multipart/byteranges)

**Use raw `http.Client` for multi-range requests. `gowebdav.ReadStreamRange` handles single-range only.**

`gowebdav` v0.12.0 exposes `ReadStreamRange(path, offset, length)` which generates a single `Range: bytes=offset-(offset+length-1)` header. It does not support multi-range requests (multiple comma-separated ranges → `multipart/byteranges` response).

For multi-range tests, issue a raw HTTP GET with `Range: bytes=0-4,10-14` and parse the `multipart/byteranges` response using stdlib `mime/multipart`:

```go
req, _ := http.NewRequest("GET", env.TS.URL+"/dav/files/test.txt", nil)
req.Header.Set("Authorization", "Bearer "+env.Token)
req.Header.Set("Range", "bytes=0-4,10-14")
resp, _ := http.DefaultClient.Do(req)

// Expect 206 with multipart/byteranges Content-Type.
assert.Equal(t, 206, resp.StatusCode)
ct := resp.Header.Get("Content-Type")
assert.Contains(t, ct, "multipart/byteranges")

// Parse the multipart body.
mediaType, params, _ := mime.ParseMediaType(ct)
assert.Equal(t, "multipart/byteranges", mediaType)
mr := multipart.NewReader(resp.Body, params["boundary"])
// Read parts, assert Content-Range headers and body bytes.
```

**Key implementation note:** `vfs.ServeFileContent` delegates to `http.ServeContent` (confirmed in `get.go` line 59). Go's `net/http.ServeContent` has supported `multipart/byteranges` since Go 1.0 (issue #3784, fixed 2012-06-29). Multi-range is handled transparently by the stdlib. The v1.2 task is to add a test proving this works — not to implement it.

**Libraries needed:** `mime` and `mime/multipart` are stdlib. No new library needed for tests. `gowebdav` remains the primary E2E client; raw `http.Client` is the complement for multi-range assertions.

---

### 5. CI Integration of Litmus (GitHub Actions)

**Use `owncloud-ci/litmus` Docker image via the `notroj/litmus` upstream + a custom GitHub Actions job. Do NOT use `workgroupengineering/litmus` GitHub Action.**

**Options researched:**

| Option | Status | Verdict |
|--------|--------|---------|
| `apt install litmus` in CI runner | Debian/Ubuntu package is litmus 0.13 (2014). Current upstream `notroj/litmus` is at December 2024 commit. Package drift is severe. | REJECT — version is stale |
| `workgroupengineering/litmus` GitHub Action | Single commit, September 2021, no releases, no maintenance. | REJECT — abandoned |
| `owncloud/litmus` Docker Hub image (`hub.docker.com/r/owncloud/litmus`) | Built from `owncloud-ci/litmus`, last meaningful update May 2024. Accepts `LITMUS_URL`, `LITMUS_USERNAME`, `LITMUS_PASSWORD`, `LITMUS_TIMEOUT` env vars. Uses `owncloudci/litmus` Docker Hub tag. | ACCEPT with caveat |
| Build from `notroj/litmus` source in CI | Upstream maintained (December 2024 activity), has Dockerfile. Build adds ~3 min to CI job but gives exact version control. | ACCEPT — preferred if Docker image stagnates |

**Recommended approach:** Use `owncloud/litmus` Docker image in a GitHub Actions job that starts `cozy-stack` as a service, creates an instance, runs litmus in a container step.

```yaml
# .github/workflows/webdav-litmus.yml (outline)
jobs:
  litmus:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - name: Build cozy-stack
        run: go build -o cozy-stack .
      - name: Start cozy-stack
        run: ./cozy-stack serve &
      - name: Wait for stack
        run: until curl -sf http://localhost:8080/version; do sleep 1; done
      - name: Create litmus instance + token
        run: |
          ./cozy-stack instances add lm-ci.localhost:8080 --passphrase cozy --email litmus@ci.test
          echo "TOKEN=$(./cozy-stack instances token-cli lm-ci.localhost:8080 io.cozy.files)" >> $GITHUB_ENV
      - name: Run litmus (native route)
        run: |
          docker run --rm --network host \
            -e LITMUS_URL="http://lm-ci.localhost:8080/dav/files/" \
            -e LITMUS_USERNAME="" \
            -e LITMUS_PASSWORD="${{ env.TOKEN }}" \
            owncloud/litmus:latest
      - name: Run litmus (Nextcloud compat route)
        run: |
          docker run --rm --network host \
            -e LITMUS_URL="http://lm-ci.localhost:8080/remote.php/webdav/" \
            -e LITMUS_USERNAME="" \
            -e LITMUS_PASSWORD="${{ env.TOKEN }}" \
            owncloud/litmus:latest
```

**CouchDB service:** The existing litmus script (`scripts/webdav-litmus.sh`) requires CouchDB. The GitHub Actions job will need a CouchDB service container. Add `services: couchdb: image: couchdb:3 ports: ['5984:5984']`.

**Apt-package risk:** The `litmus` apt package on Ubuntu 22.04/24.04 is version 0.13 from 2014. It does NOT include the `http` test suite (added in 0.14). The current upstream HEAD (notroj, Dec 2024) is at 0.15.1+. Always use Docker or build from source in CI — never `apt install litmus`.

**Go reimplementation:** No mature Go reimplementation of litmus exists. The Python `davtest` tool covers some overlap but not PROPFIND/PROPPATCH/props suite. The existing Go integration tests (`gowebdav_integration_test.go`) cover the behavioral surface but are not a compliance tester replacement. Stick with upstream litmus binary.

---

### 6. Throughput/Latency Benchmark Recording

**Use `golang.org/x/perf/cmd/benchstat` for comparison, standard `testing.B` for benchmark authorship. No Prometheus or external metrics stack needed.**

Write `BenchmarkPUT_1MB`, `BenchmarkPUT_100MB`, `BenchmarkGET_1MB`, `BenchmarkPROPFIND_Depth1` in `web/webdav/bench_test.go` following standard `testing.B` patterns:

```go
func BenchmarkPUT_1MB(b *testing.B) {
    env := newWebdavTestEnv(b, nil) // testutil_test.go harness works with testing.TB
    data := bytes.Repeat([]byte("x"), 1024*1024)
    b.SetBytes(int64(len(data)))
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        err := gowebdav.NewClient(env.TS.URL+"/dav/files", "", env.Token).
            WriteStream(fmt.Sprintf("/bench-%d.bin", i), bytes.NewReader(data), 0644)
        require.NoError(b, err)
    }
}
```

Run with: `go test -run='^$' -bench=BenchmarkPUT -benchmem -count=10 ./web/webdav/ > bench-before.txt`

Compare with `benchstat`:

```bash
go install golang.org/x/perf/cmd/benchstat@latest
benchstat bench-before.txt bench-after.txt
```

`benchstat` is at `golang.org/x/perf` module (latest published April 9, 2026). It computes medians, 95% confidence intervals, and Mann-Whitney U-test significance, making A/B comparisons statistically sound.

**In CI:** Commit baseline `bench-baseline.txt` to the repo. In CI, run benchmarks and compare with `benchstat bench-baseline.txt bench-current.txt`. Fail the job if throughput regresses beyond a threshold (manual threshold enforcement — benchstat outputs text, CI script greps for "+" indicating regression).

**No Prometheus needed.** Prometheus is for production runtime metrics. For benchmark recording in a test suite, `testing.B` + `benchstat` is the standard Go approach. Prometheus adds infrastructure complexity (push gateway, scrape config) with no benefit at test-suite scale.

**`newWebdavTestEnv` compatibility:** The existing harness takes `testing.T`. For benchmarks, change the parameter to `testing.TB` (which both `*testing.T` and `*testing.B` satisfy). This is a one-line change to `testutil_test.go`.

---

### Summary: New Libraries for v1.2

| Concern | Tool | Source | New to go.mod? |
|---------|------|--------|----------------|
| Large file fixture generation | `io.LimitReader`, `bytes.Repeat` | stdlib | No |
| Streaming memory measurement | `runtime.ReadMemStats` | stdlib | No |
| Interrupted PUT simulation | `io.Pipe`, `net/http` | stdlib | No |
| Concurrent test fan-out | `sync.WaitGroup`, `-race` flag | stdlib | No |
| Multi-range GET assertion | `mime`, `mime/multipart`, raw `http.Client` | stdlib | No |
| Benchmark authoring | `testing.B` | stdlib | No |
| Benchmark comparison | `golang.org/x/perf/cmd/benchstat` | `golang.org/x/perf` (tool, not lib dep) | Tool only — `go install`, not `go get` |
| CI litmus | `owncloud/litmus` Docker image | Docker Hub / owncloud-ci GitHub | No Go dep — Docker image |

**Zero new Go module dependencies for v1.2 robustness testing.** All Go-level tooling is stdlib or already in `go.mod`. `benchstat` is a CLI tool installed separately, not imported. `owncloud/litmus` is a Docker image, not a Go package.

---

## v1.1 Stack (unchanged — included for reference)

*(Original content below — server implementation decisions from v1.1 research.)*

---

## Recommended Stack

### Decision Summary

**Write custom handlers using `encoding/xml` directly.** Do not delegate to `golang.org/x/net/webdav`'s `Handler`. Do not adopt `emersion/go-webdav` as the server layer. Use `studio-b12/gowebdav` for integration test clients only.

This is the prescriptive recommendation. The rationale is below.

---

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `encoding/xml` (stdlib) | Go 1.25 (bundled) | RFC 4918 XML marshalling / unmarshalling for PROPFIND and PROPPATCH | Zero dependency. The WebDAV XML surface is small and static (15-20 structs). Standard library `encoding/xml` handles namespaced XML correctly with struct tags. No external library needed. |
| `net/http` (stdlib) | Go 1.25 (bundled) | HTTP request / response plumbing | Already used via Echo. Custom handlers implement normal `echo.HandlerFunc` — no separate HTTP server. |
| `golang.org/x/net/webdav` | v0.50.0 (already in go.mod) | **XML type definitions only** — borrow `webdav.Property`, `webdav.DeadPropsHolder`, `webdav.ETager` interfaces for potential reuse | Already a transitive dependency via Echo. However: **only use the type definitions, not the `Handler`**. See "What NOT to Use" and the deep analysis below. |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/studio-b12/gowebdav` | v0.12.0 (Jan 2026) | WebDAV client — drives integration tests from a real client perspective | **Integration tests only.** Use `NewClient(url, user, pass)` with `NewPreemptiveAuth` for Bearer token tests. Supports `ReadDir`, `Read`, `WriteStream`, `MkdirAll`, `Rename`, `RemoveAll`, `Stat`. BSD 3-Clause. |
| `github.com/gavv/httpexpect/v2` | v2.16.0 (already in go.mod) | Raw HTTP integration test assertions for WebDAV-specific scenarios that `gowebdav` cannot express (e.g. exact XML property assertions, Depth header behaviour) | Complement to `gowebdav`. Use for PROPFIND XML shape verification, ETag header format tests, status code assertion for edge cases. |
| `github.com/stretchr/testify` | v1.11.1 (already in go.mod) | Unit test assertions for XML builder, path mapper, error translator | Standard assertion library already in project. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `litmus` (external WebDAV compliance tester) | Runs the authoritative WebDAV test suite against the live server | Run as a Docker container: `docker run --rm -e URL=http://host/dav/files nicowillis/litmus`. Catches compliance issues that hand-written tests miss. Run in CI on each PR. |
| `go test -run TestWebDAV ./web/webdav/` | Unit + integration tests in the WebDAV handler package | Co-locate tests in `web/webdav/webdav_test.go` following stack convention. Use in-memory VFS (`mem://`) for fast CI runs. |

---

## Deep Analysis: `golang.org/x/net/webdav`

### What it provides

`golang.org/x/net/webdav` (currently at v0.52.0 as of Mar 2026; cozy-stack has v0.50.0 in go.mod) exposes:

- `Handler` — a complete `http.Handler` that dispatches all WebDAV methods, manages locking, and calls a pluggable `FileSystem`.
- `FileSystem` interface — 5 methods: `Mkdir`, `OpenFile`, `RemoveAll`, `Rename`, `Stat`.
- `LockSystem` interface — in-memory (`NewMemLS`) or custom implementation.
- Optional interfaces: `ContentTyper`, `ETager`, `DeadPropsHolder` on the `File` type.
- Built-in: `Dir` (wraps OS dir), `NewMemFS` (in-memory).

### Can we use the Handler for Cozy?

**No. The `Handler` is not usable as-is.** Three blockers:

**Blocker 1 — The `FileSystem` interface maps to POSIX semantics, not Cozy's VFS API.**

The interface requires `OpenFile(ctx, name, flag, perm)` returning a `webdav.File` (which is `io.ReadWriteSeeker` + `Readdir`). Cozy's VFS exposes `CreateFile(doc, olddoc)` returning a `vfs.File` (an `io.WriteCloser`), and `Open(doc)` returning an `io.ReadCloser`. These are not mechanically compatible. Bridging them requires writing a complete adapter that implements `Readdir`, `Seek`, `WriteAt`, and other `os.File`-like methods — work that is larger than writing the handlers directly.

**Blocker 2 — The `Handler` requires `LockSystem` even when locking is not implemented.**

Passing `NewMemLS()` would let the handler proceed, but the handler then emits `DAV: 1, 2` capabilities and accepts LOCK/UNLOCK requests, handling them in-memory with no persistence. This creates a silent false positive: the server claims to support locking, clients rely on it, and locks vanish on restart. The project scope explicitly excludes locking.

Passing `nil` causes a panic in `Handler.ServeHTTP`. There is no "no-locking" option.

**Blocker 3 — Known open bug: MOVE without Overwrite header returns 412.**

Issue #66059, opened March 2024, still open November 2025 (last checked): `handleCopyMove` uses `r.Header.Get("Overwrite") == "T"` for MOVE (returns `true` only when header is explicitly `"T"`), instead of the RFC 4918-correct `r.Header.Get("Overwrite") != "F"` (returns `true` when absent or `"T"`). macOS Finder does not send the Overwrite header. MOVE renames fail on Finder without this fix. The PITFALLS research confirmed: this bug is present as of April 2026 and there is no fix.

Also present: issue #66085 (WriteHeader called twice in PROPFIND), issue #44492 (PUT ignores If-* conditional headers), issue #44493 (ETags in `If` header conditions ignored). These are not critical blockers individually, but together they indicate the handler is not production-ready for a correctness-sensitive implementation.

### Can we use it partially (XML types only)?

Possibly, but there is little benefit. The XML types in `x/net/webdav` are internal to the package (not exported as public API structs). The exported surface is only the `Handler`, `FileSystem`, and `LockSystem` interfaces plus optional helper interfaces. The XML serialization/deserialization is not exposed as reusable components.

**Verdict: Use `x/net/webdav` for zero things.** The module is already a transitive dependency so it does not add to go.mod, but importing the webdav sub-package to use only its interfaces is not worth the coupling to a package with known open bugs.

---

## Deep Analysis: `emersion/go-webdav`

### What it provides

`emersion/go-webdav` v0.7.0 (Oct 2025) exposes:

- A `FileSystem` interface with 8 methods (see below), notably including `Copy` and `Move` as first-class operations — more appropriate for non-POSIX backends.
- `Handler` struct implementing `http.Handler`.
- `FileInfo` struct with `Path`, `Size`, `ModTime`, `IsDir`, `MIMEType`, `ETag`.
- `MoveOptions` / `CopyOptions` structs with `NoOverwrite` field.
- Context threading throughout — all methods accept `context.Context`.

```go
type FileSystem interface {
    Open(ctx context.Context, name string) (io.ReadCloser, error)
    Stat(ctx context.Context, name string) (*FileInfo, error)
    ReadDir(ctx context.Context, name string, recursive bool) ([]FileInfo, error)
    Create(ctx context.Context, name string, body io.ReadCloser, opts *CreateOptions) (*FileInfo, bool, error)
    RemoveAll(ctx context.Context, name string, opts *RemoveAllOptions) error
    Mkdir(ctx context.Context, name string) error
    Copy(ctx context.Context, name, dest string, options *CopyOptions) (bool, error)
    Move(ctx context.Context, name, dest string, options *MoveOptions) (bool, error)
}
```

### Is the interface a better fit for Cozy VFS?

Better than `x/net/webdav`, but still not a clean match:

- `Open` returning `io.ReadCloser` matches `vfs.Open(doc)` closely.
- `Create` receiving `io.ReadCloser` and returning `*FileInfo` does not match `vfs.CreateFile(newdoc, olddoc)` which returns a `vfs.File` (writer). Would need an adapter that buffers or pipes.
- `ReadDir` with `recursive bool` forces recursive listing into the interface. For Cozy, `Depth: infinity` is explicitly rejected (PITFALLS pitfall 1). The adapter would call `DirBatch` repeatedly for paged listing and ignore `recursive=true`.
- `Move` and `Copy` accept `*MoveOptions` with `NoOverwrite` — cleaner than x/net's opaque flag.

### Does emersion/go-webdav have the Overwrite bug?

The server.go passes the `overwrite` bool through `MoveOptions{NoOverwrite: !overwrite}` to the backend. The internal header parsing (in the `internal` package) could not be directly inspected, but the package passes `overwrite` as a parsed boolean — the question is what value it takes when the header is absent. Given the library is newer and more actively maintained than x/net/webdav, and the issue #66059 has been widely discussed, there is a reasonable expectation that emersion parses it correctly — but this is MEDIUM confidence (unverified against raw source). **Do not assume RFC compliance without writing a test.**

### Why not use emersion/go-webdav as the server Handler?

Three reasons:

1. **The adapter layer is still substantial.** 8 methods must be bridged from Cozy VFS semantics. For `Create`, you need an in-memory buffer or io.Pipe between `body io.ReadCloser` and `vfs.CreateFile`'s `io.WriteCloser` — non-trivial for large files. Writing the handlers directly avoids this impedance.

2. **The Handler owns the XML generation.** If we find an RFC compliance issue in emersion's XML (e.g., wrong ETag quoting, RFC 1123 date format, property namespace handling), we cannot fix it without forking the library. Writing our own `xml.go` gives us full control.

3. **Cozy has non-standard properties to expose.** The `pkg/webdav` client already parses `oc:fileid`, `nc:trashbin-filename`, `nc:trashbin-original-location` from Nextcloud responses. If we eventually want to expose Cozy-specific DAV properties (file ID, sharing state), a custom XML layer trivially supports this. emersion's handler requires implementing `DeadPropsHolder` and navigating its internal property dispatch.

**Verdict: Do not use emersion/go-webdav as the server layer.** It is a better-designed library than `x/net/webdav` and worth watching for a future locking implementation (v2). For now, write custom handlers.

---

## Recommended Architecture: Custom Handlers + stdlib XML

### Why custom handlers win

The WebDAV method surface for our scope (8 methods: OPTIONS, PROPFIND, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE) is manageable. The RFC 4918 XML for PROPFIND 207 Multi-Status is a small, well-defined schema. The known pitfalls (Overwrite default, ETag quoting, RFC 1123 dates, Content-Length, Depth:infinity rejection) are all one-line fixes that are invisible when hidden inside a third-party handler.

Implementing custom handlers means:

- Full control over every RFC compliance detail.
- No impedance mismatch: handlers call `inst.VFS()` directly using the exact API it exposes.
- No forking needed when bugs need fixing.
- Echo integration is trivial: `e.Match(webdavMethods, "/dav/files/*", handler)` using `echo.WrapHandler` or direct `echo.HandlerFunc`.
- The XML structs fit in ~100 lines of Go.

### Echo v4 Integration Pattern

Echo v4 (v4.15.1 in cozy-stack) supports arbitrary HTTP method registration via `e.Match()`. The pattern already established in `web/routing.go` for static assets uses `echo.WrapHandler`. For WebDAV, use `echo.HandlerFunc` directly to stay in the Echo context:

```go
// In web/routing.go — SetupRoutes():
webdavMethods := []string{
    "OPTIONS", "PROPFIND", "GET", "HEAD",
    "PUT", "DELETE", "MKCOL", "COPY", "MOVE",
}
dav := router.Group("/dav/files", middlewares.NeedInstance, webdav.ResolveWebDAVAuth)
dav.Match(webdavMethods, "", webdav.HandleRoot)
dav.Match(webdavMethods, "/*path", webdav.HandlePath)

// Nextcloud-compatible redirect
router.GET("/remote.php/webdav", func(c echo.Context) error {
    return c.Redirect(http.StatusMovedPermanently, "/dav/files")
})
router.Match(webdavMethods, "/remote.php/webdav/*path", func(c echo.Context) error {
    newPath := strings.Replace(c.Request().URL.RequestURI(), "/remote.php/webdav", "/dav/files", 1)
    return c.Redirect(http.StatusMovedPermanently, newPath)
})
```

Each handler is a standard `echo.HandlerFunc`:

```go
func HandlePath(c echo.Context) error {
    switch c.Request().Method {
    case "PROPFIND":
        return PropFind(c)
    case "GET", "HEAD":
        return GetHead(c)
    // ...
    }
    return echo.ErrMethodNotAllowed
}
```

No `x/net/webdav.Handler` intermediary. No `http.Handler` wrapping needed. Middleware chain (NeedInstance, auth) runs natively on Echo before the handler is reached.

### XML Implementation Plan

Define RFC 4918 structs in `web/webdav/xml.go`:

```go
// ~90 lines total for PROPFIND response XML
type Multistatus struct {
    XMLName   xml.Name   `xml:"DAV: multistatus"`
    Responses []Response `xml:"DAV: response"`
}

type Response struct {
    Href     string    `xml:"DAV: href"`
    Propstat []Propstat `xml:"DAV: propstat"`
}

type Propstat struct {
    Prop   Prop   `xml:"DAV: prop"`
    Status string `xml:"DAV: status"`
}

type Prop struct {
    ResourceType  *ResourceType `xml:"DAV: resourcetype,omitempty"`
    DisplayName   string        `xml:"DAV: displayname,omitempty"`
    GetLastModified string      `xml:"DAV: getlastmodified,omitempty"` // MUST use http.TimeFormat
    GetETag       string        `xml:"DAV: getetag,omitempty"`          // MUST be quoted: `"abc123"`
    GetContentLength int64      `xml:"DAV: getcontentlength,omitempty"`
    GetContentType string       `xml:"DAV: getcontenttype,omitempty"`
}

type ResourceType struct {
    Collection *struct{} `xml:"DAV: collection,omitempty"`
}
```

For PROPFIND request parsing:

```go
type PropFind struct {
    XMLName  xml.Name  `xml:"DAV: propfind"`
    AllProp  *struct{} `xml:"DAV: allprop"`
    PropName *struct{} `xml:"DAV: propname"`
    Prop     *Prop     `xml:"DAV: prop"`
}
```

Total XML surface: ~150 lines for a correct, tested implementation.

---

## Integration Tests: Client Library Recommendation

### Recommended: `studio-b12/gowebdav` v0.12.0

**Use for:** Black-box integration tests that exercise the server from the perspective of a real WebDAV client. Tests are written TDD-style: instantiate a `gowebdav.Client` against the `httptest.Server`, exercise `ReadDir`, `WriteStream`, `Rename`, `Copy`, `Remove`, then assert the VFS state directly via `inst.VFS()`.

```go
// In web/webdav/webdav_test.go
func TestPropfindDepth1(t *testing.T) {
    ts := setupTestServer(t) // spins up Echo + in-mem VFS
    c := gowebdav.NewAuthClient(ts.URL+"/dav/files", gowebdav.NewPreemptiveAuth(
        gowebdav.NewBearerAuth(testToken),
    ))
    files, err := c.ReadDir("/")
    require.NoError(t, err)
    assert.Len(t, files, expectedDirCount)
}
```

Installation: `go get github.com/studio-b12/gowebdav@v0.12.0`

Note: `gowebdav` does not yet have a stable Bearer token authenticator in v0.12.0 (work in progress per docs). Workaround: use `NewPreemptiveAuth` with a custom `Authorize` that sets `Authorization: Bearer <token>` directly, or use Basic Auth with the token as the password (which is the primary auth mode for Cozy anyway).

### Complement: `github.com/gavv/httpexpect/v2` (already in go.mod)

**Use for:** Tests that must assert exact HTTP-level behaviour that `gowebdav` abstracts away:

- Exact XML property values in PROPFIND responses (ETag format, date format, namespace).
- Depth header rejection (verify 403 on `Depth: infinity`).
- Conditional header tests (If-Match, If-None-Match exact status codes).
- OPTIONS response headers (`DAV: 1`, `Allow:` list).
- Content-Length presence on GET responses.

Already in go.mod at v2.16.0. No new dependency needed.

### Do NOT use: raw HTTP with custom XML (what `pkg/webdav` uses)

The existing `pkg/webdav` client is a custom HTTP client for talking to external Nextcloud servers. Do not copy its approach for the new server-side tests. It lacks Rename, Copy, Stat and the full method set. The `gowebdav` library is more complete and purpose-built for client-side testing.

### Do NOT use: emersion/go-webdav client

The emersion library does provide a WebDAV client, but it is focused on the CalDAV/CardDAV ecosystem. Its WebDAV client surface is narrower than `studio-b12/gowebdav` for the plain file operations we need. Adding it as a dependency for tests is not justified when `gowebdav` covers the same ground.

---

## Installation

```bash
# Test client only — no new server-side dependencies needed
go get github.com/studio-b12/gowebdav@v0.12.0
```

The server implementation uses only:
- `encoding/xml` — stdlib
- `net/http` — stdlib
- `path` — stdlib
- `net/url` — stdlib
- Echo v4 — already in go.mod
- `golang.org/x/net` — already in go.mod (no version bump needed; webdav sub-package unused)

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Custom handlers + stdlib XML | `golang.org/x/net/webdav` Handler | If Cozy's VFS were POSIX-compatible (`os.FileInfo`, `os.File`), and if the MOVE Overwrite bug and lock coupling were acceptable. Neither condition holds. |
| Custom handlers + stdlib XML | `emersion/go-webdav` Handler | If we needed CalDAV or CardDAV in the same codebase (it is the best choice for that), or if the project had a strictly POSIX-adjacent VFS. Revisit for a locking v2 phase. |
| Custom handlers + stdlib XML | Fork of `x/net/webdav` | If the intent was to upstream fixes. Not worth the maintenance burden for a private monolith. |
| `studio-b12/gowebdav` (test client) | `emersion/go-webdav` client | If the test suite needed CalDAV/CardDAV client operations in the same test binary. Not applicable here. |

---

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `golang.org/x/net/webdav.Handler` | LockSystem coupling, known MOVE Overwrite bug (#66059, open as of April 2026), POSIX-only FileSystem interface, WriteHeader-called-twice bug (#66085) | Custom `echo.HandlerFunc` handlers |
| `emersion/go-webdav` as server layer | Adapter boilerplate for 8-method VFS bridge, loss of XML control for custom properties, unverified Overwrite handling | Custom handlers + stdlib XML |
| CouchDB `_rev` as ETag | Semantically wrong: changes on metadata edits, not content changes; reveals internal structure | File content MD5 (`vfs.FileDoc.MD5Sum`) for files; hash of dir `updated_at` + `_rev` for directories |
| `time.RFC3339` for date properties | Breaks macOS Finder (requires RFC 1123 / `http.TimeFormat`) | `t.UTC().Format(http.TimeFormat)` — Go stdlib constant |
| Direct `x/net/webdav.NewMemLS()` for tests | Implicitly enables locking in Handler, creating false `DAV: 2` capability claim | No locking at all — set `DAV: 1` in OPTIONS response |
| `apt install litmus` in CI | Ubuntu package is litmus 0.13 (2014), missing `http` suite added in 0.14; version drift vs upstream notroj/litmus (0.15+, Dec 2024) | `owncloud/litmus` Docker image or build from `notroj/litmus` source |
| `Shopify/toxiproxy` for interruption tests | Overkill for in-process httptest tests; requires running a separate proxy process; adds Go module dependency | `io.Pipe` with `CloseWithError` on the write end |
| Binary fixture files for large file tests | Bloats the repo (git history cannot shrink), slows clones, CI disk waste | `io.LimitReader` over a synthesized `bytes.Reader` seed |

---

## Version Compatibility

| Package | Version | Compatibility Notes |
|---------|---------|---------------------|
| `golang.org/x/net` | v0.50.0 (current in go.mod) | The `webdav` sub-package is NOT imported by the implementation. No version bump needed. |
| `github.com/studio-b12/gowebdav` | v0.12.0 (Jan 2026) | Already in go.mod. `WriteStreamWithLength` available. Go 1.21+ required (satisfied). |
| `github.com/labstack/echo/v4` | v4.15.1 | `e.Match()` for custom HTTP methods is available since Echo v4.5.0. No issue. |
| `github.com/gavv/httpexpect/v2` | v2.16.0 (already in go.mod) | No change needed. |
| `golang.org/x/perf/cmd/benchstat` | latest (April 2026) | CLI tool only — `go install golang.org/x/perf/cmd/benchstat@latest`. Not imported as a library. |
| `owncloud/litmus` Docker image | latest tag | Last source update May 2024. Acceptable for CI; fall back to building from `notroj/litmus` if image stagnates. |
| `testing/synctest` | Go 1.25 (stable) | Available without GOEXPERIMENT in Go 1.25+. Optional for unit-level concurrency tests. |

---

## Sources

**v1.2 addendum sources:**
- [pkg.go.dev/github.com/studio-b12/gowebdav](https://pkg.go.dev/github.com/studio-b12/gowebdav) — v0.12.0, `ReadStreamRange` single-range only, `WriteStreamWithLength` available. HIGH confidence (direct pkg.go.dev inspection).
- [pkg.go.dev/runtime/metrics](https://pkg.go.dev/runtime/metrics) — `/memory/classes/heap/live:bytes`, `/memory/classes/total:bytes` metrics stable Go 1.16+. HIGH confidence.
- [pkg.go.dev/runtime](https://pkg.go.dev/runtime) — `ReadMemStats`, `MemStats.HeapInuse` for in-process heap measurement. HIGH confidence.
- [go.dev/src/net/http/fs.go](https://go.dev/src/net/http/fs.go) — `ServeContent` multi-range / `multipart/byteranges` support. Issue #3784 fixed 2012-06-29. HIGH confidence.
- [github.com/golang/go/issues/3784](https://github.com/golang/go/issues/3784) — Multi-range ServeContent, resolved. HIGH confidence.
- [pkg.go.dev/golang.org/x/perf/cmd/benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) — Current version April 9, 2026. Mann-Whitney U-test, 95% CI. HIGH confidence.
- [github.com/owncloud-ci/litmus](https://github.com/owncloud-ci/litmus) — Docker image for litmus, last source update May 2024. `LITMUS_URL/USERNAME/PASSWORD` env vars. MEDIUM confidence (maintenance pace unclear).
- [github.com/notroj/litmus](https://github.com/notroj/litmus) — Official upstream litmus, December 2024 commit, has Dockerfile. HIGH confidence.
- [github.com/workgroupengineering/litmus](https://github.com/workgroupengineering/litmus) — GitHub Action wrapper, single commit Sep 2021, abandoned. HIGH confidence (directly inspected).
- [go.dev/blog/synctest](https://go.dev/blog/synctest) — `testing/synctest` graduated to stable in Go 1.25. HIGH confidence.
- Datadog Go memory regression post (2025) — `HeapInuse` vs RSS distinction. MEDIUM confidence (secondary source, but technically accurate).

**v1.1 original sources:**
- [pkg.go.dev/golang.org/x/net/webdav](https://pkg.go.dev/golang.org/x/net/webdav) — FileSystem interface, Handler struct, current version v0.52.0. HIGH confidence.
- [golang/go issue #66059](https://github.com/golang/go/issues/66059) — MOVE Overwrite header bug, open as of November 2025. HIGH confidence.
- [golang/go issue #66085](https://github.com/golang/net/blob/master/webdav/webdav.go) — WriteHeader called twice in PROPFIND. HIGH confidence (issue tracker).
- [golang/go issue #44492](https://github.com/golang/go/issues/44492) — PUT ignores If-* conditional headers. HIGH confidence.
- [golang/net webdav.go source](https://github.com/golang/net/blob/master/webdav/webdav.go) — Confirmed COPY uses `!= "F"`, MOVE uses `== "T"`. HIGH confidence.
- [pkg.go.dev/github.com/emersion/go-webdav](https://pkg.go.dev/github.com/emersion/go-webdav) — v0.7.0 Oct 2025, FileSystem interface shape. HIGH confidence.
- [deepwiki.com/emersion/go-webdav](https://deepwiki.com/emersion/go-webdav/2.1-webdav-server) — Server architecture, adapter pattern. MEDIUM confidence (secondary source).
- [Echo issue #1459](https://github.com/labstack/echo/issues/1459) — WebDAV method routing in Echo, `e.Match()` workaround confirmed. HIGH confidence.
- [cozy-stack go.mod](go.mod) — Confirmed `golang.org/x/net v0.50.0` already present. HIGH confidence (direct inspection).

---

*Stack research for: WebDAV server layer on cozy-stack (Go monolith)*
*v1.1 researched: 2026-04-04*
*v1.2 addendum: 2026-04-12*

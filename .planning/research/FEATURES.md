# Feature Research — v1.2 WebDAV Robustness

**Domain:** WebDAV server robustness beyond RFC 4918 Class 1 litmus compliance
**Researched:** 2026-04-12
**Confidence:** HIGH (large-files/streaming), HIGH (interrupted-PUT), MEDIUM (Nextcloud chunked), HIGH (multi-range GET), HIGH (concurrency), MEDIUM (CI litmus), MEDIUM (iOS client matrix)

---

## Scope Reminder

v1.1 shipped litmus 63/63, streaming PUT via `io.Copy`, single-range GET via `http.ServeContent`. v1.2 targets real-world robustness that litmus does NOT exercise: multi-GB transfers, connection drops, concurrent writes, and formal client sign-off.

---

## Feature Landscape

### Table Stakes (Users Expect These)

Features that a real WebDAV deployment must handle. Missing these = complaints from real clients (rclone, iOS) or data loss.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Large-file streaming PUT (multi-GB, provably bounded memory)** | rclone, iOS Files, and Nextcloud desktop clients routinely upload 2–5 GB files. A server that OOMs or times out on large uploads is broken for production use. | MEDIUM | `io.Copy` in `put.go` already streams (no full-buffer), but memory ceiling under load is **unmeasured**. Need benchmark: upload a 1 GB file and profile goroutine heap. Expected ceiling: < 32 MB RSS delta (only read-buffer overhead). Go's `io.Copy` uses 32 KB internal buffers; VFS Swift wraps that in an `ObjectPut` pipe. The critical variable is whether cozy-stack or Echo buffers the request body upstream of our handler — investigate `echo.Binder` and middleware. |
| **Large-file streaming GET (multi-GB, 206 range serviced without full-read into RAM)** | Video players, rclone, and backup tools issue Range requests for sub-segments of large files. | LOW | `vfs.ServeFileContent` delegates to `http.ServeContent`, which opens a `io.ReadSeeker` and seeks rather than buffering. Already correct. Need E2E test with a >= 500 MB file to confirm no regression. |
| **Interrupted PUT — no partial file visible to other clients** | RFC 4918 does not specify this, but de facto convention among all major implementations (sabre/dav, Apache mod_dav, ownCloud) is that an aborted PUT must leave the VFS in a consistent state: either the previous file (for overwrites) or no file (for creates). Partial files visible after a connection drop are a data-corruption bug. | MEDIUM | The cozy VFS Swift implementation's `Close()` already handles this: on error it calls `ObjectDelete` (removes the partial Swift object) and `DeleteFileDoc` (removes the new CouchDB entry). For overwrites, `olddoc` is retained until `Close()` succeeds. **Behavior is correct by design** — but it is not tested under simulated connection-drop conditions. Need a test that cancels the request context mid-transfer and asserts no partial file appears in PROPFIND. |
| **Deterministic concurrent write semantics (If-Match + 412)** | When two clients race to write the same file, the outcome must be deterministic and not silently lose either write. The de facto standard (enforced by CouchDB MVCC) is optimistic concurrency: the second writer gets a 412 if it has a stale ETag, or last-write-wins if no ETag is asserted. | MEDIUM | `put.go` already calls `checkETagPreconditions` on overwrite. VFS uses CouchDB `_rev` which enforces sequential commit. The question is whether two concurrent writes *without* `If-Match` headers can corrupt each other. They can race at the CouchDB level, but CouchDB MVCC guarantees one wins with a valid document revision and the other gets a conflict error (which surfaces as a 500 today — should map to 409/503 with a clear message). Need a test: two goroutines PUT to the same path simultaneously, assert both get back a valid status (one 204, one 4xx) and the file is not corrupt. |
| **Multi-range GET (RFC 7233 multipart/byteranges)** | Some backup tools and HTTP/1.1-aware clients coalesce multiple range requests into one. RFC 7233 requires servers to respond with `multipart/byteranges` when multiple ranges are requested. | LOW | **Already handled for free.** `vfs.ServeFileContent` delegates to `http.ServeContent`. Go issue #3784 (multi-range support) was fixed and released in Go 1.1+. The project targets Go 1.25. Multi-range GET requires zero implementation work. Validate with a test asserting that `Range: bytes=0-9,20-29` returns `206 multipart/byteranges`. |
| **CI litmus integration (GitHub Actions)** | A compliance regression discovered only when manually running `make test-litmus` can ship unnoticed. Automated CI is the professional baseline. | LOW | The existing `go-tests.yml` workflow installs CouchDB manually. Adding `apt install litmus` and a start/stop for a real cozy-stack instance adds ~3 min to CI. The `owncloudci/litmus` Docker image (env vars `LITMUS_URL`, `LITMUS_USERNAME`, `LITMUS_PASSWORD`) offers an alternative with no host dependency on the litmus binary. The existing `scripts/webdav-litmus.sh` is reusable if we can start cozy-stack as a service step. |

### Differentiators (Nice-to-Have, Not Blocking)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **iOS Files app formal sign-off** | Confirmed working on the primary consumer iOS client. Required for any "production ready" claim. | LOW (effort) / HIGH (device dependency) | iOS Files app does not expose WebDAV natively — it requires Pages/Numbers/Keynote built-in WebDAV browser or a third-party file-provider extension. The practical test is adding cozy as a WebDAV server from the iOS Files app via a compatible third-party app (FileBrowser Pro, WebDAV Navigator). Manual checklist: connect (HTTPS required in prod), PROPFIND root, upload file, download file, rename (MOVE), delete, create folder (MKCOL). |
| **Memory proof by measurement (benchmark with heap profiling)** | Converts "we stream correctly" from assertion to evidence, enables regression detection in CI. | LOW | Add a `BenchmarkPUT_1GB` that streams 1 GB of zero-bytes and calls `runtime.ReadMemStats` before/after, failing if delta > threshold (e.g. 64 MB). sabre/dav asserts 15 GB support with "no elevated memory_limit" — we should make the same claim with evidence. |
| **Early 413 on quota-exceeded when Content-Length is known** | Clients benefit from an early 413 before the full body is read when the upload would exceed quota. | MEDIUM | Currently the VFS returns quota errors only at `file.Close()` time. If `Content-Length` is known, pre-check against remaining quota before starting the upload. Risk: TOCTOU window. Deferred unless quota-check speed becomes a complaint. |

### Anti-Features (Explicitly Should NOT Add)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Nextcloud chunked upload protocol** (`/remote.php/dav/uploads/{user}/{upload-id}/{chunk-id}`) | Enables resumable large-file uploads; Nextcloud Desktop Sync client requires it. | (1) Nextcloud-proprietary, not RFC 4918. (2) Requires a new URL tree (`/uploads/`), new MKCOL/PUT/MOVE semantics for temp assembly dirs, a 24-hour cleanup job, and quota pre-checks via `OC-Total-Length`. Chunk size constraints: 5 MB–5 GB per chunk. (3) Only helps Nextcloud-dialect clients: Nextcloud Desktop Sync, Nextcloud iOS/Android app, rclone in `--webdav-vendor nextcloud` mode. Pure WebDAV clients (rclone default, iOS Files, OnlyOffice) already work fine with single streaming PUT. (4) Nextcloud Desktop Sync also requires OCS API capabilities negotiation, which is already explicitly out of scope. | Serve large files via single streaming PUT. Document the ceiling. Defer to a future Nextcloud API compatibility milestone if OCS API is ever scoped. |
| **Content-Range PUT (partial/resumable PUT)** | Some FTP-migration tools and older clients use `Content-Range` on PUT to resume interrupted uploads. | RFC 7231 §4.3.4 explicitly states a server SHOULD respond 400 to a PUT with `Content-Range`. Apache mod_dav supports it as pre-RFC 7231 legacy. ownCloud explicitly refused to implement it (issue #1051). Lighttpd has it behind a `deprecated-unsafe-partial-put` flag disabled by default. Adding it creates correctness ambiguity (metadata, ETag, quota), opens partial-file visibility windows. Not required by any modern client in scope. | Return 400 Bad Request when `Content-Range` is present on PUT. Document this. |
| **WebDAV LOCK / Class 2 compliance** | macOS Finder requires LOCK for writes; some editors use it. | Already out of scope in PROJECT.md. LOCK requires per-resource state, distributed lock manager, timeout handling, and client token management. VFS has no lock infrastructure. | Document the limitation. macOS Finder remains read-only. |

---

## Feature Deep-Dives by Category

### Category 1: Large Files

**What major implementations do:**

- **sabre/dav (PHP):** Uses PHP streams throughout, tested to 15 GB, requires `output_buffering=off` to prevent PHP runtime from buffering. Memory consumption is proportional to buffer sizes, not file size. Source: [sabre.io/dav/large-files/](https://sabre.io/dav/large-files/)
- **Nextcloud/ownCloud:** Large file support is primarily a PHP/nginx configuration story. `upload_max_filesize` does not apply to WebDAV PUT — only PHP timeout matters. Apache 2.4.54+ changed `LimitRequestBody` default from unlimited to 1 GB. Source: [Nextcloud admin docs](https://docs.nextcloud.com/server/stable/admin_manual/configuration_files/big_file_upload_configuration.html)
- **rclone WebDAV backend:** Had a 16 GB OOM bug caused by the `go-ntlmssp` auth library buffering the entire request body in RAM — not the WebDAV layer itself. Root cause: auth middleware, not transfer logic.

**Implication for cozy-stack:** The risk is not in `put.go` (which uses `io.Copy` correctly) but in middleware layers above it: Echo body limits, `http.MaxBytesReader`, or any middleware that reads/copies the request body before the handler sees it. This must be verified by profiling, not assumed.

**LOC estimate:** ~100 LOC (benchmark test + memory assertion). Zero production code change expected if middleware is clean.

---

### Category 2: Interrupted PUT

**De facto conventions across implementations:**

- **sabre/dav:** PHP streams; connection drop triggers PHP error handler; partial temp files cleaned up. VFS transaction not committed.
- **Apache mod_dav:** Temp file + atomic rename. Connection drop = temp file deleted, destination untouched.
- **reva (CS3):** Flagged as a known bug ([cs3org/reva#86](https://github.com/cs3org/reva/issues/86)). Without a transaction, partial uploads require explicit cleanup hooks. Consensus: atomic If-Match enforcement at storage finalization step.
- **cozy-stack VFS Swift (`impl_v3.go`):** `Close()` deferred cleanup: on error, calls `c.ObjectDelete` (removes partial Swift object) + `DeleteFileDoc` (removes new CouchDB index entry). For overwrites: `olddoc` is preserved until `Close()` succeeds. This is the correct "atomic commit" pattern.

**What needs testing:** Does `Close()` get called when the client drops the HTTP connection mid-transfer? In Go/Echo, if `c.Request().Body.Read()` returns an error (connection reset), `io.Copy` returns that error, and `put.go` calls `file.Close()` in the error path — yes, `Close()` is always called (see `put.go` lines 107–112). The partial cleanup will execute. But this is currently untested behavior.

**LOC estimate:** ~80 LOC (integration test using `net.Pipe` to simulate connection drop mid-stream + PROPFIND assertion). Zero production code change expected.

---

### Category 3: Content-Range PUT and Nextcloud Chunked Upload

**RFC 7231 stance:** §4.3.4 says Content-Range in PUT is an error; server SHOULD return 400. Not ambiguous.

**Ecosystem support matrix:**

| Server | Content-Range PUT | Notes |
|--------|-------------------|-------|
| Apache mod_dav | Yes | Legacy RFC 2616 behavior |
| Nginx | No (501) | |
| Lighttpd | Optional (`deprecated-unsafe-partial-put`, default off) | |
| sabre/dav | No | |
| ownCloud | No | Explicitly refused, issue #1051 |
| cozy-stack v1.1 | Not rejected (ambiguous) | Should return 400 |

**Nextcloud v2 chunked upload protocol (as of NC 15+):**

1. `MKCOL /remote.php/dav/uploads/{user}/{uuid}` with `Destination` header pointing to final path
2. `PUT /remote.php/dav/uploads/{user}/{uuid}/{chunk-number}` — chunks numbered 1–10000, 5 MB–5 GB each; `OC-Total-Length` header enables quota pre-check
3. `MOVE /remote.php/dav/uploads/{user}/{uuid}/.file` — assembles and moves to destination; `X-OC-Mtime` header sets modification time
4. Abort: `DELETE /remote.php/dav/uploads/{user}/{uuid}`
5. Auto-cleanup of stale upload dirs after 24 hours

Source: [Nextcloud Developer Docs — Chunked file upload](https://docs.nextcloud.com/server/stable/developer_manual/client_apis/WebDAV/chunking.html)

**Who uses Nextcloud chunked upload?** Only Nextcloud-dialect clients: Nextcloud Desktop Sync, Nextcloud iOS/Android native app, rclone `--webdav-vendor nextcloud`. Pure WebDAV clients (rclone default, iOS Files, OnlyOffice) do NOT use it.

**Decision:** Explicitly return `400 Bad Request` if `Content-Range` is present on PUT (~10 LOC in `put.go`). Do not implement Nextcloud chunked upload.

---

### Category 4: Multi-Range GET

**Client usage reality:** Most WebDAV clients do not use multi-range GET. HTTP accelerators and some backup tools may coalesce ranges. RFC 7233 §4.1 states clients that cannot process `multipart/byteranges` MUST NOT request multiple ranges — this self-limits deployment.

**Go's status:** `http.ServeContent` has supported multi-range GET (`multipart/byteranges`) since Go 1.1 ([issue #3784 — closed/fixed](https://github.com/golang/go/issues/3784)). Project uses Go 1.25. `vfs.ServeFileContent` calls `http.ServeContent(w, req, filename, doc.UpdatedAt, content)` directly. Multi-range GET is already supported with zero additional implementation work.

**LOC estimate:** ~30 LOC (test asserting `Range: bytes=0-9,20-29` returns `206 multipart/byteranges` with correct boundary and part bodies). Zero production code change needed.

---

### Category 5: Concurrent Writes

**Protection mechanisms in cozy-stack:**

1. **CouchDB MVCC:** Every file update requires a correct `_rev`. Two concurrent writers compete; CouchDB guarantees one wins with a valid document and the other gets a 409 Conflict from CouchDB. The VFS currently maps this to a 500 — should be a 409 or 503.
2. **VFS Swift `mu` lock (`ErrorRWLocker`):** `impl_v3.go` uses a distributed lock around all CouchDB index mutations. Two concurrent PUTs to the same file will serialize at the VFS lock — one proceeds, the other waits, then sees the updated document.
3. **WebDAV `If-Match`:** `put.go` checks ETag preconditions on overwrite. A client with a stale ETag after another client updated the file receives `412 Precondition Failed`. This is correct WebDAV behavior.

**Gap:** Without `If-Match`, it is last-write-wins (serialized by VFS lock). This is the same behavior as Apache mod_dav, sabre/dav, and ownCloud. WebDAV does not require LOCK for concurrency — LOCK is optional Class 2 behavior.

**LOC estimate:** ~80 LOC (concurrent write test: two goroutines, assert no data corruption + both get valid HTTP status). ~20 LOC production fix to map CouchDB 409 conflicts to HTTP 409/503 instead of 500.

---

### Category 6: CI Litmus Integration

**Existing infrastructure:**
- `scripts/webdav-litmus.sh`: starts instance, gets token, runs litmus, destroys instance. Works locally.
- `.github/workflows/go-tests.yml`: Ubuntu 22.04, installs CouchDB from apt. Does not start cozy-stack as a service.

**Integration options:**

1. **`apt install litmus` in the workflow + start `cozy-stack serve` as background process.** Litmus is in Ubuntu 22.04 apt as version 0.13. The existing script handles instance create/destroy. Requires building the stack binary and starting it with a minimal config.
2. **`owncloudci/litmus` Docker image** ([owncloud-ci/litmus](https://github.com/owncloud-ci/litmus)): env vars `LITMUS_URL`, `LITMUS_USERNAME`, `LITMUS_PASSWORD`, `LITMUS_TIMEOUT`. No host binary dependency. Requires Docker services or `host` network mode to reach `localhost`.
3. **Dedicated job `litmus` in a new workflow file** that: (a) installs `litmus` via apt, (b) builds and starts `cozy-stack serve` in background, (c) runs `scripts/webdav-litmus.sh`, (d) kills the stack. Only runs on one matrix row (Go 1.25 + CouchDB 3.3.3 is sufficient).

**Recommendation:** Option 3 — new `webdav-litmus.yml` workflow, `apt install litmus`, build + start cozy-stack. Reuses existing script. Avoids Docker-in-Docker. Estimated time overhead: +4–6 min per run.

**LOC estimate:** ~60 YAML lines (new workflow file). A minimal cozy-stack CI config template (~20 lines) may also be needed.

---

### Category 7: iOS Files App Client Matrix

**iOS Files native WebDAV support:**
- iOS Files app (iOS 11+) "Connect to Server" dialog supports SMB. WebDAV is **not directly exposed** from the native Files app without a third-party file provider extension.
- Pages, Numbers, and Keynote have a built-in WebDAV browser that works natively.
- Third-party apps (FileBrowser Pro, WebDAV Navigator, WebDAV Manager) register as file provider extensions and appear in the iOS Files sidebar.

**Practical test approach for v1.2:**

1. **Keynote/Pages WebDAV test** — Open Keynote, add WebDAV server (`https://{instance}/dav/files/`), browse, open a file. Tests PROPFIND + GET without requiring a third-party app.
2. **FileBrowser Pro test** — Add cozy as WebDAV server, navigate, upload, download, rename, delete. Tests the full CRUD matrix.
3. **WebDAV Navigator** — Simpler client, tests basic connectivity and browsing.

**Manual test checklist (per client):**
- [ ] Connect with Basic Auth (token-as-password)
- [ ] PROPFIND root — directory listing loads
- [ ] PROPFIND subdirectory — nested listing loads
- [ ] GET (download file) — content correct
- [ ] PUT small file (< 10 MB) — visible in PROPFIND after upload
- [ ] PUT large file (>= 100 MB) — no timeout, content correct
- [ ] MOVE (rename) — old path gone, new path appears
- [ ] DELETE — file removed from listing
- [ ] MKCOL (create folder) — appears in listing
- [ ] Error case: GET non-existent path — 404 surfaces gracefully in client UI

**LOC estimate:** Zero code. Human effort ~1–2 hours per client. Requires physical iOS device. HTTPS endpoint required for production (HTTP localhost acceptable for dev/lab testing).

---

## Feature Dependencies

```
[Large-file streaming proof (benchmark)]
    requires --> [io.Copy streaming PUT — exists in put.go]
    requires --> [Echo middleware does not buffer body — verify first]

[Interrupted PUT test]
    requires --> [VFS Close() cleanup on error — exists in impl_v3.go]
    requires --> [net.Pipe or context-cancel simulation]

[Concurrent write test]
    requires --> [VFS mu lock — exists]
    may-require --> [CouchDB 409 -> HTTP 409/503 mapping fix ~20 LOC]

[Multi-range GET test]
    requires --> [http.ServeContent Go 1.1+ — present, Go 1.25]
    no production code change needed

[CI litmus job]
    requires --> [cozy-stack serve start in CI — new config]
    reuses --> [scripts/webdav-litmus.sh — exists]

[iOS sign-off]
    requires --> [physical iOS device]
    requires --> [HTTPS endpoint or iOS trust bypass for local HTTP]

[Content-Range PUT -> 400]
    standalone ~10 LOC in put.go
```

### Dependency Notes

- **Large-file benchmark must verify middleware first:** If Echo or any middleware buffers the request body, the benchmark will expose this and require a middleware fix before the benchmark can pass. This is the first investigation point for Category 1.
- **Interrupted PUT test has no production code dependency:** VFS cleanup is already correct. The test only needs to simulate the condition.
- **CouchDB 409 mapping is a latent bug independent of the concurrency stress test.** It should be fixed regardless.
- **Multi-range GET needs a test, not code:** Go already handles it. The test documents the behavior and guards against regression.

---

## MVP Definition for v1.2

### Launch With (v1.2 core)

- [ ] **FOLLOWUP-01 race harness fix** — pre-existing race in `pkg/config`/`model/stack`/`model/job`. Blocks `-race` usage in CI. Not WebDAV code but blocks clean testing. (~50–100 LOC in test harness)
- [ ] **Content-Range PUT -> 400** — explicit rejection per RFC 7231. ~10 LOC in `put.go`. Closes ambiguous behavior.
- [ ] **Interrupted PUT test** — verify no partial file visible after connection drop. Zero production code expected; ~80 LOC test. Closes a real data-integrity question.
- [ ] **Large-file streaming benchmark** — 1 GB upload with heap profiling and ceiling assertion. ~100 LOC. Converts "streaming" from claim to evidence.
- [ ] **Multi-range GET test** — assert `Range: bytes=0-9,20-29` returns `206 multipart/byteranges`. ~30 LOC. Zero production code.
- [ ] **Concurrent write test** + CouchDB 409 mapping fix — two goroutines PUT same path, assert deterministic outcome. ~80 LOC test + ~20 LOC fix.
- [ ] **CI litmus integration** — new GitHub Actions workflow calling `scripts/webdav-litmus.sh`. ~60 YAML lines. Closes the automation debt deferred from v1.1.

### Add After Validation (v1.2+)

- [ ] **iOS Files formal sign-off** — requires physical iOS device. Checklist above. Zero code, human effort ~1–2 hours.
- [ ] **Quota pre-check on known Content-Length** — early 413 before body read. ~50 LOC. Low priority unless quota issues reported by users.

### Future Consideration (v2+)

- [ ] **Nextcloud chunked upload protocol** — only valuable if OCS API is scoped and Nextcloud Desktop Sync becomes a target client.
- [ ] **PROPPATCH CouchDB persistence** — dead properties lost on restart. Currently in-memory only.
- [ ] **WebDAV LOCK (Class 2)** — required for macOS Finder read-write. Needs VFS lock subsystem redesign.

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| FOLLOWUP-01 race fix | HIGH (enables -race CI) | LOW (~100 LOC) | P1 |
| Content-Range PUT -> 400 | MEDIUM (correctness/spec) | LOW (~10 LOC) | P1 |
| Interrupted PUT test | HIGH (data integrity) | LOW (~80 LOC) | P1 |
| Large-file streaming benchmark | HIGH (production confidence) | LOW (~100 LOC) | P1 |
| Multi-range GET test | LOW (already works, guard only) | LOW (~30 LOC) | P1 |
| Concurrent write test + 409 fix | MEDIUM (correctness) | LOW-MEDIUM (~100 LOC) | P1 |
| CI litmus integration | HIGH (regression safety) | LOW-MEDIUM (~60 YAML) | P2 |
| iOS sign-off | HIGH (client compat) | LOW code / requires device | P2 |
| Nextcloud chunked upload | LOW for current clients | HIGH (~500+ LOC + cleanup job) | P3 |
| PROPPATCH CouchDB persistence | LOW (in-memory passes litmus) | MEDIUM (~200 LOC) | P3 |
| WebDAV LOCK / Class 2 | HIGH (macOS Finder writes) | VERY HIGH (subsystem redesign) | P3 |

**Priority key:**
- P1: Ship in v1.2 core
- P2: Ship in v1.2 if device/CI available
- P3: Defer to v2+

---

## Competitor Feature Analysis

| Feature | Apache mod_dav | sabre/dav (Nextcloud) | cozy-stack v1.1 | cozy-stack v1.2 target |
|---------|---------------|----------------------|-----------------|------------------------|
| Large file streaming | Yes (temp file + kernel splice) | Yes (PHP streams, <=15 GB claimed) | Yes (io.Copy, unmeasured ceiling) | Yes + measured ceiling (benchmark) |
| Interrupted PUT atomicity | Yes (temp rename) | Yes (PHP error handler) | Yes (VFS Close cleanup, untested) | Yes + test coverage |
| Content-Range PUT | Yes (legacy RFC 2616) | No | Ambiguous (not rejected) | Explicit 400 |
| Multi-range GET | Yes | Yes | Yes (via http.ServeContent, untested) | Yes + test coverage |
| Nextcloud chunked upload | No | Yes (proprietary) | No | No (anti-feature, out of scope) |
| Concurrent write safety | Last-write-wins (no LOCK) | Last-write-wins + optional LOCK | Last-write-wins (If-Match enforced, 409 maps to 500) | Last-write-wins + test + 409 fix |
| CI compliance testing | Optional | Yes (extensive) | Manual only (make test-litmus) | Automated GitHub Actions |

---

## Sources

- [sabre/dav Large Files documentation](https://sabre.io/dav/large-files/) — streaming architecture, 15 GB claim, output_buffering requirement — HIGH confidence (official docs)
- [Nextcloud Chunked Upload Developer Docs](https://docs.nextcloud.com/server/stable/developer_manual/client_apis/WebDAV/chunking.html) — chunk URL structure, MKCOL/PUT/MOVE protocol, OC-Total-Length, 24h cleanup, 5 MB-5 GB chunk size — HIGH confidence (official docs)
- [ownCloud issue #1051: Content-Range PUT explicitly refused](https://github.com/owncloud/core/issues/1051) — ecosystem precedent for rejecting Content-Range PUT — MEDIUM confidence (issue tracker)
- [golang/go issue #3784: multi-range ServeContent — FIXED](https://github.com/golang/go/issues/3784) — confirmed fixed Go 1.1+, applies to Go 1.25 — HIGH confidence (issue tracker + official)
- [cs3org/reva issue #86: WebDAV PUT atomicity](https://github.com/cs3org/reva/issues/86) — If-Match at finalization pattern, transaction discussion — MEDIUM confidence (issue tracker)
- [owncloud-ci/litmus Docker image](https://github.com/owncloud-ci/litmus) — env vars, image name `owncloudci/litmus`, MIT license — MEDIUM confidence (GitHub repo)
- [Lighttpd Content-Range discussion](https://redmine.lighttpd.net/boards/3/topics/8844) — deprecated-unsafe-partial-put flag context — MEDIUM confidence (forum)
- [Nextcloud large file admin docs](https://docs.nextcloud.com/server/stable/admin_manual/configuration_files/big_file_upload_configuration.html) — Apache LimitRequestBody change in 2.4.54, timeout constraints — HIGH confidence (official docs)
- [rclone forum: WebDAV 16 GB OOM](https://forum.rclone.org/t/huge-memory-usage-10gb-when-upload-a-single-large-file-16gb-in-webdav/43312) — root cause is NTLMSSP library buffering full body (not the WebDAV layer) — MEDIUM confidence (community forum)
- cozy-stack source: `web/webdav/put.go` lines 99-112 — io.Copy + Close error path — HIGH confidence (first-party code)
- cozy-stack source: `model/vfs/vfsswift/impl_v3.go` lines 865-876 — Close() deferred cleanup on error — HIGH confidence (first-party code)
- cozy-stack source: `model/vfs/file.go` lines 251-280 — ServeFileContent -> http.ServeContent delegation — HIGH confidence (first-party code)
- cozy-stack source: `.github/workflows/go-tests.yml` — existing CI structure — HIGH confidence (first-party)

---

*Feature research for: WebDAV robustness beyond RFC 4918 litmus compliance (v1.2)*
*Researched: 2026-04-12*

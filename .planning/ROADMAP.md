# Roadmap: Cozy WebDAV

## Milestones

- ✅ **v1.1 WebDAV RFC 4918 Class 1** — Phases 1-3 (shipped 2026-04-12, see [archive](milestones/v1.1-ROADMAP.md))
- 🚧 **v1.2 Robustness** — Phases 4-9 (in progress)

## Phases

<details>
<summary>✅ v1.1 WebDAV RFC 4918 Class 1 (Phases 1-3) — SHIPPED 2026-04-12</summary>

- [x] Phase 1: Foundation (9/9 plans) — completed 2026-04-05
- [x] Phase 2: Write Operations (5/5 plans) — completed 2026-04-06
- [x] Phase 3: COPY, Compliance, and Documentation (10/10 plans) — completed 2026-04-12

Full details: `.planning/milestones/v1.1-ROADMAP.md`

</details>

### 🚧 v1.2 Robustness (In Progress)

**Milestone Goal:** Prove the WebDAV server is correct under conditions the litmus suite does not exercise — multi-GB transfers, interrupted connections, byte-range edge cases, and small-scale concurrent writes. The milestone also closes v1.1 technical debt (race harness, CI litmus automation, iOS manual sign-off). This is a **correctness** milestone, not a performance or load-testing milestone. All concurrency tests stay at 2-3 goroutines maximum; no throughput assertions.

- [x] **Phase 4: Prerequisites and Instrumentation** — Race harness fix, testing.TB widening, memory/fixture/drain helpers (completed 2026-04-14)
- [ ] **Phase 5: Large-File Streaming Proof** — 1 GB PUT and GET with heap ceiling assertions
- [ ] **Phase 6: Interrupted PUT and Byte-Range Edge Cases** — Connection drops, overwrite rollback, Content-Range rejection, single/multi-range GET, 416 handling
- [ ] **Phase 7: Concurrent Access Correctness** — Two-client same-path PUT race, CouchDB 409 mapping, dirty-read guard, goroutine leak detection
- [ ] **Phase 8: CI Automation** — litmus workflow on both routes + `-short`/heavy split for Go tests so LARGE/CONC don't break the default CI budget
- [ ] **Phase 9: iOS Files Manual Validation** — Physical device sign-off with documented checklist

## Phase Details

### Phase 4: Prerequisites and Instrumentation
**Goal**: The test environment is clean and equipped — the pre-existing race is gone, the harness accepts benchmarks, and every large-file and concurrency test has the measurement primitives it needs.
**Depends on**: Phase 3 (v1.1 shipped baseline)
**Requirements**: DEBT-01, DEBT-02, INSTR-01, INSTR-02, INSTR-03
**Success Criteria** (what must be TRUE):
  1. `go test -race -count=1 ./web/webdav/...` produces zero "WARNING: DATA RACE" lines — the pre-existing `pkg/config`/`model/stack`/`model/job` race no longer fires during WebDAV test runs.
  2. `newWebdavTestEnv` accepts a `*testing.B` argument without compilation error — existing `*testing.T` callers are unaffected.
  3. A concurrent `HeapInuse` sampler helper exists in the test package — it can be called with any `func()` body and returns peak heap observed during that call.
  4. A streaming SHA-256 drain helper exists — it consumes an `io.Reader` of arbitrary size without ever allocating the full body as `[]byte`.
  5. A deterministic large-fixture generator exists — it produces an `io.Reader` of N bytes from a fixed seed, with no binary file committed to the repository.
**Plans:** 3/3 plans complete
- [ ] 04-01-PLAN.md — DEBT-01 race harness fix (env-var gate on AntivirusTrigger, with Option B escalation)
- [ ] 04-02-PLAN.md — DEBT-02 testing.TB widening + remove blanket testing.Short skip
- [ ] 04-03-PLAN.md — INSTR-01/02/03 helpers (measurePeakHeap + drainStreaming + largeFixture)

### Phase 5: Large-File Streaming Proof
**Goal**: "We stream" is no longer a claim — it is a measured, reproducible fact captured in the test suite. A 1 GB PUT and a 1 GB GET both complete with server heap below 128 MB.

**Non-goal:** This phase does not measure throughput, latency, or compare against other servers. The benchmark figures (MB/s) are informative only — no pass/fail threshold on speed.

**Depends on**: Phase 4
**Requirements**: LARGE-01, LARGE-02
**Success Criteria** (what must be TRUE):
  1. `TestPut_LargeFile_Streaming` passes: a 1 GB PUT via gowebdav completes successfully and the peak `HeapInuse` sampled concurrently during the transfer stays below 128 MB.
  2. `TestGet_LargeFile` passes: a 1 GB GET via gowebdav completes successfully, the SHA-256 of the downloaded body matches the uploaded fixture, and peak `HeapInuse` stays below 128 MB.
  3. Neither test uses `io.ReadAll`, `httpexpect.Body().Raw()`, or any accumulating buffer on the large body path — confirmed by code review.
  4. No binary fixture file is present in the repository — the 1 GB body is generated in-memory at test time.
**Plans**: TBD

### Phase 6: Interrupted PUT and Byte-Range Edge Cases
**Goal**: The server handles two families of correctness edge cases that the standard does not test: abrupt connection drops leave no orphaned data, and byte-range GET responses are well-formed across all RFC 7233 cases including multipart and out-of-bounds.

**Non-goal:** This phase does not test partial-upload resume, chunked transfer extensions, or Nextcloud-style multipart upload.

**Depends on**: Phase 5
**Requirements**: INTERRUPT-01, INTERRUPT-02, INTERRUPT-03, RANGE-01, RANGE-02, RANGE-03
**Success Criteria** (what must be TRUE):
  1. After a PUT to a new path is interrupted mid-transfer, a subsequent GET on that path returns 404 — no partial file and no orphaned CouchDB document remain.
  2. After a PUT overwriting an existing file is interrupted mid-transfer, the original file is still retrievable with its original content and original ETag — the rollback never deletes the pre-existing file.
  3. A PUT request carrying a `Content-Range` header is rejected with 501 Not Implemented before any body is read — no sparse or corrupt file is created.
  4. A single-range GET (`Range: bytes=X-Y`) returns 206 Partial Content with a correct `Content-Range` header and the exact bytes requested.
  5. A multi-range GET (`Range: bytes=X-Y,A-B`) returns 206 with a `multipart/byteranges` body; each part contains the correct sub-range bytes and its own `Content-Range` header.
  6. A GET with a range that starts beyond the file's last byte returns 416 Range Not Satisfiable with a `Content-Range: bytes */{size}` header.
**Plans**: TBD

### Phase 7: Concurrent Access Correctness
**Goal**: Two or three simultaneous clients operating on the same paths produce deterministic, non-corrupting outcomes. CouchDB MVCC conflicts are surfaced as HTTP 409 rather than silently becoming 500. No goroutine is left running after any test completes.

**Non-goal:** This phase does not measure throughput under concurrent load, does not test more than 3 goroutines, and does not simulate network-level races external to the test process.

**Depends on**: Phase 5 (streaming path proven clean), Phase 4 (race-free harness required)
**Requirements**: CONC-01, CONC-02, CONC-03, CONC-04
**Success Criteria** (what must be TRUE):
  1. When two goroutines PUT the same path concurrently, exactly one succeeds and the other receives a clear non-500 error (4xx or 503) — the winning content is retrievable, and any orphaned blob from the losing write is cleaned up by the VFS.
  2. A CouchDB MVCC conflict (409) triggered by concurrent writes is returned to the client as HTTP 409 Conflict or 503 Service Unavailable — never as 500 Internal Server Error.
  3. A PROPFIND issued while a PUT is in progress on the same path returns either the old metadata or the new metadata — never a mixed or corrupt view of the resource.
  4. Every concurrent test calls `goleak.VerifyNone(t)` in its cleanup and passes — no goroutine created during the test survives beyond the test boundary.
**Plans**: TBD

### Phase 8: CI Automation
**Goal**: The full regression safety net — litmus 63/63 + the v1.2 heavy Go tests (LARGE, CONC) — runs automatically on every push to master and every PR touching `web/webdav/`. The default Go-tests workflow stays fast and lightweight for non-WebDAV PRs via the `-short` convention. Regressions are caught before merge, not by manual re-run.

**Non-goal:** This phase does not add cross-platform matrices, does not benchmark throughput, and does not add long-running endurance tests.

**Depends on**: Phase 4 (race-free baseline required for reliable CI), Phase 5 (LARGE tests exist and need the `-short` guard), Phase 7 (CONC tests exist if they also need the guard)
**Requirements**: CI-01, CI-02, CI-03
**Success Criteria** (what must be TRUE):
  1. A `.github/workflows/webdav-litmus.yml` file exists and is syntactically valid — the workflow triggers on `push` to `master` and on PRs that touch `web/webdav/**`.
  2. The workflow runs litmus against both `/dav/files/` and `/remote.php/webdav/` using the `owncloud/litmus` Docker image (not `apt install litmus`) and passes 63/63 on each route.
  3. The workflow includes an explicit CouchDB readiness poll (`/_up` endpoint, ≥30 retries) before any test begins — it does not rely on Docker `healthy` status alone.
  4. The litmus results appear as a formatted table (pass/fail per suite) in the GitHub Actions job summary, visible from the PR checks page without opening raw logs.
  5. The existing `.github/workflows/go-tests.yml` passes `-short` in its `go test` invocation — heavy tests are skipped in that workflow so non-WebDAV PRs keep their current CI duration.
  6. Every heavy v1.2 Go test (LARGE in Phase 5, and any CONC test that triggers the same issue in Phase 7) calls `if testing.Short() { t.Skip(...) }` at the top of its function — verified by a grep check in the success criteria.
  7. A new `.github/workflows/webdav-heavy.yml` file exists, triggers on push-to-master and on PRs touching `web/webdav/**`, runs `go test` without `-short` on the `web/webdav/` package with a 30-minute timeout, and passes all LARGE and CONC tests.
  8. Local `go test ./web/webdav/...` (no flags) continues to run the full suite including heavy tests — the `-short` convention is CI-side only, not a local-default change.
**Plans**: TBD

### Phase 9: iOS Files Manual Validation
**Goal**: A human with a physical iOS device has verified that the WebDAV server works end-to-end with the iOS Files app, and the results are documented in a permanent artifact.
**Depends on**: Phase 6 (server correctness proven), Phase 5 (large file handling verified)
**Requirements**: VAL-01
**Success Criteria** (what must be TRUE):
  1. A file `v1.2-MANUAL-VALIDATION-IOS.md` exists in the repository documenting the validation session — device model, iOS version, date, tester identity, and per-step results.
  2. The checklist covers at minimum: connecting the WebDAV account, directory listing, uploading a file from Photos, downloading a file to Files, renaming a file, moving a file between folders, and opening a file in Pages, Numbers, or Keynote.
  3. All checklist steps passed — no blocking failures observed on an iOS 17+ device.
  4. Any observed quirks or non-blocking issues are noted in the document for future reference.
**Plans**: TBD

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Foundation | v1.1 | 9/9 | Complete | 2026-04-05 |
| 2. Write Operations | v1.1 | 5/5 | Complete | 2026-04-06 |
| 3. COPY, Compliance, and Documentation | v1.1 | 10/10 | Complete | 2026-04-12 |
| 4. Prerequisites and Instrumentation | 3/3 | Complete   | 2026-04-14 | - |
| 5. Large-File Streaming Proof | v1.2 | 0/? | Not started | - |
| 6. Interrupted PUT and Byte-Range Edge Cases | v1.2 | 0/? | Not started | - |
| 7. Concurrent Access Correctness | v1.2 | 0/? | Not started | - |
| 8. CI Litmus Automation | v1.2 | 0/? | Not started | - |
| 9. iOS Files Manual Validation | v1.2 | 0/? | Not started | - |

---

*Last milestone shipped: v1.1 (2026-04-12)*
*v1.2 Robustness started: 2026-04-13*

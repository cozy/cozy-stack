# Architecture Research

**Domain:** WebDAV robustness — v1.2 integration points
**Researched:** 2026-04-12
**Confidence:** HIGH (all findings from direct code inspection)

## Standard Architecture

### System Overview

```
  HTTP client (gowebdav / rclone / iOS Files)
         │
         ▼
  Echo v4 router  (web/webdav/webdav.go → Routes / NextcloudRoutes)
         │
  resolveWebDAVAuth middleware   (web/webdav/auth.go)
         │
  handlePath dispatcher          (web/webdav/handlers.go)
         │
  ┌──────┴─────────────────────────────────────────────┐
  │  handleGet  handlePut  handleCopy  handleMove …    │
  │  (get.go)  (put.go)   (copy.go)   (move.go)  …    │
  └──────────────────────┬─────────────────────────────┘
                         │  delegates all metadata + bytes
                         ▼
          vfs.VFS interface  (model/vfs/vfs.go)
          ┌──────────────────────────────┐
          │  Indexer (CouchDB metadata)  │
          │  Fs      (bytes storage)     │
          │  DiskThresholder             │
          └──────┬────────┬─────────────┘
                 │        │
          CouchDB         Swift object store (or afero for tests)
```

### Component Responsibilities

| Component | File | Responsibility |
|-----------|------|----------------|
| `handlePath` | `web/webdav/handlers.go:30` | Method dispatch; no business logic |
| `handlePut` | `web/webdav/put.go:27` | File create/overwrite; ETag preconditions |
| `handleGet` | `web/webdav/get.go:27` | File read; delegates range/ETag to `vfs.ServeFileContent` |
| `vfs.ServeFileContent` | `model/vfs/file.go:251` | Opens file, sets ETag, calls `http.ServeContent` |
| `swiftVFSV3.CreateFile` | `model/vfs/vfsswift/impl_v3.go:187` | Locks VFS, creates Swift object, returns write handle |
| `swiftFileCreationV3.Close` | `model/vfs/vfsswift/impl_v3.go:865` | Commits MD5, size; calls `Indexer.CreateFileDoc` / `UpdateFileDoc` |
| `aferoFileCreation.Close` | `model/vfs/vfsafero/impl.go:807` | Test-path equivalent; writes to tmp file then renames |

---

## Integration Point Analysis Per v1.2 Feature

### 1. Streaming PUT — body flow and buffering hot spots

**Trace** (`put.go` → VFS → Swift):

```
put.go:104  io.Copy(file, c.Request().Body)
              └─ file is swiftFileCreationV3 (impl_v3.go:837)
                   └─ Write() → f.f.Write(p)  (Swift ObjectCreateFile — chunked HTTP to Swift)
                   └─ meta extractor (EXIF/media sniffer, goroutine pipe, non-blocking)
put.go:107  file.Close()
              └─ swiftFileCreationV3.Close() (impl_v3.go:865)
                   └─ f.f.Close()  → Swift finalises object, returns ETag in headers
                   └─ Indexer.CreateFileDoc / UpdateFileDoc  → CouchDB write
```

**No accumulation buffer in happy path.** The `io.Copy` in `put.go:104` streams 32 KiB chunks directly from `c.Request().Body` into the Swift `ObjectCreateFile` writer. Swift's Go client sends these over a persistent HTTP/1.1 connection to the object store. Neither put.go nor the VFS layer accumulates the full body in memory.

**Hot spots that need `-memprofile` verification:**

| Location | Risk | What to check |
|----------|------|---------------|
| `put.go:104` `io.Copy` default 32 KiB buffer | Negligible per-call | Baseline for comparison |
| `swiftFileCreationV3.meta` (impl_v3.go:838) | MetaExtractor drains a copy of each chunk via a goroutine pipe | Confirm the pipe's internal buffer (io.Pipe — no buffer) |
| `aferoFileCreation` (test path) | Writes to `afero.TempFile` on disk — no memory accumulation | Low concern |
| Echo's `c.Request().Body` | Echo does NOT buffer by default; body is raw `net.Conn` reader | Confirm no `io.ReadAll` anywhere in middleware stack |

**Test path note:** `seedFile` in `get_test.go:28` uses `fs.CreateFile` + `io.Copy` + `f.Close()` in-process — same streaming path as production.

---

### 2. Interrupted PUT — VFS state on client disconnect

**What happens when `r.Body.Read` returns an error mid-transfer:**

`io.Copy` at `put.go:104` propagates the read error. `file.Close()` is called regardless (via the `if cerr := file.Close()` block at put.go:107).

In `swiftFileCreationV3.Close()` (`impl_v3.go:865`):

```go
defer func() {
    if err != nil {
        _ = f.fs.c.ObjectDelete(f.fs.ctx, f.fs.container, f.name)  // line 869
        if f.olddoc == nil {
            _ = f.fs.Indexer.DeleteFileDoc(f.newdoc)               // line 873
        }
    }
}()
```

The deferred cleanup in `Close()` deletes the partial Swift object and (for new files) removes the CouchDB document. For overwrites (`olddoc != nil`), the old CouchDB document is preserved — the overwrite's new doc never reaches the index because `UpdateFileDoc` is only called after a successful `Close`. The old bytes in Swift are only replaced after a successful close, not on open.

- **New file, interrupted:** Swift object deleted, CouchDB doc deleted. Clean state.
- **Overwrite, interrupted:** Old CouchDB doc intact, old Swift object intact. Clean state.
- **Race window (create path):** `CreateFileDoc` is called at impl_v3.go:967, inside `Close()`, only after `f.f.Close()` succeeds. If Swift close fails, the defer fires, `ObjectDelete` is called. No CouchDB doc was written yet. Clean state.

**Conclusion:** VFS handles interruption cleanly by design. No partial-committed state is possible. The v1.2 interrupted-PUT tests should verify the observable HTTP-level behavior (connection drop → no orphan visible via subsequent PROPFIND/GET), not VFS internals.

**Integration point for interruption test:** `testutil_test.go:21` — `env.TS` is `*httptest.Server`. Call `env.TS.CloseClientConnections()` mid-upload to simulate disconnect. Verify with a subsequent `env.E.GET(path)` that returns 404.

---

### 3. Byte-range GET — which layer handles ranges

**Short answer:** `http.ServeContent` (stdlib) handles all range logic.

Trace in `get.go:59`:
```go
return vfs.ServeFileContent(inst.VFS(), fileDoc, nil, "", "", c.Request(), c.Response())
```

`ServeFileContent` at `model/vfs/file.go:251–280`:
1. Sets `Content-Type` from `doc.Mime`.
2. Sets `Etag` header from `base64(doc.MD5Sum)` — only for non-range requests (line 261: `if header.Get("Range") == ""`).
3. Opens file with `fs.OpenFile(doc)` → returns `swiftFileOpenV3` (implements `io.ReadSeeker`).
4. Calls `http.ServeContent(w, req, filename, doc.UpdatedAt, content)`.

`http.ServeContent` (stdlib) handles:
- Single `Range: bytes=N-M` → 206 with `Content-Range`
- Multi-range `Range: bytes=N-M,P-Q` → 206 `multipart/byteranges`
- `If-Range` / `If-None-Match` / `If-Modified-Since` conditional logic
- HEAD (no body written)

**For single-range GET (v1.2 scope):** Nothing to implement. Already works (confirmed by `gowebdav_integration_test.go:162–168` which exercises `bytes=0-4`).

**For multi-range GET (v1.2 scope, if required):** Also already works via `http.ServeContent`. No code change needed in `get.go`. The question is whether `swiftFileOpenV3` implements `io.ReadSeeker` correctly for seek-based multi-range access — this needs a test, not a code change.

**ETag note for range requests:** `ServeFileContent` deliberately does NOT set `Etag` when a `Range` header is present (file.go:261). `http.ServeContent` sets its own `ETag` from the `modtime` argument. This means ETag on ranged responses will differ from PROPFIND's ETag. This is known behaviour, worth documenting.

---

### 4. Content-Range PUT vs chunked upload

**Content-Range PUT (offset write into existing file):**

The VFS `File` interface (`model/vfs/vfs.go:139`) is `io.Reader + io.ReaderAt + io.Seeker + io.Writer + io.Closer`. The creation handle (`swiftFileCreationV3`) returns `os.ErrInvalid` for `Seek` (impl_v3.go:833). VFS does not support partial/offset writes to an in-progress upload. Swift's object store also does not support random-write streaming.

To implement Content-Range PUT, you would need to download the existing object from Swift, patch the byte range, and re-upload the full object. This is expensive for large files and out of scope for v1.2.

**Correct v1.2 action:** Reject `Content-Range` in PUT with `501 Not Implemented`. Add a header guard at `put.go` near line 80, before the call to `fs.CreateFile`.

**Chunked upload (TUS / Nextcloud chunked):**

Requires new endpoints `/remote.php/dav/uploads/{user}/{id}/{chunk}` and a MOVE-to-finalize step. This is a separate sub-protocol requiring:
- New route group in `webdav.go`
- New handler file `web/webdav/chunked.go`
- An assembly stage that stitches chunk objects in Swift (or a staging area in afero)
- Upload session state tracking — either in-memory or CouchDB

This is a major refactor. Out of scope for v1.2.

---

### 5. Concurrency — CouchDB MVCC + VFS locking

**Current locking model:**

Both `aferoVFS` and `swiftVFSV3` hold an `ErrorRWLocker` (`pkg/lock/`). In tests, this is `InMemoryLockGetter` → `memLock` wrapping `sync.RWMutex`. In production, it is a Redis-backed distributed lock.

`CreateFile` acquires the write lock at entry (`impl_v3.go:188–191`), does pre-checks, then releases it with `defer sfs.mu.Unlock()`. The write lock is NOT held during the actual bytes transfer to Swift — streaming happens outside the lock.

`Close()` re-acquires the write lock at impl_v3.go:937 before calling `CreateFileDoc`/`UpdateFileDoc`. This is the critical section for the index write.

**Race window for two concurrent PUTs to the same path (new file):**

1. Both calls enter `CreateFile`, pass the `DirChildExists` check (file not yet in index), both receive a `swiftFileCreationV3` handle with distinct `InternalID` and `objName`.
2. Both stream bytes to Swift independently (no lock held).
3. Both call `Close()`. The first to re-acquire the lock writes `CreateFileDoc`. The second re-acquires the lock, hits the `DirChildExists` check at impl_v3.go:946, sees the file now exists, returns `os.ErrExist`.
4. The second's Swift object is deleted by the deferred cleanup at impl_v3.go:869.

**Outcome:** Deterministic — no silent data loss. The loser gets `os.ErrExist` which `mapVFSWriteError` maps to a 5xx response. The winner's content is preserved.

**ETag composition (inconsistency between methods):**

- PROPFIND uses `doc.DocRev` (CouchDB `_rev`) as ETag.
- GET/HEAD uses `base64(doc.MD5Sum)` via `ServeFileContent` (file.go:262).
- PUT `If-Match` preconditions (`checkETagPreconditions` in `write_helpers.go`) compare against the MD5-based ETag.

These are different values for the same resource. This inconsistency is pre-existing and out of scope for v1.2, but must be documented.

**Implication for v1.2 concurrency tests:** Test should verify that after two concurrent PUTs, exactly one response is 2xx and the file content matches the successful PUT's body. No need to assert which client wins.

---

### 6. Test harness extensions

**Current capabilities of `testutil_test.go`:**

| Capability | Current State |
|------------|---------------|
| Single test instance + token | Yes (`newWebdavTestEnv`) |
| Multiple routes on same server | Yes (done in `gowebdav_integration_test.go:206`) |
| Multiple concurrent clients | Not built-in; trivially addable with two `gowebdav.NewClient` calls to `env.TS.URL` |
| Large bodies without memory explosion | Not tested; `seedFile` uses `bytes.NewReader` (in-memory). For large-file tests, use `io.LimitReader(rand.Reader, N)` — generates a reader without allocating N bytes upfront |
| RSS measurement across test boundaries | Not present. Achievable with `runtime.ReadMemStats` before/after, or `testing.B` with `-memprofile` |
| Connection drop simulation | `env.TS.CloseClientConnections()` exists on `*httptest.Server` — not yet used in webdav tests |
| CouchDB required | Yes (`testutils.NeedCouchdb(t)`) |

**Additions needed for v1.2:**

```go
// Concurrent clients: no new infrastructure needed
c1 := gowebdav.NewClient(env.TS.URL+"/dav/files", "", env.Token)
c2 := gowebdav.NewClient(env.TS.URL+"/dav/files", "", env.Token)
var wg sync.WaitGroup
// launch goroutines, collect results

// Large streaming body (no allocation spike):
body := io.LimitReader(rand.Reader, 100<<20)  // 100 MiB, never materialised as a slice

// RSS measurement helper:
var before, after runtime.MemStats
runtime.ReadMemStats(&before)
// ... operation ...
runtime.ReadMemStats(&after)
delta := int64(after.HeapInuse) - int64(before.HeapInuse)

// Connection drop:
go func() {
    time.Sleep(50 * time.Millisecond)
    env.TS.CloseClientConnections()
}()
```

---

### 7. CI integration of litmus

**Existing pattern:** `system-tests.yml` — separate workflow, runs `make system-tests`, boots a real cozy-stack binary, long-lived job on ubuntu-22.04.

**Litmus requirements:**
- `litmus` binary: `apt install litmus` (available on ubuntu-22.04 universe)
- Running cozy-stack: needs `go install` then `cozy-stack serve` in background
- CouchDB: same install block as go-tests.yml

**Recommended file:** `.github/workflows/webdav-litmus.yml`

**Structure:** Mirror `system-tests.yml`:
- Same CouchDB install block from go-tests.yml
- `apt install litmus`
- `go install` to build the `cozy-stack` binary
- Start `cozy-stack serve` in background (requires a config file; see existing cozy.yaml in the scripts directory)
- Call `make test-litmus` (which invokes `scripts/webdav-litmus.sh`)
- The script handles instance lifecycle via `trap cleanup EXIT`

**Trigger:** Same as `system-tests.yml` — `push: master` + `pull_request`, ignore `docs/**`.

**Do NOT add litmus to `go-tests.yml`:** The existing test job uses `-timeout 5m` for the full suite. Litmus requires a running HTTP server (30–60 s), which is incompatible with in-process unit tests.

**No changes needed to `scripts/webdav-litmus.sh`** — it already handles instance create/destroy, both routes, and exit codes correctly.

---

### 8. New components vs modifications — per capability

| Capability | Type | File(s) | Notes |
|------------|------|---------|-------|
| Memory profiling / RSS measurement helper | New test helper | `web/webdav/memstats_test.go` (or inline in first large-file test) | Must come first — all streaming proofs depend on it |
| Large-file streaming PUT test | New test function | `web/webdav/put_test.go` | Add `TestPut_LargeFile_Streaming` |
| Large-file GET test | New test function | `web/webdav/get_test.go` | Add `TestGet_LargeFile` |
| Interrupted PUT test | New test function | `web/webdav/put_test.go` | Add `TestPut_Interrupted_NoOrphan`; uses `env.TS.CloseClientConnections()` |
| Content-Range PUT rejection | Modify | `web/webdav/put.go` near line 80 | 1-line header guard; add companion test in `put_test.go` |
| Byte-range GET multi-range test | New test function | `web/webdav/get_test.go` | Add `TestGet_MultiRange`; no code change to `get.go` expected |
| Concurrency PUT test | New test function | `web/webdav/put_test.go` | Add `TestPut_Concurrent_SamePath`; uses `sync.WaitGroup` + two clients |
| FOLLOWUP-01 race fix | Modify | `pkg/config/`, `model/stack/`, `model/job/` | Outside webdav; first task per PROJECT.md |
| CI litmus workflow | New file | `.github/workflows/webdav-litmus.yml` | Model after `system-tests.yml` |

---

## Recommended Build Order

Dependencies flow top-down. Each step unlocks the next.

```
Step 0 (prerequisite):
  Fix FOLLOWUP-01 race in pkg/config / model/stack / model/job
  → Eliminates pre-existing test flakiness that would corrupt memory measurements

Step 1 (instrumentation first):
  web/webdav/memstats_test.go — RSS / HeapInuse measurement helper
  → Required by Step 2 to prove streaming without allocation

Step 2 (streaming proof):
  TestPut_LargeFile_Streaming  (put_test.go)
  TestGet_LargeFile            (get_test.go)
  → Proves or disproves streaming; guides any fixes if accumulation is found

Step 3 (interruption):
  TestPut_Interrupted_NoOrphan (put_test.go)
  → Validates VFS cleanup; no code change expected (VFS already cleans up)

Step 4 (defensive guard):
  Content-Range PUT rejection   (put.go + put_test.go)
  → Small one-line code change; prevents future misuse

Step 5 (range completeness):
  TestGet_MultiRange            (get_test.go)
  → Validates http.ServeContent multi-range; no code change expected

Step 6 (concurrency):
  TestPut_Concurrent_SamePath  (put_test.go)
  → Most complex test; requires Steps 0–3 stable to avoid flakiness

Step 7 (CI):
  .github/workflows/webdav-litmus.yml
  → Can be written in parallel with Steps 1–6 but benefits from Step 6 stability
```

---

## Data Flow

### PUT request (streaming path)

```
c.Request().Body  (net/http body reader — no buffer)
    │  io.Copy, 32 KiB chunks (put.go:104)
    ▼
swiftFileCreationV3.Write()  (impl_v3.go:837)
    ├─ MetaExtractor goroutine pipe (non-blocking, EXIF/media sniff)
    └─ swift.ObjectCreateFile.Write()  → chunked HTTP PUT to Swift object store
         │
    file.Close() called at put.go:107
         │
    swiftFileCreationV3.Close()  (impl_v3.go:865)
         ├─ swift.ObjectCreateFile.Close()  — Swift finalises, returns Etag
         ├─ re-acquire sfs.mu (Redis lock in prod, sync.RWMutex in tests)
         ├─ DirChildExists check (second gate for concurrent creates, line 946)
         └─ Indexer.CreateFileDoc / UpdateFileDoc  → CouchDB
```

### GET request (range path)

```
c.Request() with optional Range header
    │
handleGet (get.go:59)
    │
vfs.ServeFileContent (model/vfs/file.go:251)
    ├─ Set Content-Type from doc.Mime
    ├─ Set Etag from base64(MD5Sum) IF Range header absent
    ├─ fs.OpenFile(doc) → swiftFileOpenV3 (io.ReadSeeker into Swift)
    └─ http.ServeContent(w, req, filename, modtime, content)
         ├─ Parses Range header (single or multi-range)
         ├─ Seeks reader to range start
         └─ Copies range bytes to response writer (206 or 200)
```

---

## Integration Boundaries

| Boundary | Interface | Notes |
|----------|-----------|-------|
| `web/webdav` → `model/vfs` | `vfs.VFS` interface | No direct CouchDB or Swift access from handlers |
| `vfs.VFS` → CouchDB | `vfs.Indexer` (couchdb_indexer.go) | All metadata ops go through indexer |
| `vfs.VFS` → Swift | `swiftVFSV3` or `aferoVFS` | Selected at instance creation time |
| Test harness → VFS | Same `vfs.VFS` interface, `aferoVFS` backend | FS URL set to `t.TempDir()` in testutil_test.go:45 |
| WebDAV handler → Lock | Via `swiftVFSV3.mu` / `aferoVFS.mu` | In tests: in-memory sync.RWMutex; prod: Redis |

---

## Sources

All findings from direct code inspection (no external sources):

- `web/webdav/put.go` — PUT handler, io.Copy location (line 104)
- `web/webdav/get.go` — GET handler, ServeFileContent delegation (line 59)
- `web/webdav/handlers.go` — dispatch
- `web/webdav/testutil_test.go` — test harness structure
- `web/webdav/gowebdav_integration_test.go` — E2E patterns including range test
- `web/webdav/get_test.go` — seedFile implementation (line 18)
- `model/vfs/vfs.go` — VFS/Fs/Indexer interfaces, File interface (line 139)
- `model/vfs/file.go` — ServeFileContent (line 251), FileDoc, NewFileDoc
- `model/vfs/vfsswift/impl_v3.go` — CreateFile (line 187), swiftFileCreationV3.Write (line 837), Close (line 865), cleanup defer (line 866), MD5/CouchDB commit (lines 937–970)
- `model/vfs/vfsafero/impl.go` — CreateFile (line 189), aferoFileCreation.Close (line 807)
- `pkg/lock/` — ErrorRWLocker, InMemoryLockGetter, redisLock
- `.github/workflows/go-tests.yml` — test pipeline convention (timeout 5m)
- `.github/workflows/system-tests.yml` — long-running CI job pattern
- `Makefile` — test-litmus target (line 83)
- `scripts/webdav-litmus.sh` — litmus orchestration, instance lifecycle

---
*Architecture research for: cozy-stack WebDAV v1.2 robustness*
*Researched: 2026-04-12*

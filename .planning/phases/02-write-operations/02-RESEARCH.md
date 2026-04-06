# Phase 2: Write Operations - Research

**Researched:** 2026-04-05
**Domain:** WebDAV write methods (PUT, DELETE, MKCOL, MOVE) over the Cozy VFS
**Confidence:** HIGH — all findings verified directly in the codebase; no external searches required.

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**DELETE semantics — soft-trash, not hard destroy**
- DELETE calls `vfs.TrashFile` (files) and `vfs.TrashDir` (directories) — NOT `DestroyFile`/`DestroyDirAndContent`
- REQUIREMENTS.md WRITE-05/06 reference `DestroyFile`/`DestroyDirAndContent` — those apply to items already in trash. Planner MUST use TrashFile/TrashDir.
- DELETE on a directory: `TrashDir` trashes the entire tree in one operation — no ErrDirNotEmpty case to handle
- Success response: 204 No Content

**DELETE on items inside .cozy_trash/***
- Returns 405 Method Not Allowed with `Allow: PROPFIND, GET, HEAD, OPTIONS`

**PUT — native VFS overwrite, no temp file**
- `vfs.CreateFile(newdoc, olddoc)` — olddoc=nil for create, olddoc=existing for overwrite
- `io.Copy` from request body into the returned `io.WriteCloser`
- Zero-byte PUT accepted
- Content-Type: trust client header if present; fall back to `ExtractMimeAndClassFromFilename` if missing or `application/octet-stream`
- Parent directory missing: 409 Conflict

**PUT — conditional writes**
- `If-Match: "etag"` → compare against VFS ETag (MD5-based); mismatch → 412 Precondition Failed
- `If-None-Match: *` → if file already exists → 412
- If-Match absent on overwrite: unconditional overwrite
- `If:` header with lock tokens: ignored entirely

**MKCOL**
- Single directory only via `vfs.Mkdir` (NOT MkdirAll — known race condition)
- Parent directory missing → 409 Conflict
- Path already exists → 405 Method Not Allowed
- MKCOL with request body → 415 Unsupported Media Type

**MOVE — overwrite and edge cases**
- Absent `Overwrite` header treated as T (RFC 4918 default — fixes x/net/webdav bug #66059)
- `Overwrite: F` with existing destination → 412 Precondition Failed
- `Overwrite: T` with existing destination → trash the target first, then rename
- `vfs.ModifyFileMetadata` with `DocPatch` for files; `vfs.ModifyDirMetadata` for directories
- Destination header: URL-decoded and validated via `davPathToVFSPath`
- MOVE into `.cozy_trash/*` → 403 Forbidden
- MOVE of directory containing shared items: surface VFS permission error as 403

**Lock-token handling (If: header)**
- Ignored entirely — no parsing, no evaluation

**Audit logging for writes**
- WARN level for: write attempt to .cozy_trash (403/405), quota exceeded on PUT (507), permission denied / sharing violation on MOVE/DELETE (403), path traversal on Destination header
- NOT logged: successful PUT/DELETE/MKCOL/MOVE

**Error mapping**
- Quota exceeded → 507 Insufficient Storage
- Parent not found → 409 Conflict
- Path already exists (MKCOL) → 405 Method Not Allowed
- Permission / sharing error → 403 Forbidden
- File/dir not found → 404 Not Found
- ETag mismatch / If-None-Match violated → 412 Precondition Failed
- All errors use `sendWebDAVError` (RFC 4918 §8.7 XML)

**Testing (TDD strict)**
- RED → GREEN → REFACTOR cycle with separate commits
- Integration tests using gowebdav client
- Never mock the VFS — use test instance with real afero/mem-backed VFS
- `testutil_test.go` harness from Phase 1 reused

### Claude's Discretion
- File split within `web/webdav/` (one file per method? grouped by concern?)
- Exact `DocPatch` construction for MOVE (field names, nil-vs-zero handling)
- Whether to expose `Content-Length` in PUT response (201/204 have no body, but some clients expect it)
- Internal helper structure for Destination header parsing
- How to detect and surface VFS-specific error types (type assertion patterns)

### Deferred Ideas (OUT OF SCOPE)
- App-specific passwords, LOCK/UNLOCK, PROPPATCH, quota properties, metrics, rate limiting, alerting, Digest Auth, PROPFIND cap — all deferred to v2.
- COPY deferred to Phase 3.
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| WRITE-01 | PUT — streaming upload, uses `vfs.CreateFile`/`io.Copy` | VFS `CreateFile(newdoc, olddoc)` returns `io.WriteCloser`; caller does `io.Copy(w, r.Body)` then `w.Close()`. Pattern confirmed in `web/files/files.go:311-316`. |
| WRITE-02 | PUT creates or overwrites existing file | `olddoc=nil` → create; `olddoc=existing FileDoc` → overwrite. VFS handles atomicity. |
| WRITE-03 | PUT supports `If-Match` and `If-None-Match` conditional headers | ETag is MD5-based (`buildETag` in `xml.go:128`). Must implement custom comparison since VFS uses MD5 not CouchDB rev. |
| WRITE-04 | PUT on path with missing parent returns 409 Conflict | VFS returns `vfs.ErrParentDoesNotExist` → map to 409. |
| WRITE-05 | DELETE file — soft-trash via `vfs.TrashFile` | `vfs.TrashFile(fs, olddoc)` confirmed at `model/vfs/file.go:386`. Returns `(newdoc, err)`; on `ErrFileInTrash` → 405. |
| WRITE-06 | DELETE directory — soft-trash via `vfs.TrashDir` | `vfs.TrashDir(fs, olddoc)` confirmed at `model/vfs/directory.go:292`. Returns `(newdoc, err)`; on `ErrFileInTrash` → 405. |
| WRITE-07 | MKCOL — single-level via `vfs.Mkdir` | `vfs.Mkdir(fs, name, tags)` at `model/vfs/vfs.go:492`. Returns `(*DirDoc, error)`. |
| WRITE-08 | MKCOL with missing parent returns 409 Conflict | `vfs.Mkdir` calls `DirByPath(path.Dir(name))` internally; returns `ErrParentDoesNotExist` on miss. |
| WRITE-09 | MKCOL on existing path returns 405 Method Not Allowed | `fs.CreateDir(dir)` returns `os.ErrExist` when path already present. |
| MOVE-01 | MOVE file — `vfs.ModifyFileMetadata` with `DocPatch{Name, DirID}` | Confirmed at `model/vfs/file.go:306`. Patch takes `*string` pointers for Name and DirID. |
| MOVE-02 | MOVE directory — `vfs.ModifyDirMetadata` | Confirmed at `model/vfs/directory.go:232`. Guards against root/trash dir IDs. |
| MOVE-03 | Absent `Overwrite` header treated as T | Implement in handler: `if ovr := r.Header.Get("Overwrite"); ovr == "" { ovr = "T" }` |
| MOVE-04 | `Overwrite: F` with existing destination → 412 | Check destination existence via `DirOrFileByPath`; if exists and Overwrite=F → 412. |
| MOVE-05 | Destination header URL-decoded and path-traversal validated | Parse `Destination` header URL, extract path, pass through `davPathToVFSPath`. |
| TEST-03 | Integration tests via gowebdav covering all write methods | Extend `gowebdav_integration_test.go` or add `write_integration_test.go`; uses `newWebdavTestEnv` + `gowebdav.NewClient`. |
</phase_requirements>

---

## Summary

Phase 2 adds PUT, DELETE, MKCOL, and MOVE to the already-complete Phase 1 WebDAV foundation. All write primitives exist and are stable in `model/vfs/` — there is no new VFS code to write, only handlers in `web/webdav/`. The dispatcher in `handlers.go` already has the method-switch scaffold; the four new methods plug in as cases that replace the current `501 Not Implemented` default.

The key architectural decision (DELETE = soft-trash, not hard-destroy) is locked. Every method shares the same error-mapping pattern from Phase 1: VFS error → `sendWebDAVError` with RFC 4918 XML body. The ETag used for `If-Match` is the MD5-based form from `buildETag` (not CouchDB `_rev`), and the Destination header for MOVE must be parsed through `davPathToVFSPath` with the same traversal guards as read paths.

The most complex handler is MOVE: it requires parsing a full URL from the `Destination` header, resolving source and destination VFS paths, optionally trashing an existing destination (Overwrite=T), and using `DocPatch{Name, DirID}` to move files and directories atomically via the VFS metadata update path.

**Primary recommendation:** Implement handlers in the order PUT → DELETE → MKCOL → MOVE, each in its own file (`put.go`, `delete.go`, `mkcol.go`, `move.go`) following the Phase 1 convention of one concern per file, with the TDD RED→GREEN→REFACTOR cycle strictly respected per commit.

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `model/vfs` (internal) | — | All file/directory mutations | Sole VFS abstraction; all write primitives present |
| `github.com/labstack/echo/v4` | v4 | HTTP handler context, request/response | Already in use; no change |
| `github.com/studio-b12/gowebdav` | current | Integration test WebDAV client | Already in use from Phase 1 TEST-04 |
| `github.com/gavv/httpexpect/v2` | v2 | Raw HTTP assertions in tests | Already in use from Phase 1 |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `net/url` (stdlib) | — | Parse `Destination` header URL | MOVE handler only |
| `io` (stdlib) | — | `io.Copy` for PUT streaming | PUT handler |
| `os` (stdlib) | — | `os.ErrNotExist`, `os.ErrExist` error checks | All handlers |
| `github.com/cozy/cozy-stack/model/permission` | — | `permission.PUT`, `permission.DELETE` verb checks | PUT/DELETE permission guards |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `vfs.TrashFile`/`TrashDir` | `fs.DestroyFile`/`fs.DestroyDirAndContent` | Hard-destroy is irreversible; trash chosen for user safety |
| `vfs.Mkdir` | `vfs.MkdirAll` | MkdirAll has a documented race condition (CONCERNS.md) — single-level Mkdir only |
| Custom ETag matching | `web/files.CheckIfMatch` | That function compares CouchDB `_rev`; WebDAV ETag is MD5-based — must write custom comparison |

**Installation:** No new dependencies. All required packages are already in `go.mod`.

---

## Architecture Patterns

### Recommended Project Structure

```
web/webdav/
├── handlers.go        # Dispatcher — add PUT/DELETE/MKCOL/MOVE cases (already exists)
├── put.go             # handlePut — new (Phase 2)
├── delete.go          # handleDelete — new (Phase 2)
├── mkcol.go           # handleMkcol — new (Phase 2)
├── move.go            # handleMove + parseDestination helper — new (Phase 2)
├── put_test.go        # TDD tests for PUT
├── delete_test.go     # TDD tests for DELETE
├── mkcol_test.go      # TDD tests for MKCOL
├── move_test.go       # TDD tests for MOVE
├── write_integration_test.go  # gowebdav Phase 2 success criteria (TEST-03)
│
│   [Phase 1 — unchanged]
├── get.go             propfind.go  xml.go  errors.go  auth.go
├── path_mapper.go     webdav.go
└── testutil_test.go   gowebdav_integration_test.go
```

### Pattern 1: VFS Write (CreateFile + io.Copy)

**What:** The canonical Cozy write flow. VFS handles atomicity, quota check, rollback on partial write.
**When to use:** Every PUT handler — both create and overwrite cases.

```go
// Source: web/files/files.go:311-316 (OverwriteFileContent)
// Source: web/webdav/get_test.go:28-35 (seedFile — same pattern in tests)

file, err := inst.VFS().CreateFile(newdoc, olddoc) // olddoc=nil → create
if err != nil {
    return mapVFSError(c, err)
}
_, err = io.Copy(file, c.Request().Body)
if cerr := file.Close(); cerr != nil && err == nil {
    err = cerr
}
if err != nil {
    return mapVFSError(c, err)
}
// olddoc==nil → respond 201 Created
// olddoc!=nil → respond 204 No Content
```

**Critical:** `file.Close()` MUST be called and its error checked. The VFS only commits the write on a successful Close. If Close returns an error (quota overflow, Swift error), the caller must treat the write as failed.

### Pattern 2: Soft-Trash (TrashFile / TrashDir)

**What:** Move resource to `.cozy_trash` — recoverable. VFS handles conflict naming if trash already contains an item with the same name.
**When to use:** DELETE handler.

```go
// Source: model/vfs/file.go:386-413 (TrashFile)
// Source: model/vfs/directory.go:292-318 (TrashDir)

dir, file, err := inst.VFS().DirOrFileByPath(vfsPath)
if err != nil {
    if errors.Is(err, os.ErrNotExist) {
        return sendWebDAVError(c, http.StatusNotFound, "not-found")
    }
    return mapVFSError(c, err)
}

if file != nil {
    _, err = vfs.TrashFile(inst.VFS(), file)
} else {
    _, err = vfs.TrashDir(inst.VFS(), dir)
}
// TrashFile/TrashDir return ErrFileInTrash if already trashed
// → should not happen because davPathToVFSPath blocks .cozy_trash prefix
// → guard with 405 if it does occur
```

### Pattern 3: MOVE via DocPatch

**What:** Rename or reparent a file or directory atomically via metadata update.
**When to use:** MOVE handler.

```go
// Source: model/vfs/file.go:306-382 (ModifyFileMetadata)
// Source: model/vfs/directory.go:232-288 (ModifyDirMetadata)
// Source: model/vfs/permissions_test.go:64-75 (DocPatch construction)

newName := path.Base(dstVFSPath)
dstParent, err := inst.VFS().DirByPath(path.Dir(dstVFSPath))
if err != nil {
    if errors.Is(err, os.ErrNotExist) {
        return sendWebDAVError(c, http.StatusConflict, "conflict")
    }
    return mapVFSError(c, err)
}
newDirID := dstParent.ID()

patch := &vfs.DocPatch{
    Name:  &newName,
    DirID: &newDirID,
}

if file != nil {
    _, err = vfs.ModifyFileMetadata(inst.VFS(), file, patch)
} else {
    _, err = vfs.ModifyDirMetadata(inst.VFS(), dir, patch)
}
```

### Pattern 4: Destination Header Parsing

**What:** Extract, decode, and validate the target path from the RFC 4918 `Destination` header.
**When to use:** MOVE handler.

```go
// Source: model/vfs/vfs.go:492 (Mkdir validates path via DirByPath)
// Source: web/webdav/path_mapper.go:35-56 (davPathToVFSPath)

func parseDestination(c echo.Context) (string, error) {
    rawDest := c.Request().Header.Get("Destination")
    if rawDest == "" {
        return "", errMissingDestination
    }
    u, err := url.Parse(rawDest)
    if err != nil {
        return "", err
    }
    // u.Path is already URL-decoded by url.Parse
    // Strip the /dav/files prefix before passing to davPathToVFSPath
    const prefix = "/dav/files"
    if !strings.HasPrefix(u.Path, prefix) {
        return "", errInvalidDestination
    }
    param := strings.TrimPrefix(u.Path, prefix)
    return davPathToVFSPath(param)
}
```

**Important:** The `Destination` header contains a full URL (scheme + host + path) per RFC 4918 §10.3. The host in the Destination URL should match the request host; if it doesn't, return 502 Bad Gateway per RFC 4918 §9.9.4. For v1, a simplified check (reject cross-host Destinations) is sufficient.

### Pattern 5: Error Mapping

**What:** Convert VFS sentinel errors to RFC 4918 HTTP status + XML error body.
**When to use:** Every write handler.

```go
// Source: model/vfs/errors.go (all sentinels)
// Source: web/webdav/errors.go (sendWebDAVError)

func mapVFSWriteError(c echo.Context, err error) error {
    switch {
    case errors.Is(err, vfs.ErrFileTooBig), errors.Is(err, vfs.ErrMaxFileSize):
        auditLog(c, "quota exceeded", "")
        return sendWebDAVError(c, http.StatusInsufficientStorage, "quota-not-exceeded")
    case errors.Is(err, vfs.ErrParentDoesNotExist), errors.Is(err, vfs.ErrParentInTrash):
        return sendWebDAVError(c, http.StatusConflict, "conflict")
    case errors.Is(err, vfs.ErrForbiddenDocMove):
        auditLog(c, "forbidden move", "")
        return sendWebDAVError(c, http.StatusForbidden, "forbidden")
    case errors.Is(err, vfs.ErrFileInTrash):
        return sendWebDAVError(c, http.StatusMethodNotAllowed, "method-not-allowed")
    case errors.Is(err, os.ErrNotExist):
        return sendWebDAVError(c, http.StatusNotFound, "not-found")
    case errors.Is(err, os.ErrExist):
        return sendWebDAVError(c, http.StatusMethodNotAllowed, "method-not-allowed")
    default:
        return err // Echo → 500 (unexpected VFS/CouchDB error)
    }
}
```

### Pattern 6: ETag Conditional Matching for PUT

**What:** Compare the client-supplied `If-Match` or `If-None-Match` against the file's VFS ETag (MD5-based).
**When to use:** PUT handler, overwrite path only.

```go
// Source: web/webdav/xml.go:128-130 (buildETag)
// The ETag is base64(MD5Sum), double-quoted: `"<base64>"`
// Client sends the same quoted form in If-Match.

func checkETagMatch(c echo.Context, file *vfs.FileDoc) error {
    fileETag := buildETag(file.MD5Sum)  // double-quoted

    if ifMatch := c.Request().Header.Get("If-Match"); ifMatch != "" {
        if ifMatch != fileETag && ifMatch != "*" {
            return errETagMismatch
        }
    }
    if ifNoneMatch := c.Request().Header.Get("If-None-Match"); ifNoneMatch == "*" {
        // "If-None-Match: *" means "fail if any version exists"
        return errETagMismatch
    }
    return nil
}
```

**Note:** Do NOT use `web/files.CheckIfMatch` — it compares CouchDB `_rev` strings, not MD5 ETags. The WebDAV ETag is MD5-based (`buildETag`), not `_rev`. This is a Phase 1 established convention (see `xml.go:125-130`).

### Pattern 7: .cozy_trash Write-Fence

**What:** All write methods must check whether the target path is inside `.cozy_trash` before calling VFS. This check must happen before the VFS call because TrashFile/TrashDir also detect it and return ErrFileInTrash, but the HTTP status codes differ by method.
**When to use:** DELETE (→ 405), MOVE destination (→ 403), PUT (→ 403 if parent is .cozy_trash).

```go
// Source: model/vfs/vfs.go:35 (TrashDirName = "/.cozy_trash")

if vfsPath == vfs.TrashDirName || strings.HasPrefix(vfsPath, vfs.TrashDirName+"/") {
    auditLog(c, "write attempt to trash", vfsPath)
    // For DELETE inside trash: 405
    // For MOVE destination = trash: 403
    // For PUT into trash: 403
}
```

### Anti-Patterns to Avoid

- **Using `vfs.MkdirAll` for MKCOL:** Has a documented race condition in the stack (CONCERNS.md). Use `vfs.Mkdir` only — MKCOL is single-level per RFC 4918 §9.3.
- **Buffering PUT body:** Read directly from `c.Request().Body` into the VFS writer via `io.Copy`. Never `io.ReadAll` + write — defeats streaming and risks OOM on large files.
- **Calling `fs.DestroyFile`/`fs.DestroyDirAndContent` for DELETE:** These are for permanent deletion of trash contents only. Use `TrashFile`/`TrashDir`.
- **Using `vfs.Remove`/`vfs.RemoveAll` for DELETE:** These call `DestroyFile`/`DestroyDirAndContent` — confirmed hard-destroy in `vfs.go:558-586`. Do not use.
- **Comparing CouchDB `_rev` for If-Match:** The WebDAV ETag is MD5-based (`buildETag`), not `_rev`.
- **Ignoring `file.Close()` error:** The VFS only persists the write on a clean Close. A Close error means the write failed (quota overflow during streaming, storage backend error).
- **Absent `Overwrite` header default to F:** RFC 4918 §10.6 says absent = T. The `x/net/webdav` library defaults to F (bug #66059) — this implementation must explicitly default to T.
- **Parsing `Destination` as a plain path:** It is a full URL per RFC 4918 §10.3 — must use `url.Parse` before extracting the path segment.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Quota check before PUT | Custom disk-usage computation | VFS quota is checked inside `CreateFile`/during `io.Copy`; `ErrFileTooBig` raised at stream time | VFS handles progressive quota accounting |
| Trash naming conflicts | Custom conflict resolver | `TrashFile`/`TrashDir` call `tryOrUseSuffix` internally (e.g. "foo (2)") | Already in VFS |
| Path traversal guard on Destination | Custom string inspection | `davPathToVFSPath` (Phase 1) — reuse for MOVE Destination | Tested, handles double-encoding, null bytes |
| ETag computation | Custom MD5 encoder | `buildETag(file.MD5Sum)` from `xml.go` | Already standardised in Phase 1 |
| RFC 4918 error XML body | Custom XML builder | `sendWebDAVError(c, status, condition)` from `errors.go` | Established Phase 1 helper |
| Directory existence check for MKCOL | Manual DirByPath + type switch | `os.ErrExist` returned by `fs.CreateDir` when path exists | Single error check |
| Directory ID lookup for MOVE | Manual path-to-ID traversal | `inst.VFS().DirByPath(path.Dir(dstVFSPath))` returns `*DirDoc` with `.ID()` | Direct indexer call |

**Key insight:** The VFS layer has already solved quota, atomicity, conflict naming, and permission checking. The handler's job is path resolution, conditional header evaluation, method routing, and error translation — nothing else.

---

## Common Pitfalls

### Pitfall 1: Close() Error Not Checked on PUT

**What goes wrong:** The VFS write appears successful (io.Copy returns nil), but quota is exceeded or the storage backend fails during the final commit. The error surfaces only on `file.Close()`.
**Why it happens:** VFS writers are lazy — they buffer quota checks to Close time in some backends (Swift). `io.Copy` returning nil does not guarantee the write is committed.
**How to avoid:** Always pattern: `_, err = io.Copy(file, body); cerr := file.Close(); if cerr != nil && err == nil { err = cerr }`.
**Warning signs:** Files appear to be created (201 returned) but are zero-length or missing content in the VFS.

### Pitfall 2: Wrong ETag Comparison for If-Match

**What goes wrong:** Comparing client `If-Match` value against CouchDB `_rev` → always fails for well-behaved clients.
**Why it happens:** `web/files.CheckIfMatch` uses `olddoc.Rev()` (CouchDB `_rev`). WebDAV clients send the ETag they received from PROPFIND, which is MD5-based (`buildETag`).
**How to avoid:** Implement a WebDAV-specific ETag check using `buildETag(file.MD5Sum)` from `xml.go`.
**Warning signs:** Every conditional PUT returns 412 even when the client has the correct file version.

### Pitfall 3: Absent Overwrite Header Defaults to F

**What goes wrong:** MOVE fails with 412 when the destination doesn't exist, because the handler defaults to Overwrite=F.
**Why it happens:** RFC 4918 §10.6 says absent = T, but `x/net/webdav` library (bug #66059) defaults to F. If the handler copies this behavior, macOS Finder and OnlyOffice MOVE operations fail unexpectedly.
**How to avoid:** `ovr := r.Header.Get("Overwrite"); if ovr == "" { ovr = "T" }` — explicit default in the handler.
**Warning signs:** MOVE requests without the Overwrite header return 412 to well-behaved clients.

### Pitfall 4: Destination Header Treated as a Path

**What goes wrong:** `Destination: http://localhost/dav/files/foo/bar.txt` treated as a relative path → path-mapper receives the full URL string → rejects it or maps it incorrectly.
**Why it happens:** RFC 4918 §10.3 requires Destination to be an absolute URI. Some implementations skip `url.Parse` and treat it as a path.
**How to avoid:** Always `url.Parse(rawDest)` first, extract `.Path`, then strip the `/dav/files` prefix before passing to `davPathToVFSPath`.
**Warning signs:** MOVE returns 403 or 404 on all requests regardless of target.

### Pitfall 5: DELETE Response Code

**What goes wrong:** DELETE returns 200 OK with a body instead of 204 No Content.
**Why it happens:** RFC 4918 §9.6.1 states 204 for a successful DELETE on a resource. Some implementations return 200 + body.
**How to avoid:** `return c.NoContent(http.StatusNoContent)` (204) after TrashFile/TrashDir succeeds.
**Warning signs:** macOS Finder shows "operation not supported" after successful DELETE.

### Pitfall 6: MKCOL 405 vs 409 Confusion

**What goes wrong:** MKCOL on a path where the *parent* doesn't exist returns 405 instead of 409, or vice versa.
**Why it happens:** Both conditions look like "can't create directory here." RFC 4918 §9.3.1 is explicit: parent missing = 409 Conflict, path itself exists = 405 Method Not Allowed.
**How to avoid:** `vfs.Mkdir` returns `ErrParentDoesNotExist` for missing parent and `os.ErrExist` (from `fs.CreateDir`) for existing path — map them separately.
**Warning signs:** Windows Explorer shows misleading errors when creating directories.

### Pitfall 7: MOVE into .cozy_trash Succeeds

**What goes wrong:** A client MOVEs a file directly to `/.cozy_trash/foo`, bypassing the soft-trash semantics and populating trash with wrong metadata (no `RestorePath`, no `Trashed=true` flag).
**Why it happens:** `vfs.ModifyFileMetadata` will accept any valid DirID as the target, including the trash DirID, if the fence check is missing.
**How to avoid:** Check destination VFS path for `.cozy_trash` prefix in the MOVE handler before calling the VFS. Return 403 Forbidden with audit log.
**Warning signs:** Files appear in trash but cannot be restored (missing `RestorePath`).

---

## Code Examples

### PUT Handler — Create or Overwrite

```go
// Source: web/files/files.go:311-322 (OverwriteFileContent pattern)
// Source: web/webdav/get_test.go:18-36 (seedFile — identical pattern in tests)

func handlePut(c echo.Context) error {
    rawParam := c.Param("*")
    vfsPath, err := davPathToVFSPath(rawParam)
    if err != nil {
        auditLog(c, "put path rejected", rawParam)
        return sendWebDAVError(c, http.StatusForbidden, "forbidden")
    }

    // Guard: no writes into .cozy_trash
    if vfsPath == vfs.TrashDirName || strings.HasPrefix(vfsPath, vfs.TrashDirName+"/") {
        auditLog(c, "put into trash rejected", vfsPath)
        return sendWebDAVError(c, http.StatusForbidden, "forbidden")
    }

    // MKCOL-with-body guard: not needed for PUT (body is the file content)
    inst := middlewares.GetInstance(c)
    fs := inst.VFS()

    olddoc, _ := fs.FileByPath(vfsPath) // nil if file does not exist

    // Conditional headers: only check if file already exists
    if olddoc != nil {
        if err := checkETagConditions(c, olddoc); err != nil {
            return sendWebDAVError(c, http.StatusPreconditionFailed, "precondition-failed")
        }
    } else {
        // If-None-Match: * on new file: no conflict (file doesn't exist, proceed)
        // If-Match on new file: fail (client asserts a specific version exists)
        ifMatch := c.Request().Header.Get("If-Match")
        if ifMatch != "" {
            return sendWebDAVError(c, http.StatusPreconditionFailed, "precondition-failed")
        }
    }

    // Build FileDoc
    name := path.Base(vfsPath)
    dirDoc, err := fs.DirByPath(path.Dir(vfsPath))
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return sendWebDAVError(c, http.StatusConflict, "conflict")
        }
        return err
    }

    contentType := c.Request().Header.Get("Content-Type")
    if contentType == "" || contentType == "application/octet-stream" {
        mime, class = vfs.ExtractMimeAndClassFromFilename(name)
    } else {
        mime, class = vfs.ExtractMimeAndClass(contentType)
    }

    newdoc, err := vfs.NewFileDoc(name, dirDoc.ID(), -1, nil, mime, class,
        time.Now(), false, false, false, nil)
    if err != nil {
        return sendWebDAVError(c, http.StatusBadRequest, "forbidden")
    }

    file, err := fs.CreateFile(newdoc, olddoc)
    if err != nil {
        return mapVFSWriteError(c, err)
    }
    _, err = io.Copy(file, c.Request().Body)
    if cerr := file.Close(); cerr != nil && err == nil {
        err = cerr
    }
    if err != nil {
        return mapVFSWriteError(c, err)
    }

    if olddoc == nil {
        return c.NoContent(http.StatusCreated) // 201
    }
    return c.NoContent(http.StatusNoContent) // 204
}
```

### DELETE Handler

```go
// Source: model/vfs/file.go:386 (TrashFile), model/vfs/directory.go:292 (TrashDir)

func handleDelete(c echo.Context) error {
    rawParam := c.Param("*")
    vfsPath, err := davPathToVFSPath(rawParam)
    if err != nil {
        auditLog(c, "delete path rejected", rawParam)
        return sendWebDAVError(c, http.StatusForbidden, "forbidden")
    }

    // DELETE inside .cozy_trash → 405
    if vfsPath == vfs.TrashDirName || strings.HasPrefix(vfsPath, vfs.TrashDirName+"/") {
        auditLog(c, "delete inside trash rejected", vfsPath)
        c.Response().Header().Set("Allow", "PROPFIND, GET, HEAD, OPTIONS")
        return sendWebDAVError(c, http.StatusMethodNotAllowed, "method-not-allowed")
    }

    inst := middlewares.GetInstance(c)
    dir, file, err := inst.VFS().DirOrFileByPath(vfsPath)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return sendWebDAVError(c, http.StatusNotFound, "not-found")
        }
        return err
    }

    if file != nil {
        _, err = vfs.TrashFile(inst.VFS(), file)
    } else {
        _, err = vfs.TrashDir(inst.VFS(), dir)
    }
    if err != nil {
        return mapVFSWriteError(c, err)
    }
    return c.NoContent(http.StatusNoContent)
}
```

### MKCOL Handler

```go
// Source: model/vfs/vfs.go:492 (Mkdir)

func handleMkcol(c echo.Context) error {
    // RFC 4918 §9.3: MKCOL with a request body → 415
    if c.Request().ContentLength > 0 {
        return sendWebDAVError(c, http.StatusUnsupportedMediaType, "unsupported-media-type")
    }

    rawParam := c.Param("*")
    vfsPath, err := davPathToVFSPath(rawParam)
    if err != nil {
        auditLog(c, "mkcol path rejected", rawParam)
        return sendWebDAVError(c, http.StatusForbidden, "forbidden")
    }

    _, err = vfs.Mkdir(middlewares.GetInstance(c).VFS(), vfsPath, nil)
    if err != nil {
        return mapVFSWriteError(c, err)
    }
    return c.NoContent(http.StatusCreated)
}
```

### MOVE Handler (Outline)

```go
// Source: model/vfs/vfs.go:339-354 (DocPatch)
// Source: model/vfs/file.go:306 (ModifyFileMetadata)
// Source: model/vfs/directory.go:232 (ModifyDirMetadata)

func handleMove(c echo.Context) error {
    // 1. Parse source path
    srcVFSPath, err := davPathToVFSPath(c.Param("*"))
    // ... error handling ...

    // 2. Parse destination path from Destination header (full URL)
    dstVFSPath, err := parseDestination(c)
    // ... error handling, check for .cozy_trash prefix → 403 ...

    // 3. Determine overwrite policy (absent = T)
    overwrite := c.Request().Header.Get("Overwrite")
    if overwrite == "" {
        overwrite = "T"
    }

    // 4. Check destination existence
    inst := middlewares.GetInstance(c)
    dstDir, dstFile, dstErr := inst.VFS().DirOrFileByPath(dstVFSPath)
    destExists := (dstErr == nil && (dstDir != nil || dstFile != nil))

    if destExists && overwrite == "F" {
        return sendWebDAVError(c, http.StatusPreconditionFailed, "precondition-failed")
    }

    // 5. Trash existing destination if Overwrite=T
    if destExists && overwrite == "T" {
        if dstFile != nil {
            _, err = vfs.TrashFile(inst.VFS(), dstFile)
        } else {
            _, err = vfs.TrashDir(inst.VFS(), dstDir)
        }
        // ... error handling ...
    }

    // 6. Resolve source
    srcDir, srcFile, err := inst.VFS().DirOrFileByPath(srcVFSPath)
    // ... error handling ...

    // 7. Compute new name + parent DirID
    newName := path.Base(dstVFSPath)
    parentDoc, err := inst.VFS().DirByPath(path.Dir(dstVFSPath))
    // ... error handling (parent not found → 409) ...
    newDirID := parentDoc.ID()

    // 8. Apply move via DocPatch
    patch := &vfs.DocPatch{Name: &newName, DirID: &newDirID}
    if srcFile != nil {
        _, err = vfs.ModifyFileMetadata(inst.VFS(), srcFile, patch)
    } else {
        _, err = vfs.ModifyDirMetadata(inst.VFS(), srcDir, patch)
    }
    // ... error handling ...

    if destExists {
        return c.NoContent(http.StatusNoContent) // 204 — replaced existing
    }
    return c.NoContent(http.StatusCreated) // 201 — new location
}
```

### dispatcher update (handlers.go)

```go
// Source: web/webdav/handlers.go (current — Phase 1)

func handlePath(c echo.Context) error {
    switch c.Request().Method {
    case "PROPFIND":
        return handlePropfind(c)
    case http.MethodGet, http.MethodHead:
        return handleGet(c)
    case http.MethodPut:      // Phase 2
        return handlePut(c)
    case http.MethodDelete:   // Phase 2
        return handleDelete(c)
    case "MKCOL":             // Phase 2
        return handleMkcol(c)
    case "MOVE":              // Phase 2
        return handleMove(c)
    default:
        return sendWebDAVError(c, http.StatusNotImplemented, "not-implemented")
    }
}
```

### Allow header update (webdav.go)

```go
// Current Phase 1 value:
const davAllowHeader = "OPTIONS, PROPFIND, GET, HEAD"

// Phase 2 value (add write methods):
const davAllowHeader = "OPTIONS, PROPFIND, GET, HEAD, PUT, DELETE, MKCOL, MOVE"
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `fs.DestroyFile` for DELETE | `vfs.TrashFile` (soft-trash) | Phase 2 decision | Users can recover deleted files |
| x/net/webdav Overwrite=F default | Explicit Overwrite=T default in handler | Phase 2 design | Fixes bug #66069; MOVE works correctly |
| `CheckIfMatch` using CouchDB `_rev` | Custom ETag check using MD5 `buildETag` | Phase 1 established ETag convention | Correct RFC 7232 semantics |

**Deprecated in this codebase (for WebDAV writes):**
- `vfs.Remove` / `vfs.RemoveAll`: These hard-destroy. Never use for WebDAV DELETE.
- `vfs.MkdirAll`: Race condition. Never use for MKCOL.
- `web/files.CheckIfMatch`: Uses CouchDB `_rev`, not WebDAV ETag.

---

## Open Questions

1. **MKCOL with empty body (Content-Length: 0) vs no Content-Length header**
   - What we know: RFC 4918 §9.3 says "a MKCOL request message may contain a message body". If body present → 415.
   - What's unclear: Should `Content-Length: 0` trigger the 415 check? Technically an empty body has been sent.
   - Recommendation: Only reject if `Content-Length > 0`. Zero means "empty body", which is normal for MKCOL. The current check `if c.Request().ContentLength > 0` handles this correctly.

2. **MOVE response code when source == destination**
   - What we know: RFC 4918 §9.9.4 says this should work (it's a no-op rename to same path).
   - What's unclear: Return 204 (no-op) or 201 (created)?
   - Recommendation: 204 No Content — source "already exists" at destination. Low priority; skip edge-case test in v1.

3. **Permission check on PUT overwrite**
   - What we know: Phase 1 uses `middlewares.AllowVFS(c, permission.GET, fileDoc)` for reads.
   - What's unclear: Should PUT call `middlewares.AllowVFS(c, permission.PUT, fileDoc)`? This is required for shared cozy scenarios.
   - Recommendation: Yes — add `middlewares.AllowVFS(c, permission.PUT, ...)` for both create and overwrite. The token has `io.cozy.files` scope (from `testutil_test.go`) which covers PUT.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing + testify (stretchr/testify v1) |
| Config file | none — `go test ./web/webdav/... -count=1 -timeout 5m` |
| Quick run command | `COZY_COUCHDB_URL=http://localhost:5984 go test ./web/webdav/... -count=1 -run TestPut -timeout 2m` |
| Full suite command | `COZY_COUCHDB_URL=http://localhost:5984 go test ./web/webdav/... -count=1 -timeout 5m` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| WRITE-01 | PUT streams body, no full buffer | integration | `go test ./web/webdav/... -run TestPut_Streaming` | ❌ Wave 0 |
| WRITE-02 | PUT creates or overwrites | integration | `go test ./web/webdav/... -run TestPut_Create\|TestPut_Overwrite` | ❌ Wave 0 |
| WRITE-03 | PUT If-Match / If-None-Match honored | unit+integration | `go test ./web/webdav/... -run TestPut_IfMatch\|TestPut_IfNoneMatch` | ❌ Wave 0 |
| WRITE-04 | PUT missing parent → 409 | integration | `go test ./web/webdav/... -run TestPut_MissingParent` | ❌ Wave 0 |
| WRITE-05 | DELETE file → trash | integration | `go test ./web/webdav/... -run TestDelete_File` | ❌ Wave 0 |
| WRITE-06 | DELETE dir → trash tree | integration | `go test ./web/webdav/... -run TestDelete_Dir` | ❌ Wave 0 |
| WRITE-07 | MKCOL creates single dir | integration | `go test ./web/webdav/... -run TestMkcol_Create` | ❌ Wave 0 |
| WRITE-08 | MKCOL missing parent → 409 | integration | `go test ./web/webdav/... -run TestMkcol_MissingParent` | ❌ Wave 0 |
| WRITE-09 | MKCOL existing path → 405 | integration | `go test ./web/webdav/... -run TestMkcol_Existing` | ❌ Wave 0 |
| MOVE-01 | MOVE file rename/reparent | integration | `go test ./web/webdav/... -run TestMove_File` | ❌ Wave 0 |
| MOVE-02 | MOVE dir rename/reparent | integration | `go test ./web/webdav/... -run TestMove_Dir` | ❌ Wave 0 |
| MOVE-03 | Absent Overwrite treated as T | integration | `go test ./web/webdav/... -run TestMove_OverwriteDefault` | ❌ Wave 0 |
| MOVE-04 | Overwrite: F + existing → 412 | integration | `go test ./web/webdav/... -run TestMove_OverwriteFalse` | ❌ Wave 0 |
| MOVE-05 | Destination decoded + validated | unit | `go test ./web/webdav/... -run TestMove_DestinationParsing` | ❌ Wave 0 |
| TEST-03 | gowebdav write integration | integration | `go test ./web/webdav/... -run TestE2E_GowebdavWrite` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `COZY_COUCHDB_URL=http://localhost:5984 go test ./web/webdav/... -count=1 -run Test<MethodUnderTest> -timeout 2m`
- **Per wave merge:** `COZY_COUCHDB_URL=http://localhost:5984 go test ./web/webdav/... -count=1 -timeout 5m`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `web/webdav/put_test.go` — covers WRITE-01, WRITE-02, WRITE-03, WRITE-04
- [ ] `web/webdav/delete_test.go` — covers WRITE-05, WRITE-06
- [ ] `web/webdav/mkcol_test.go` — covers WRITE-07, WRITE-08, WRITE-09
- [ ] `web/webdav/move_test.go` — covers MOVE-01, MOVE-02, MOVE-03, MOVE-04, MOVE-05
- [ ] `web/webdav/write_integration_test.go` — covers TEST-03 (gowebdav Phase 2 success criteria)
- [ ] Implementation files: `put.go`, `delete.go`, `mkcol.go`, `move.go` (RED tests written first, then GREEN)

*(No new test framework needed — existing `go test` + `testutil_test.go` harness covers all requirements.)*

---

## Sources

### Primary (HIGH confidence)
- `/home/ben/Dev-local/cozy-stack-feat-webdav/model/vfs/file.go` — `TrashFile`, `ModifyFileMetadata`, `NewFileDoc`, `CreateFile` pattern
- `/home/ben/Dev-local/cozy-stack-feat-webdav/model/vfs/directory.go` — `TrashDir`, `ModifyDirMetadata`
- `/home/ben/Dev-local/cozy-stack-feat-webdav/model/vfs/vfs.go` — `Mkdir`, `DocPatch`, `ExtractMimeAndClass`, `ErrFileTooBig`, `CheckAvailableDiskSpace`
- `/home/ben/Dev-local/cozy-stack-feat-webdav/model/vfs/errors.go` — Full error sentinel list
- `/home/ben/Dev-local/cozy-stack-feat-webdav/web/files/files.go` — `OverwriteFileContent` (lines 253-322): canonical CreateFile+io.Copy pattern
- `/home/ben/Dev-local/cozy-stack-feat-webdav/web/webdav/handlers.go` — dispatcher structure
- `/home/ben/Dev-local/cozy-stack-feat-webdav/web/webdav/errors.go` — `sendWebDAVError`, `buildErrorXML`
- `/home/ben/Dev-local/cozy-stack-feat-webdav/web/webdav/path_mapper.go` — `davPathToVFSPath`
- `/home/ben/Dev-local/cozy-stack-feat-webdav/web/webdav/xml.go` — `buildETag` (line 128)
- `/home/ben/Dev-local/cozy-stack-feat-webdav/web/webdav/get.go` — error mapping pattern
- `/home/ben/Dev-local/cozy-stack-feat-webdav/web/webdav/testutil_test.go` — test harness
- `/home/ben/Dev-local/cozy-stack-feat-webdav/web/webdav/get_test.go` — `seedFile` helper
- `/home/ben/Dev-local/cozy-stack-feat-webdav/web/webdav/propfind_test.go` — `seedDir` helper
- `.planning/phases/02-write-operations/02-CONTEXT.md` — all locked decisions

### Secondary (MEDIUM confidence)
- RFC 4918 §9.3 (MKCOL), §9.6 (DELETE), §9.7 (PUT), §9.9 (MOVE), §10.3 (Destination), §10.6 (Overwrite) — applied from prior research (01-RESEARCH.md references)
- RFC 7232 §2.3 (ETag), §3.1 (If-Match), §3.2 (If-None-Match) — conditional request semantics
- `x/net/webdav` GitHub issue #66059 — Overwrite-absent defaults to F (confirmed in 01-RESEARCH.md)

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all packages verified in the live codebase
- Architecture: HIGH — all VFS function signatures and return types read directly from source
- Pitfalls: HIGH — most derived from direct code inspection (error types, Close semantics, ETag format); x/net/webdav bug confirmed in Phase 1 research

**Research date:** 2026-04-05
**Valid until:** 2026-05-05 (VFS API is stable; this is a production codebase)

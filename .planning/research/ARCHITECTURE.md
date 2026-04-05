# Architecture Research

**Domain:** WebDAV API layer on top of an existing VFS abstraction (Go monolith)
**Researched:** 2026-04-04
**Confidence:** HIGH (derived entirely from direct inspection of the cozy-stack source tree)

---

## Standard Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  WebDAV Clients (iOS Files, OnlyOffice Mobile, Cyberduck, etc.)     │
│  PROPFIND · GET · PUT · DELETE · MKCOL · COPY · MOVE · OPTIONS      │
└───────────────────────────┬─────────────────────────────────────────┘
                            │ HTTP (Basic Auth or Bearer)
┌───────────────────────────▼─────────────────────────────────────────┐
│  web/webdav/  (new package)                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │
│  │ auth.go      │  │ handlers.go  │  │ xml.go       │               │
│  │ BasicAuth    │  │ PROPFIND     │  │ multistatus  │               │
│  │ Bearer auth  │  │ GET / HEAD   │  │ prop structs │               │
│  │ → permission │  │ PUT          │  │ RFC 4918     │               │
│  │   resolution │  │ DELETE       │  └──────────────┘               │
│  └──────┬───────┘  │ MKCOL        │                                  │
│         │          │ COPY / MOVE  │                                  │
│         │          │ OPTIONS      │                                  │
│         │          └──────┬───────┘                                  │
│         │                 │                                          │
│  ┌──────▼─────────────────▼──────────────────────────────────┐      │
│  │  path_mapper.go                                            │      │
│  │  /dav/files/<path> → vfs path (/Documents/foo.txt)        │      │
│  │  URL-decode · Unicode normalize · block hidden dirs       │      │
│  └──────────────────────────────┬─────────────────────────────┘      │
└─────────────────────────────────│───────────────────────────────────┘
                                  │ calls into existing model layer
┌─────────────────────────────────▼───────────────────────────────────┐
│  Existing cozy-stack (unchanged)                                     │
│                                                                      │
│  web/middlewares/  model/permission/  model/oauth/                   │
│  NeedInstance · GetPermission · ParseJWT · vfs.Allows               │
│                                                                      │
│  model/vfs/  (VFS interface)                                         │
│  DirOrFileByPath · OpenFile · CreateFile · CreateDir                 │
│  ModifyFileMetadata · ModifyDirMetadata · CopyFile                   │
│  DirBatch / DirIterator · DestroyFile / TrashFile                   │
│                                                                      │
│  ┌─────────────────────┐  ┌───────────────────────┐                 │
│  │ model/vfs/vfsafero/ │  │ model/vfs/vfsswift/   │                 │
│  │ (dev / local)       │  │ (production / Swift)  │                 │
│  └─────────────────────┘  └───────────────────────┘                 │
│                                                                      │
│  pkg/couchdb/  (file metadata index)                                 │
│  pkg/limits/   (rate limiting)                                       │
└─────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | File(s) |
|-----------|----------------|---------|
| Route registration | Register `/dav/files/*` and `/remote.php/webdav` redirect on Echo | `web/routing.go` (1 line) + `web/webdav/routes.go` |
| Auth middleware | Extract Basic Auth password or Bearer token, resolve to `*permission.Permission` | `web/webdav/auth.go` (new) calls `web/middlewares/permissions.go` |
| Path mapper | Strip `/dav/files` prefix, URL-decode, normalize Unicode, block system dirs | `web/webdav/path_mapper.go` (new) |
| Request dispatcher | Route by HTTP method to the right handler function | `web/webdav/handlers.go` (new) |
| XML builder | Produce RFC 4918 `207 Multi-Status` XML responses | `web/webdav/xml.go` (new) |
| VFS delegation | All read/write/rename/delete operations via `inst.VFS()` | handlers call `model/vfs` directly — no new model package needed |
| Error translator | Map VFS errors to WebDAV HTTP status codes | `web/webdav/errors.go` (new) |

---

## Recommended Project Structure

```
web/webdav/
├── webdav.go          # package doc comment; Routes(*echo.Group)
├── auth.go            # resolveWebDAVAuth(c) — Basic or Bearer → *permission.Permission
├── path_mapper.go     # davPathToVFSPath(rawParam) string
├── handlers.go        # Options, PropFind, Get, Head, Put, Delete, MkCol, Copy, Move
├── xml.go             # RFC 4918 structs: Multistatus, Response, Propstat, Prop
├── errors.go          # wrapWebDAVError(err) → HTTP status + DAV error XML body
└── webdav_test.go     # integration tests using httptest + in-mem VFS
```

No `model/webdav/` package is needed. The handlers are thin: they map HTTP verbs to existing `vfs.*` functions. Business logic already lives in `model/vfs/`. Any new VFS helper needed for WebDAV efficiency (e.g., batched listing) should be added to `model/vfs/`, not to `web/webdav/`.

### Structure Rationale

- `web/webdav/` mirrors every other API handler package (`web/files/`, `web/notes/`, etc.)
- Separation into small files keeps each concern reviewable in isolation
- No `model/webdav/` because there is no new business logic — only protocol translation
- `webdav_test.go` co-located (stack convention) — uses `tests/testutils/` helpers

---

## Data Flow

### Full Request Flow

```
WebDAV client
  │
  │  e.g. PROPFIND /dav/files/Documents HTTP/1.1
  │  Authorization: Basic <base64(user:token)>
  │  Depth: 1
  ▼
Echo main router (web/server.go)
  │
  ├─ middleware: NeedInstance        (web/middlewares/instance.go)
  │  → resolves instance from Host header
  │
  ├─ middleware: resolveWebDAVAuth   (web/webdav/auth.go)  ← NEW
  │  → extracts token from Basic-Auth password field or Bearer header
  │  → calls ParseJWT / GetForOauth  (web/middlewares/permissions.go)
  │  → stores *permission.Permission in echo context
  │  → on failure: 401 with WWW-Authenticate: Basic realm="Cozy"
  │
  ├─ middleware: CheckInstanceBlocked / CheckInstanceDeleting
  │
  ▼
  handler: PropFind (web/webdav/handlers.go)  ← NEW
  │
  ├─ davPathToVFSPath(c.Param("*"))            (web/webdav/path_mapper.go)
  │  → "/dav/files/Documents" → "/Documents"
  │
  ├─ inst.VFS().DirOrFileByPath("/Documents")  (model/vfs — existing)
  │  → *DirDoc or *FileDoc
  │
  ├─ vfs.Allows(inst.VFS(), pdoc.Permissions, permission.GET, dirDoc)
  │  (model/vfs/permissions.go — existing)
  │
  ├─ if Depth:1 — paginated batch fetch:
  │    inst.VFS().DirBatch(dirDoc, cursor)      (model/vfs/couchdb_indexer.go — existing)
  │    repeat until cursor exhausted OR limit reached
  │
  ▼
  buildMultistatus(dirDoc, children)            (web/webdav/xml.go) ← NEW
  │
  ▼
  c.Response() — 207 Multi-Status XML
  (Content-Type: application/xml; charset=utf-8)
```

### Auth Sub-Flow: Basic Auth with OAuth Token as Password

```
Authorization: Basic <base64("anything:<oauth_access_token>")>
                                        │
                                        ▼
web/webdav/auth.go: resolveWebDAVAuth()
  req.BasicAuth() → (username, password, ok)
  password = the OAuth access token (JWT)
                        │
                        ▼
  middlewares.ParseJWT(c, inst, password)
  → ExtractClaims → validates JWT signature + expiry + issuer
  → oauth.FindClient(inst, claims.Subject)  — verifies client not revoked
  → GetForOauth(inst, claims, client)        — builds *permission.Permission
                        │
                        ▼
  c.Set("permissions_doc", pdoc)
  c.Set("instance", inst)
  → next handler
```

The key insight: `GetRequestToken` in `web/middlewares/permissions.go` (line 63-75) already handles both Bearer and Basic-Auth-as-password. The WebDAV auth middleware can reuse this logic directly — `_, pass, _ := req.BasicAuth()` already extracts the token from Basic Auth password. No new token extraction code is needed; the new code only needs to call `ParseJWT` and set the permission on the context, then emit a WebDAV-appropriate 401 (with `WWW-Authenticate: Basic realm="Cozy"` instead of a JSON:API error).

### Auth Sub-Flow: Bearer Token (OAuth)

```
Authorization: Bearer <oauth_access_token>
                          │
                          ▼
  same ParseJWT path as above — identical to the existing files API
```

### PUT (large file upload) Flow

```
WebDAV client
  PUT /dav/files/Documents/photo.jpg HTTP/1.1
  Content-Length: 45000000
  Content-Type: image/jpeg
  │
  ▼
handler: Put (web/webdav/handlers.go)
  │
  ├─ path → parent dir path + filename
  ├─ inst.VFS().DirByPath(parentPath)          → *DirDoc (or 409 Conflict)
  ├─ permission check: vfs.Allows(..., PATCH/POST, ...)
  │
  ├─ olddoc, _ := inst.VFS().FileByPath(fullPath)
  │  (nil if new file, non-nil if overwrite)
  │
  ├─ newdoc := vfs.NewFileDoc(name, parentID, -1, nil, mime, class, now, ...)
  │
  ├─ file, err := inst.VFS().CreateFile(newdoc, olddoc)
  │  ← returns a vfs.File (io.WriteCloser backed by Swift or afero)
  │
  ├─ io.Copy(file, c.Request().Body)
  │  ← streams directly from request body to storage — NO buffering in RAM
  │
  ├─ file.Close()  ← flushes + commits to Swift, updates CouchDB metadata
  │
  ▼
  201 Created  (or 204 No Content if overwrite)
```

`CreateFile` already handles both create and overwrite atomically. The old doc must be passed for overwrite so the indexer can update the revision.

### GET (large file download) Flow

```
handler: Get
  ├─ fileDoc := inst.VFS().FileByPath(vfsPath)
  ├─ permission check
  ├─ vfs.ServeFileContent(inst.VFS(), fileDoc, nil, "", "", req, resp)
  │   → sets Content-Type, ETag, calls http.ServeContent(w, req, name, modTime, content)
  │   → http.ServeContent handles Range requests, conditional GETs, chunked transfer
  │   → content is an io.ReadSeeker from fs.OpenFile(doc) — streams from Swift/disk
  ▼
  200 OK (streamed body) or 206 Partial Content
```

`vfs.ServeFileContent` (`model/vfs/file.go` line 251) already does exactly what WebDAV needs. Reuse it directly.

---

## Path Mapping Strategy

### Mapping Rule

```
WebDAV URL path          VFS path
/dav/files               /   (root)
/dav/files/              /
/dav/files/Documents     /Documents
/dav/files/A/B/C.txt     /A/B/C.txt
```

Strip `/dav/files` prefix. The remainder is the VFS absolute path. VFS paths always start with `/`.

### Implementation (`web/webdav/path_mapper.go`)

```go
// davPathToVFSPath converts a raw Echo wildcard param to a VFS absolute path.
// It URL-decodes each segment, rejects paths that escape to hidden system dirs,
// and returns "/" for an empty remainder (root).
func davPathToVFSPath(rawParam string) (string, error) {
    // rawParam is the "*" capture from Echo, already stripped of the prefix
    // Echo does NOT double-decode, so we decode manually.
    decoded, err := url.PathUnescape(rawParam)
    if err != nil {
        return "", vfs.ErrIllegalPath
    }
    // path.Clean removes double slashes, resolves ".." — prevents traversal.
    clean := path.Clean("/" + decoded)
    if err := validateVFSPath(clean); err != nil {
        return "", err
    }
    return clean, nil
}

// validateVFSPath rejects paths into cozy internal directories that must not
// be visible to WebDAV clients.
var hiddenPrefixes = []string{
    vfs.TrashDirName,       // /.cozy_trash
    vfs.ThumbsDirName,      // /.thumbs
    vfs.WebappsDirName,     // /.cozy_apps
    vfs.KonnectorsDirName,  // /.cozy_konnectors
    vfs.OrphansDirName,     // /.cozy_orphans
    vfs.VersionsDirName,    // /.cozy_versions
}
```

### Edge Cases

| Case | Handling |
|------|---------|
| Trailing slash (`/Documents/`) | `path.Clean` strips it; both `/Documents/` and `/Documents` resolve to the same `DirDoc` |
| URL-encoded chars (`%2F`, `%20`) | `url.PathUnescape` decodes before `path.Clean` |
| Unicode filenames (`café.txt`) | Go's `path.Clean` is Unicode-safe; no normalization needed at this layer — names are stored and returned as-is by VFS |
| `..` traversal | `path.Clean("/" + decoded)` always produces an absolute path starting with `/`; any `..` is resolved within the absolute path |
| Root (`/dav/files` or `/dav/files/`) | Maps to `/`, handled by `inst.VFS().DirByID(consts.RootDirID)` |
| Hidden system dirs (`.cozy_trash`, etc.) | Rejected in `validateVFSPath` with 403 Forbidden |
| Empty filename component | `path.Clean` collapses double slashes; filename validation in VFS rejects empty names |
| Names with `\0`, `\n`, `\r` | `vfs.ForbiddenFilenameChars` already rejects these at the VFS layer |

---

## PROPFIND Depth:1 — Pagination Strategy

### Problem

A folder with thousands of children cannot be fetched in a single CouchDB query without memory pressure. RFC 4918 does not define pagination for PROPFIND; clients expect a complete listing.

### Available VFS Primitives

- `vfs.DirBatch(*DirDoc, couchdb.Cursor) ([]DirOrFileDoc, error)` — fetches a page of children using a CouchDB cursor (`model/vfs/couchdb_indexer.go` line 563). The cursor tracks position via `StartKey` / `StartKeyDocID`.
- `vfs.DirIterator(*DirDoc, *IteratorOptions) DirIterator` — iterator that calls `DirBatch` internally in pages of `ByFetch` (default 10 from `IteratorOptions`).

### Recommended Approach

Use `DirIterator` with `ByFetch: 200` per page. Collect all results into a slice bounded by a hard cap (e.g., 10,000 items). Stream the XML response using `encoding/xml.Encoder` to avoid building a huge in-memory string.

```
handler PROPFIND (Depth: 1)
  ├─ dirDoc := inst.VFS().DirByPath(vfsPath)
  ├─ xml.Encoder writes opening <multistatus> to c.Response()
  ├─ iter := dirDoc.DirIterator(inst.VFS(), &IteratorOptions{ByFetch: 200})
  │   loop:
  │     d, f, err := iter.Next()
  │     if err == ErrIteratorDone → break
  │     xml.Encoder.EncodeElement(propstatFor(d or f))
  ├─ xml.Encoder writes closing </multistatus>
  └─ flush
```

This streams XML incrementally to the client as children are fetched from CouchDB. Memory is bounded by one page (200 docs).

**Hard limit:** Cap at 10,000 children per PROPFIND. Clients that need more should re-PROPFIND on subdirectories. Return a `X-Cozy-Warning` header if the cap is hit (informational, non-standard but harmless).

**Depth:0 (stat):** No iteration — just `DirOrFileByPath` + one propstat element.

**Depth:infinity:** Refuse with `403 Forbidden` and `DAV:propfind-finite-depth` condition (RFC 4918 §9.1). Protects against crawling the entire tree.

---

## COPY and MOVE Mapping

### MOVE

MOVE in WebDAV means rename and/or reparent. The destination is given by the `Destination:` header.

```
Destination: https://host/dav/files/NewName.txt
```

Map to VFS via `vfs.ModifyFileMetadata` (or `ModifyDirMetadata`) with a `DocPatch` setting the new `Name` and/or `DirID`.

```go
// Rename only (same parent)
patch := &vfs.DocPatch{Name: &newName}
vfs.ModifyFileMetadata(inst.VFS(), fileDoc, patch)

// Move to new parent
newParentDoc, _ := inst.VFS().DirByPath(destParentPath)
patch := &vfs.DocPatch{DirID: &newParentDoc.DocID}
vfs.ModifyFileMetadata(inst.VFS(), fileDoc, patch)

// Rename + reparent: set both Name and DirID in the patch
```

`ModifyDirMetadata` (`model/vfs/directory.go` line 232) handles the recursive path update for directories via `MoveDir`. No custom recursion is needed.

**Overwrite header:** If `Overwrite: T` and destination exists, delete the destination first, then move. If `Overwrite: F` and destination exists, return 412 Precondition Failed.

### COPY

```go
// File copy:
newdoc := vfs.CreateFileDocCopy(srcDoc, destParentDirID, destName)
inst.VFS().CopyFile(srcDoc, newdoc)

// Directory copy: no single VFS primitive — must walk and recursively copy.
// Use vfs.Walk(inst.VFS(), srcPath, walkFn) + CopyFile for each file.
// This is acceptable for v1; directory COPY is rare in practice.
```

`Fs.CopyFile` (`model/vfs/vfs.go` line 97) copies file binary + creates new metadata doc. `CreateFileDocCopy` (`model/vfs/vfs.go` line 913) builds the new `FileDoc`. No direct CouchDB access needed.

---

## Authentication Integration Points

### Existing Code to Reuse

| What | Location |
|------|----------|
| Extract token from Basic-Auth password field | `web/middlewares/permissions.go` `GetRequestToken` line 62 |
| Parse and validate JWT, look up OAuth client | `web/middlewares/permissions.go` `ParseJWT` line 250 |
| Build permission set from token claims | `web/middlewares/permissions.go` `GetForOauth` line 95 |
| Check permission on a file/dir | `model/vfs/permissions.go` `vfs.Allows` line 25 |
| AllowWholeType (bulk permission) | `web/middlewares/permissions.go` `AllowWholeType` line 422 |

### New Code Required (web/webdav/auth.go)

The existing `GetPermission` middleware cannot be used directly because it emits a JSON:API error response on failure. WebDAV clients expect a `401 Unauthorized` with `WWW-Authenticate: Basic realm="Cozy"` so they can prompt the user for credentials.

```go
// resolveWebDAVAuth is an Echo middleware for the WebDAV route group.
// It replaces the standard GetPermission flow with WebDAV-appropriate error responses.
func resolveWebDAVAuth(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        inst := middlewares.GetInstance(c)
        tok := middlewares.GetRequestToken(c)  // works for both Bearer and Basic
        if tok == "" {
            c.Response().Header().Set("WWW-Authenticate", `Basic realm="Cozy"`)
            return echo.NewHTTPError(http.StatusUnauthorized)
        }
        pdoc, err := middlewares.ParseJWT(c, inst, tok)
        if err != nil {
            c.Response().Header().Set("WWW-Authenticate", `Basic realm="Cozy"`)
            return echo.NewHTTPError(http.StatusUnauthorized)
        }
        middlewares.ForcePermission(c, pdoc)
        return next(c)
    }
}
```

`middlewares.GetRequestToken` already handles both `Authorization: Bearer <token>` and `Authorization: Basic <base64(user:token)>` — when Basic, it returns the password field as the token (line 69-71 of `permissions.go`). This is exactly what we need: the OAuth token is sent as the Basic Auth password.

---

## Route Registration

### In web/webdav/webdav.go

```go
// Routes registers the WebDAV handler on the given Echo group.
// The group is expected to have NeedInstance and resolveWebDAVAuth applied.
func Routes(g *echo.Group) {
    g.Any("/*", dispatch)
}
```

### In web/routing.go (SetupRoutes)

```go
// WebDAV — authenticated, XML responses
{
    mws := []echo.MiddlewareFunc{
        middlewares.NeedInstance,
        webdav.AuthMiddleware,        // new — replaces generic GetPermission
        middlewares.CheckInstanceBlocked,
        middlewares.CheckInstanceDeleting,
    }
    webdav.Routes(router.Group("/dav/files", mws...))

    // Compatibility redirect for Nextcloud-style clients
    router.GET("/remote.php/webdav", func(c echo.Context) error {
        return c.Redirect(http.StatusMovedPermanently, "/dav/files")
    })
    router.Any("/remote.php/webdav/*", func(c echo.Context) error {
        return c.Redirect(http.StatusMovedPermanently, "/dav/files/"+c.Param("*"))
    })
}
```

Note: The middleware chain does NOT include `middlewares.Accept(...)` (which enforces `application/json`) — WebDAV uses `application/xml`. The `Accept` middleware is part of the standard JSON:API middleware chain but must be omitted for the WebDAV group.

---

## XML Response Structure (RFC 4918)

The server-side XML structs needed (in `web/webdav/xml.go`) mirror the client structs already in `pkg/webdav/webdav.go` but in the opposite direction:

```go
// Multistatus is the top-level RFC 4918 response element.
type Multistatus struct {
    XMLName   xml.Name   `xml:"D:multistatus"`
    NS        string     `xml:"xmlns:D,attr"`
    Responses []Response `xml:"D:response"`
}

type Response struct {
    Href     string    `xml:"D:href"`
    Propstat Propstat  `xml:"D:propstat"`
}

type Propstat struct {
    Prop   Prop   `xml:"D:prop"`
    Status string `xml:"D:status"`
}

type Prop struct {
    DisplayName     string   `xml:"D:displayname"`
    ResourceType    *ResType `xml:"D:resourcetype,omitempty"`  // present + <D:collection/> for dirs
    ContentLength   int64    `xml:"D:getcontentlength,omitempty"`
    ContentType     string   `xml:"D:getcontenttype,omitempty"`
    LastModified    string   `xml:"D:getlastmodified"`          // RFC 1123 format
    ETag            string   `xml:"D:getetag,omitempty"`        // base64(md5sum)
    CreationDate    string   `xml:"D:creationdate,omitempty"`   // ISO 8601
}
```

Use `xml.NewEncoder(c.Response())` and write elements incrementally for large PROPFIND responses rather than marshalling the full slice at once.

---

## Error Translation

WebDAV error codes differ from JSON:API. A dedicated translator is needed:

| VFS / Go error | WebDAV HTTP status |
|---------------|--------------------|
| `os.ErrNotExist` | 404 Not Found |
| `os.ErrExist` | 409 Conflict (on MKCOL/PUT creation where parent missing) or 405 Method Not Allowed (on MKCOL where already exists) |
| `vfs.ErrParentDoesNotExist` | 409 Conflict |
| `vfs.ErrForbiddenDocMove` | 403 Forbidden |
| `vfs.ErrIllegalFilename` | 400 Bad Request |
| `vfs.ErrFileTooBig` / `vfs.ErrMaxFileSize` | 507 Insufficient Storage |
| `vfs.ErrConflict` | 409 Conflict |
| `vfs.ErrDirNotEmpty` | 409 Conflict (DELETE on non-empty dir) |
| `permission.ErrForbidden` / `ErrForbidden` | 403 Forbidden |
| `permission.ErrInvalidToken` | 401 Unauthorized |
| any other error | 500 Internal Server Error |

This lives in `web/webdav/errors.go`. Do not re-export `wrapVfsError` from `web/files/` — it returns `*jsonapi.Error` which is the wrong type. Implement a parallel `wrapDAVError(err) (int, string)`.

---

## Build Order (Phase Dependencies)

The components have clear dependencies. Build in this order:

```
1. web/webdav/xml.go           — no dependencies; pure data structures
   web/webdav/errors.go        — depends on model/vfs error sentinels only
   web/webdav/path_mapper.go   — depends on model/vfs constants only

2. web/webdav/auth.go          — depends on web/middlewares (existing)
                                 Build after path_mapper; before all handlers

3. web/webdav/webdav.go        — route registration; depends on Echo + auth middleware
   web/routing.go edit         — add webdav.Routes call + compat redirect

4. handlers: OPTIONS            — simplest; just returns Allow: header
             PROPFIND Depth:0  — stat only; tests path_mapper + VFS lookup
             PROPFIND Depth:1  — listing; tests DirIterator + XML streaming
             GET / HEAD        — file read; tests ServeFileContent reuse
             PUT               — file write; tests CreateFile streaming
             DELETE            — file delete; tests TrashFile / DestroyFile
             MKCOL             — dir create; tests vfs.Mkdir
             MOVE              — rename/reparent; tests ModifyFileMetadata
             COPY              — copy; tests CopyFile + directory walk
```

Each handler can be built and tested independently. The route registration stub can return 501 Not Implemented for unimplemented methods to allow incremental delivery.

---

## Integration Points with Existing Code

| What WebDAV needs | Existing symbol | File |
|-------------------|----------------|------|
| Instance from context | `middlewares.GetInstance(c)` | `web/middlewares/instance.go` |
| VFS access | `inst.VFS()` → `vfs.VFS` | `model/instance/instance.go:227` |
| Root dir | `inst.VFS().DirByID(consts.RootDirID)` | `pkg/consts/file.go:12` |
| Lookup by path | `inst.VFS().DirOrFileByPath(name)` | `model/vfs/vfs.go:229` |
| Paginated dir listing | `inst.VFS().DirBatch(doc, cursor)` | `model/vfs/couchdb_indexer.go:563` |
| Dir iteration | `inst.VFS().DirIterator(doc, opts)` | `model/vfs/couchdb_indexer.go:559` |
| File open for read | `inst.VFS().OpenFile(doc)` | `model/vfs/vfs.go:82` |
| Streaming file serve | `vfs.ServeFileContent(fs, doc, ...)` | `model/vfs/file.go:251` |
| File create / overwrite | `inst.VFS().CreateFile(newdoc, olddoc)` | `model/vfs/vfs.go:93` |
| Dir create | `vfs.Mkdir(inst.VFS(), path, nil)` | `model/vfs/vfs.go:493` |
| Rename / move (file) | `vfs.ModifyFileMetadata(fs, doc, patch)` | `model/vfs/file.go:306` |
| Rename / move (dir) | `vfs.ModifyDirMetadata(fs, doc, patch)` | `model/vfs/directory.go:232` |
| File copy | `inst.VFS().CopyFile(olddoc, newdoc)` | `model/vfs/vfs.go:97` |
| New file doc for copy | `vfs.CreateFileDocCopy(doc, parentID, name)` | `model/vfs/vfs.go:913` |
| Soft delete (trash) | `vfs.TrashFile(fs, doc)` / `vfs.TrashDir(fs, doc)` | `model/vfs/file.go:386`, `directory.go:292` |
| Hard delete | `fs.DestroyFile(doc)` / `fs.DestroyDirAndContent(dir, ...)` | `model/vfs/vfs.go:108-113` |
| Permission check | `vfs.Allows(fs, pdoc.Permissions, verb, fetcher)` | `model/vfs/permissions.go:25` |
| Token extraction | `middlewares.GetRequestToken(c)` | `web/middlewares/permissions.go:62` |
| JWT parsing | `middlewares.ParseJWT(c, inst, token)` | `web/middlewares/permissions.go:250` |
| Force permission on ctx | `middlewares.ForcePermission(c, pdoc)` | `web/middlewares/permissions.go:417` |
| VFS error sentinels | `vfs.ErrIllegalFilename`, `vfs.ErrFileTooBig`, etc. | `model/vfs/` various files |
| Hidden dir names | `vfs.TrashDirName`, `vfs.WebappsDirName`, etc. | `model/vfs/vfs.go:30-45` |
| Logger | `inst.Logger().WithNamespace("webdav")` | `model/instance/instance.go:221` |

---

## Architectural Patterns

### Pattern 1: Thin HTTP-to-VFS Translation Layer

**What:** WebDAV handlers contain no business logic. Each handler: (1) maps the DAV path to a VFS path, (2) fetches the VFS document, (3) checks permissions, (4) calls the VFS function, (5) writes the XML response.

**When to use:** Always. This is the core principle of the project.

**Trade-offs:** A thin handler is easy to test and maintain, but any performance bottleneck must be fixed in the VFS layer, not in the WebDAV handler.

### Pattern 2: Streaming I/O for File Content

**What:** Never buffer file content in memory. For GET: use `vfs.ServeFileContent` which calls `http.ServeContent` (streaming with range support). For PUT: pipe `c.Request().Body` directly into `fs.CreateFile` via `io.Copy`.

**When to use:** Always for file content — even small files benefit from consistent behavior.

**Trade-offs:** Streaming is more complex to test (need a real VFS backend) but prevents memory exhaustion under concurrent large-file transfers.

### Pattern 3: Incremental XML for PROPFIND

**What:** Use `xml.NewEncoder(w)` and `EncodeElement()` per child, rather than building a `[]Response` slice and calling `xml.Marshal` once.

**When to use:** PROPFIND Depth:1 with potentially large directories.

**Trade-offs:** Incremental encoding means the response has already started when an error occurs mid-stream (cannot change status code to 500). Use a pre-flight check (does the directory exist? is it accessible?) before starting the XML stream.

---

## Anti-Patterns

### Anti-Pattern 1: Accessing CouchDB Directly from WebDAV Handlers

**What people do:** Import `pkg/couchdb` and write queries against `io.cozy.files` directly in `web/webdav/`.

**Why it's wrong:** Bypasses VFS locking, quota enforcement, realtime events, and sharing indexer. Any metadata written this way will be inconsistent.

**Do this instead:** Call `inst.VFS()` methods exclusively. If a needed operation is missing from the VFS interface, add it to `model/vfs/` and all backend implementations.

### Anti-Pattern 2: Loading All Children Into Memory for PROPFIND

**What people do:** `iter.Next()` in a loop appending to `[]Response` then `xml.Marshal` the full slice.

**Why it's wrong:** A directory with 50,000 files allocates ~50 MB just for the response struct before writing a single byte.

**Do this instead:** Stream with `xml.Encoder.EncodeElement()` per child as shown in the PROPFIND flow above.

### Anti-Pattern 3: Reimplementing Auth Token Extraction

**What people do:** Write a new Basic-Auth parser in `web/webdav/auth.go`.

**Why it's wrong:** The existing `middlewares.GetRequestToken` already handles both `Bearer` and `Basic` (treating the password field as the token). Duplicating it diverges over time.

**Do this instead:** Call `middlewares.GetRequestToken(c)` directly; only the error response format needs to be WebDAV-specific (401 + `WWW-Authenticate: Basic`).

### Anti-Pattern 4: Using the Existing Accept Middleware

**What people do:** Add `middlewares.Accept(...)` to the WebDAV middleware chain.

**Why it's wrong:** The Accept middleware rejects requests that do not offer `application/json`. WebDAV clients send `application/xml` or no `Accept` header.

**Do this instead:** Omit the `Accept` middleware entirely for the `/dav/files` group. WebDAV handlers set `Content-Type: application/xml; charset=utf-8` explicitly.

### Anti-Pattern 5: Redirecting COPY/MOVE to the JSON:API

**What people do:** For COPY and MOVE, issue internal HTTP requests to the existing `/files/` JSON:API endpoints.

**Why it's wrong:** Adds roundtrip latency, couples the WebDAV handler to HTTP internals, and is harder to test.

**Do this instead:** Call `vfs.ModifyFileMetadata` / `vfs.CopyFile` directly — the same model functions that the JSON:API handlers use.

---

## Integration Points Summary

### External Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| WebDAV client ↔ web/webdav | HTTP (PROPFIND, GET, PUT, etc.) + Basic/Bearer auth | RFC 4918 compliance required |
| web/webdav ↔ web/middlewares | Direct Go function calls (`GetRequestToken`, `ForcePermission`, `GetInstance`) | Existing middleware functions; no new interfaces |
| web/webdav ↔ model/vfs | Direct Go function calls via `inst.VFS()` interface | All file operations must go through VFS |
| model/vfs ↔ CouchDB | pkg/couchdb HTTP client (internal to VFS) | WebDAV layer never touches CouchDB directly |
| model/vfs ↔ Swift / afero | Fs interface backends | Transparent to WebDAV layer |

### Compat Redirect

`/remote.php/webdav/*` → 301 → `/dav/files/*`

This is a simple route in `web/routing.go`. It does not need auth — the redirect preserves the Authorization header. The client will retry the WebDAV request against `/dav/files/*` with its existing credentials.

---

## Sources

All findings are HIGH confidence — derived directly from source code inspection of the `feat/webdav` branch of cozy-stack.

- `model/vfs/vfs.go` — VFS and Indexer interfaces
- `model/vfs/file.go` — FileDoc, ServeFileContent, ModifyFileMetadata, TrashFile
- `model/vfs/directory.go` — DirDoc, ModifyDirMetadata, TrashDir
- `model/vfs/couchdb_indexer.go` — DirBatch, DirIterator implementations
- `model/vfs/permissions.go` — vfs.Allows permission check
- `web/middlewares/permissions.go` — GetRequestToken, ParseJWT, GetForOauth, ForcePermission
- `web/middlewares/instance.go` — NeedInstance, GetInstance
- `web/middlewares/basic_auth.go` — existing Basic Auth pattern for admin
- `web/files/files.go` — reference implementation for VFS-backed handler patterns
- `web/routing.go` — middleware chain composition, route group registration pattern
- `pkg/webdav/webdav.go` — existing WebDAV client XML structs (reference for server-side XML)
- `pkg/consts/file.go` — RootDirID, consts.Files doctype
- `model/instance/instance.go` — inst.VFS(), inst.Logger()

---

*Architecture research for: Cozy WebDAV API layer*
*Researched: 2026-04-04*

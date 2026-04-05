# Feature Research: WebDAV API — Method and Property Requirements

**Domain:** WebDAV server — RFC 4918 pragmatic subset
**Researched:** 2026-04-04
**Confidence:** MEDIUM (wire-level client captures not available; derived from RFC 4918, sabre/dav client docs, Nextcloud compatibility reports, OnlyOffice community issues)

---

## Clarification: iOS Files App vs. iOS/iPadOS Context

The iOS/iPadOS native "Files" app does NOT support WebDAV natively. It only supports SMB connections. WebDAV access on iOS requires either:

1. A third-party app that registers a File Provider Extension (FileBrowser Professional, etc.)
2. A standalone WebDAV client app (WebDAV Navigator, WebDAV Manager, Documents by Readdle)
3. **OnlyOffice Documents iOS/Android app** — which has its own built-in WebDAV client UI

The PROJECT.md target "iOS/iPadOS Files app" most likely refers to:
- The **OnlyOffice Documents iOS app** connecting via its built-in WebDAV client
- Possibly a future Cozy Drive iOS app using a File Provider Extension backed by WebDAV

This document researches what both OnlyOffice mobile (primary target) and generic iOS WebDAV clients require.

---

## Feature Landscape

### Table Stakes — Any Conforming WebDAV Client

These are the methods and behaviors a server MUST implement for ANY WebDAV client to function. RFC 4918 Class 1 compliance is the baseline.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `OPTIONS` response with `DAV: 1` header | Every WebDAV client sends OPTIONS first to discover capabilities; `DAV: 1` signals Class 1 compliance | LOW | Must include `Allow:` listing supported methods |
| `PROPFIND` with Depth: 0 and Depth: 1 | Directory listing and resource metadata; core of WebDAV | MEDIUM | Must return 207 Multi-Status with XML body |
| `PROPFIND` allprop and specific-prop requests | Clients use both; empty body = allprop per RFC | MEDIUM | Must handle absent body, `<allprop/>`, and `<prop>` variants |
| `GET` file download | Retrieve file content | LOW | Standard HTTP, must include ETag and Last-Modified headers |
| `HEAD` request | Clients probe existence and metadata without download | LOW | Same headers as GET but no body |
| `PUT` file upload | Write file content | LOW | Must return 201 Created or 204 No Content; must handle chunked Transfer-Encoding |
| `DELETE` file or folder | Remove resource | LOW | Recursive delete of collections per RFC; 204 No Content |
| `MKCOL` create folder | Create directory/collection | LOW | 201 Created on success; 405 if resource exists |
| `COPY` copy resource | Required by most file managers | MEDIUM | Requires `Destination:` header; `Overwrite: T/F` support; 207 for partial failures |
| `MOVE` move/rename resource | Required by most file managers | MEDIUM | Same as COPY but deletes source; 207 for partial failures |
| `207 Multi-Status` XML response | PROPFIND, COPY, MOVE, DELETE responses | MEDIUM | Must be well-formed XML in `DAV:` namespace with proper prefixing |
| Standard PROPFIND live properties | Clients expect these to be populated | MEDIUM | See property list below |
| `ETag` header on GET/HEAD/PUT responses | Cache validation; conditional requests | LOW | Must be a strong ETag (not weak); required by rclone and syncing clients |
| `Content-Length` header on all responses | macOS Finder will produce "strange results" if absent | LOW | macOS Finder specifically requires this on every file response |
| HTTPS support | Authentication over Basic Auth requires TLS | LOW | Already handled by cozy-stack infrastructure |
| Authentication: Basic Auth (app passwords) | Primary auth for WebDAV clients that don't support OAuth | LOW | Cozy app-specific passwords mechanism |

### PROPFIND Live Properties — Minimum Required Set

Every conforming server must return these properties. Clients rely on them for display, caching, and navigation.

| Property (DAV: namespace) | Type | Required For | Notes |
|--------------------------|------|--------------|-------|
| `DAV:resourcetype` | Live | ALL clients | `<collection/>` for folders, empty for files. Most critical property. |
| `DAV:getlastmodified` | Live | ALL clients | HTTP date format (RFC 1123/7231). Finder requires UTC. |
| `DAV:getcontentlength` | Live | ALL clients | Byte count for files. MUST be returned for files. |
| `DAV:getetag` | Live | ALL clients | Strong ETag. Required for conditional requests and sync. |
| `DAV:getcontenttype` | Live | ALL clients | MIME type. `application/octet-stream` fallback for unknowns. |
| `DAV:displayname` | Live | Most clients | Human-readable name. MAY differ from URL segment. |
| `DAV:creationdate` | Live | Some clients | ISO 8601 format. Not all servers populate this. |
| `DAV:supportedlock` | Live | Class 2 servers only | Return empty `<supportedlock/>` on Class 1 servers. |
| `DAV:lockdiscovery` | Live | Class 2 servers only | Return empty `<lockdiscovery/>` on Class 1 servers. |

**Wire example — PROPFIND Depth: 1 response structure:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<D:multistatus xmlns:D="DAV:">
  <D:response>
    <D:href>/dav/files/</D:href>
    <D:propstat>
      <D:prop>
        <D:resourcetype><D:collection/></D:resourcetype>
        <D:displayname>files</D:displayname>
        <D:getlastmodified>Mon, 07 Apr 2025 10:00:00 GMT</D:getlastmodified>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
  <D:response>
    <D:href>/dav/files/document.docx</D:href>
    <D:propstat>
      <D:prop>
        <D:resourcetype/>
        <D:getcontentlength>12345</D:getcontentlength>
        <D:getcontenttype>application/vnd.openxmlformats-officedocument.wordprocessingml.document</D:getcontenttype>
        <D:getetag>"abc123def456"</D:getetag>
        <D:getlastmodified>Mon, 07 Apr 2025 09:00:00 GMT</D:getlastmodified>
        <D:displayname>document.docx</D:displayname>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
</D:multistatus>
```

**Critical XML namespace note:** Windows Mini-Redirector cannot handle default namespace declarations (bare `xmlns="DAV:"`). All elements MUST use a `D:` prefix with `xmlns:D="DAV:"` on the root element. This is the safe cross-client format.

---

### OnlyOffice Mobile — Specific Requirements (iOS + Android)

Confidence: MEDIUM — derived from sabre/dav client docs, OnlyOffice community issues, GitHub issue #349, and Android overview docs.

| Feature | Evidence | Complexity | Notes |
|---------|----------|------------|-------|
| PROPFIND for directory listing | Core operation; confirmed via community logs showing PROPFIND requests | LOW | Standard; Depth: 1 on collections |
| GET for file download/opening | Confirmed: desktop issue #349 shows "PROPFIND and GET requests are sent" | LOW | Downloads file for local editing |
| PUT for file save | Required for "upload files to WebDAV storage" capability | LOW | After local edit, PUT saves back |
| MKCOL for folder creation | "Create new folders" listed in feature set | LOW | Standard |
| DELETE for file/folder removal | "Remove files and folders" listed in feature set | LOW | Standard |
| COPY / MOVE | "Copy and move files and folders" explicitly listed | MEDIUM | Both methods needed |
| Basic Auth with app passwords | Mobile apps use username/password; OAuth not typical for generic WebDAV | LOW | Cozy app-specific passwords |
| Nextcloud-compatible URL path | App has built-in Nextcloud preset using `/remote.php/webdav/` | LOW | Redirect from `/remote.php/webdav` → `/dav/files` already planned |
| No LOCK/UNLOCK required | Confirmed via issue #349: OnlyOffice does NOT send LOCK requests for editing | LOW | Unlike MS Office, OO edits without locking — confirmed resolved in 2023 for desktop; mobile behavior similar |
| PROPPATCH (optional) | Not confirmed as required for basic operation | MEDIUM | May be used for dead property storage by some workflows |

**OnlyOffice mobile does NOT require:**
- WebDAV locking (LOCK/UNLOCK) — confirmed from issue #349 and community reports
- Any Nextcloud-specific properties (oc:fileid, oc:permissions, etc.)
- REPORT method

**Key compatibility risk:** OnlyOffice iOS 9.2 broke Nextcloud support due to username vs. full-name mismatch in auth. The cozy-stack implementation must ensure the authenticated username matches exactly what PROPFIND returns for user identity, and that subdirectory PROPFIND requests carry the same auth token as root requests.

---

### iOS/iPadOS WebDAV Client Apps — Specific Requirements

This covers apps like WebDAV Navigator, WebDAV Manager, Documents by Readdle, and any app acting as a File Provider using WebDAV as backend. These conform more strictly to RFC 4918.

| Feature | Evidence | Complexity | Notes |
|---------|----------|------------|-------|
| PROPFIND `propname` requests | Cadaver uses it; some RFC-strict clients do property discovery | LOW | Must handle `<propname/>` returning 207 with property names only (no values) |
| OPTIONS compliance check | All iOS WebDAV apps send OPTIONS before connecting | LOW | Response must include `DAV: 1` and `Allow: OPTIONS, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE, PROPFIND, PROPPATCH` |
| Proper 401 + WWW-Authenticate on unauth | Clients retry with credentials only if 401 has correct header | LOW | `WWW-Authenticate: Basic realm="Cozy"` |
| Chunked PUT bodies | macOS/iOS WebDAV clients use chunked transfer for uploads | MEDIUM | Server MUST handle `Transfer-Encoding: chunked` in PUT; many naive servers fail with 411 |
| PROPPATCH | RFC 4918 Class 1 compliance requires it | MEDIUM | Must return 207 even if no properties are writable (return 403 for read-only live props) |
| Redirect handling for compat path | `/remote.php/webdav` → `/dav/files` 301 redirect | LOW | Clients that hardcode Nextcloud path |

---

### Differentiators — Broader Client Support

Features not required for v1 targets but valuable for broader adoption.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| `rclone` compatibility | Power users sync/backup with rclone; very popular tool | LOW | rclone uses standard PROPFIND + GET/PUT/DELETE/MKCOL/MOVE; no locking needed |
| `cadaver` CLI compatibility | Developer/sysadmin debugging tool | LOW | Uses propname PROPFIND; handle empty bodies gracefully |
| macOS Finder read-only mode | Finder without locking works read-only | MEDIUM | Finder requires `Content-Length` on all responses; LOCK-less = read-only but functional |
| ETag-based conditional requests (`If-Match`, `If-None-Match`) | Correct PUT semantics; prevents lost-update conflicts for sync clients | MEDIUM | Return 412 Precondition Failed when condition fails |
| `X-OC-Mtime` header support | rclone + Nextcloud clients use this to preserve modification times | LOW | Non-standard but widely supported; allows rclone to sync mtimes |
| PROPFIND Depth: infinity (limited) | Some clients request it; should return 403 or limited depth | LOW | Return 403 Forbidden or honor with pagination — security concern if deep trees |
| `Overwrite: F` header on COPY/MOVE | RFC compliance; some clients send explicit non-overwrite | LOW | Return 412 if destination exists and Overwrite: F |
| `creationdate` property populated | Some clients display creation date | LOW | Cozy VFS tracks creation dates |

---

### Anti-Features — Deliberately NOT Build

| Anti-Feature | Why Requested | Why Problematic | What To Do Instead |
|--------------|---------------|-----------------|-------------------|
| `LOCK` / `UNLOCK` methods | macOS Finder requires them for read-write access; MS Office requires them | Cozy stack has no server-side locking mechanism; implementing fake locks creates data corruption risk | Advertise `DAV: 1` only (not `DAV: 1, 2`); accept that macOS Finder mounts read-only; OnlyOffice mobile doesn't need locks |
| `DeltaV` (RFC 3253 versioning) | "Version history would be nice" | Massive complexity; Cozy does not expose version history via VFS API | Out of scope for v1; use ETag for cache consistency |
| `CalDAV` / `CardDAV` | Contacts/Calendar sync via WebDAV | Completely different protocols; separate feature domain | Separate project |
| `quota-available-bytes` / `quota-used-bytes` | Some clients display storage quota | Cozy quota is user-level, not per-WebDAV-collection; leaks internal billing info | Return 404 Not Found for these properties in PROPFIND |
| PROPFIND Depth: infinity auto-allowed | RFC says SHOULD support it | Full-tree PROPFIND on a large account = DoS; Nextcloud blocked this for the same reason | Return 403 Forbidden for Depth: infinity; require pagination |
| Anonymous/public access | "Allow sharing public folders" | Security perimeter breaks; crawl risk | All WebDAV requests require authenticated session |
| Microsoft-specific extensions (`MS-Author-Via`, `X-MSDAVEXT`) | Windows XP web folders / SharePoint compat | Obsolete; Windows XP is EOL; modern Windows WebDAV works without these | Do not implement; return 400 if these are relied upon |
| `PROPPATCH` for live property writes | Some clients try to set `getlastmodified` | RFC says live properties controlled by server; clients should accept 403 | Return 403 Forbidden for attempts to write live properties |
| Nextcloud/ownCloud custom properties (oc:*) | OnlyOffice has Nextcloud preset | Not required for basic file browsing/editing; adds maintenance burden | Return 404 in propstat for any `oc:` namespace properties; clients gracefully ignore 404 propstats |

---

## Feature Dependencies

```
OPTIONS (advertise capabilities)
    └──required by──> ALL other methods (clients check OPTIONS first)

PROPFIND (list + metadata)
    └──required by──> Directory browsing
    └──required by──> OnlyOffice open-file flow
    └──required by──> rclone sync

GET (download)
    └──required by──> OnlyOffice file editing (download to local then edit)
    └──enhances──> ETag (conditional GET for caching)

PUT (upload)
    └──required by──> OnlyOffice save-back after editing
    └──enhances──> ETag + If-Match (safe overwrites)

MKCOL (create folder)
    └──required by──> OnlyOffice "create folder" UI

DELETE (remove)
    └──required by──> OnlyOffice "delete" UI
    └──required by──> MOVE (server-side: delete after copy)

COPY / MOVE (copy + rename)
    └──requires──> DELETE (MOVE deletes source)
    └──requires──> MKCOL (parent must exist)

PROPPATCH (write dead properties)
    └──required by──> RFC 4918 Class 1 (must implement, even if only to return 403)
    └──optional──> dead property storage

ETag (strong)
    └──required by──> Conditional requests (If-Match, If-None-Match)
    └──required by──> rclone sync correctness

Auth (Basic + Bearer)
    └──required by──> ALL methods
```

---

## MVP Definition

### Launch With (v1) — OnlyOffice Mobile + Generic WebDAV Clients

- [x] `OPTIONS` — with `DAV: 1`, `Allow:` header listing all supported methods
- [x] `PROPFIND` — Depth: 0, Depth: 1; allprop, specific-prop, propname; 207 Multi-Status XML
- [x] `GET` — file download with `ETag`, `Last-Modified`, `Content-Length`
- [x] `HEAD` — same as GET sans body
- [x] `PUT` — file upload; support `Transfer-Encoding: chunked`; return ETag on response
- [x] `DELETE` — files and folders (recursive on collections)
- [x] `MKCOL` — create folder
- [x] `COPY` — with `Destination:` and `Overwrite:` headers
- [x] `MOVE` — with `Destination:` and `Overwrite:` headers (rename + move)
- [x] `PROPPATCH` — must exist; return 403 for live props; 200/207 for dead props
- [x] Live properties: `resourcetype`, `getlastmodified`, `getcontentlength`, `getetag`, `getcontenttype`, `displayname`, `creationdate`
- [x] 401 + `WWW-Authenticate: Basic` for unauthenticated requests
- [x] Redirect `/remote.php/webdav` → `/dav/files` (301)
- [x] `Content-Length` on ALL responses (required by macOS Finder)
- [x] Namespace-prefixed XML (`xmlns:D="DAV:"` with `D:` prefix) — required by Windows Mini-Redirector

### Add After Validation (v1.x)

- [ ] `ETag`-based conditional request handling (`If-Match`, `If-None-Match`, `If-Modified-Since`) — adds sync safety; trigger: first user reports lost-update issues
- [ ] `X-OC-Mtime` header support for rclone mtime preservation — trigger: rclone user requests
- [ ] PROPFIND Depth: infinity → 403 explicit rejection with proper error body — trigger: security audit
- [ ] Pagination for large directory listings — trigger: performance testing with large account

### Future Consideration (v2+)

- [ ] macOS Finder read-write mode — requires LOCK/UNLOCK and Cozy-level file locking mechanism; major architectural change
- [ ] CalDAV / CardDAV — separate protocol; completely different feature set
- [ ] File Provider Extension for native iOS Files app — requires separate iOS app development; out of scope for cozy-stack

---

## Feature Prioritization Matrix

| Feature | Client Value | Implementation Cost | Priority |
|---------|-------------|---------------------|----------|
| PROPFIND (Depth 0+1, allprop, prop) | HIGH | MEDIUM | P1 |
| OPTIONS with DAV: 1 header | HIGH | LOW | P1 |
| GET + HEAD with ETag | HIGH | LOW | P1 |
| PUT (chunked-safe) | HIGH | LOW-MEDIUM | P1 |
| DELETE (recursive) | HIGH | LOW | P1 |
| MKCOL | HIGH | LOW | P1 |
| MOVE + COPY | HIGH | MEDIUM | P1 |
| PROPPATCH (even if mostly 403) | MEDIUM | LOW | P1 |
| Live property set (resourcetype, getetag, etc.) | HIGH | MEDIUM | P1 |
| Redirect /remote.php/webdav | MEDIUM | LOW | P1 |
| Namespace-prefixed XML responses | HIGH | LOW | P1 |
| Content-Length on all responses | HIGH | LOW | P1 |
| Conditional requests (If-Match) | MEDIUM | MEDIUM | P2 |
| X-OC-Mtime support | LOW | LOW | P2 |
| PROPFIND propname variant | LOW | LOW | P2 |
| Depth: infinity → 403 | MEDIUM | LOW | P2 |
| LOCK / UNLOCK | LOW (breaks architecture) | HIGH | NEVER (v1) |
| CalDAV / CardDAV | N/A | HIGH | NEVER (separate project) |
| quota properties | LOW | MEDIUM | NEVER (leaks billing) |

---

## Client-Specific Compatibility Matrix

| Method / Feature | OnlyOffice mobile | Generic iOS WebDAV apps | rclone | cadaver | macOS Finder | Windows WebDAV |
|------------------|:-----------------:|:-----------------------:|:------:|:-------:|:------------:|:--------------:|
| OPTIONS | Required | Required | Required | Required | Required | Required |
| PROPFIND Depth:0 | Required | Required | Required | Required | Required | Required |
| PROPFIND Depth:1 | Required | Required | Required | Required | Required | Required |
| PROPFIND allprop | Required | Required | Required | — | Required | Required |
| PROPFIND propname | — | Possible | — | Required | — | — |
| GET | Required | Required | Required | Required | Required | Required |
| HEAD | Required | Required | Required | — | Required | Required |
| PUT | Required | Required | Required | Required | Read-only* | Required |
| DELETE | Required | Required | Required | Required | Read-only* | Required |
| MKCOL | Required | Required | Required | Required | Read-only* | Required |
| COPY | Required | Possible | Required | Required | Read-only* | Required |
| MOVE | Required | Possible | Required | Required | Read-only* | Required |
| PROPPATCH | — | RFC Class 1 | — | Possible | — | — |
| LOCK / UNLOCK | NOT required | — | — | — | Required for RW | Required for RW |
| ETag strong | Expected | Expected | Required | — | Expected | — |
| Content-Length | Expected | Expected | — | — | **REQUIRED** | — |
| Chunked PUT | — | — | — | — | **REQUIRED** | — |
| DAV: 1 in OPTIONS | — | Expected | — | — | Expected | Required |
| DAV: 2 in OPTIONS | NOT needed | — | — | — | Required for RW | Required for RW |

*macOS Finder is read-only without LOCK/UNLOCK (Class 2). This is acceptable for v1.

---

## Wire-Level Request/Response Examples

### OPTIONS (server discovery)

**Request:**
```
OPTIONS /dav/files HTTP/1.1
Host: user.cozy.io
```

**Response:**
```
HTTP/1.1 200 OK
DAV: 1
Allow: OPTIONS, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE, PROPFIND, PROPPATCH
Content-Length: 0
```

### PROPFIND (directory listing)

**Request:**
```
PROPFIND /dav/files/ HTTP/1.1
Host: user.cozy.io
Depth: 1
Content-Type: application/xml; charset=utf-8

<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:">
  <D:prop>
    <D:resourcetype/>
    <D:getlastmodified/>
    <D:getcontentlength/>
    <D:getetag/>
    <D:getcontenttype/>
    <D:displayname/>
  </D:prop>
</D:propfind>
```

**Response:**
```
HTTP/1.1 207 Multi-Status
Content-Type: application/xml; charset=utf-8

<?xml version="1.0" encoding="UTF-8"?>
<D:multistatus xmlns:D="DAV:">
  <D:response>
    <D:href>/dav/files/</D:href>
    <D:propstat>
      <D:prop>
        <D:resourcetype><D:collection/></D:resourcetype>
        <D:displayname>files</D:displayname>
        <D:getlastmodified>Mon, 07 Apr 2025 10:00:00 GMT</D:getlastmodified>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
    <D:propstat>
      <D:prop>
        <D:getcontentlength/>
        <D:getetag/>
        <D:getcontenttype/>
      </D:prop>
      <D:status>HTTP/1.1 404 Not Found</D:status>
    </D:propstat>
  </D:response>
  <D:response>
    <D:href>/dav/files/report.docx</D:href>
    <D:propstat>
      <D:prop>
        <D:resourcetype/>
        <D:displayname>report.docx</D:displayname>
        <D:getlastmodified>Fri, 04 Apr 2025 08:00:00 GMT</D:getlastmodified>
        <D:getcontentlength>54321</D:getcontentlength>
        <D:getetag>"5f3b9a12c"</D:getetag>
        <D:getcontenttype>application/vnd.openxmlformats-officedocument.wordprocessingml.document</D:getcontenttype>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
</D:multistatus>
```

**Key implementation note:** Requested properties that do not exist on a resource MUST be returned in a separate `<propstat>` with `404 Not Found` status — not silently omitted and not causing the whole response to fail.

### MKCOL (create folder)

**Request:**
```
MKCOL /dav/files/NewFolder HTTP/1.1
Host: user.cozy.io
```

**Response (success):** `HTTP/1.1 201 Created`

**Response (already exists):** `HTTP/1.1 405 Method Not Allowed`

### MOVE (rename)

**Request:**
```
MOVE /dav/files/old-name.docx HTTP/1.1
Host: user.cozy.io
Destination: https://user.cozy.io/dav/files/new-name.docx
Overwrite: T
```

**Response (success, no prior destination):** `HTTP/1.1 201 Created`

**Response (success, overwrote destination):** `HTTP/1.1 204 No Content`

---

## Sources

- [RFC 4918 — HTTP Extensions for Web Distributed Authoring and Versioning](https://www.rfc-editor.org/rfc/rfc4918.html) — authoritative standard
- [sabre/dav — Finder client quirks](https://sabre.io/dav/clients/finder/) — MEDIUM confidence; documents real-world Finder behavior
- [sabre/dav — Windows client quirks](https://sabre.io/dav/clients/windows/) — MEDIUM confidence; documents XML namespace requirements
- [sabre/dav — Clients overview](https://sabre.io/dav/clients/) — MEDIUM confidence
- [Nextcloud WebDAV Basic API docs](https://docs.nextcloud.com/server/20/developer_manual/client_apis/WebDAV/basic.html) — HIGH confidence for what properties matter to clients
- [OnlyOffice Android app overview](https://helpcenter.onlyoffice.com/mobile/android/documents/overview.aspx) — MEDIUM confidence; lists supported file operations
- [OnlyOffice iOS app overview](https://helpcenter.onlyoffice.com/mobile/ios/documents/overview.aspx) — MEDIUM confidence
- [OnlyOffice community — iOS 9.2 breaks WebDAV/Nextcloud](https://community.onlyoffice.com/t/ios-app-9-2-breaks-webdav-nextcloud-support/17415) — MEDIUM confidence; real-world auth bug
- [OnlyOffice DesktopEditors issue #349 — editor won't lock documents in WebDAV](https://github.com/ONLYOFFICE/DesktopEditors/issues/349) — HIGH confidence; confirms no LOCK needed
- [golang.org/x/net webdav/prop.go](https://github.com/golang/net/blob/master/webdav/prop.go) — HIGH confidence; standard Go WebDAV live properties
- [WebDAV Class 2 server creation guide](https://www.webdavsystem.com/server/documentation/creating_class_2_webdav_server) — MEDIUM confidence; describes Class 1 vs 2
- [PROPFIND davwiki](https://github.com/dmfs/davwiki/wiki/PROPFIND) — MEDIUM confidence; PROPFIND behavior documentation

---

*Feature research for: WebDAV server — cozy-stack*
*Researched: 2026-04-04*

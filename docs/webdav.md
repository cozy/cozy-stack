[Table of contents](README.md#table-of-contents)

# WebDAV

## Introduction

cozy-stack exposes the user's file tree as a WebDAV server conforming to RFC 4918
Class 1. This means any standard WebDAV client — OnlyOffice mobile, rclone, curl,
or the iOS/iPadOS Files app — can mount a Cozy instance and read, write, rename,
and delete files using the familiar WebDAV protocol.

**Scope.** Only the `/files/` tree is accessible via WebDAV. The server implements
the nine methods listed in RFC 4918: OPTIONS, PROPFIND, GET, HEAD, PUT, DELETE,
MKCOL, COPY, and MOVE. PROPPATCH is supported with an in-memory dead-property store
(properties do not survive a server restart; CouchDB persistence is planned for v2).

**Out of scope.**

- LOCK / UNLOCK — not implemented. macOS Finder requires LOCK for write operations,
  so Finder mounts are read-only. See [Compatibility notes](#compatibility-notes--limitations).
- CalDAV / CardDAV — cozy-stack is a file server, not a calendar or address-book server.
- Quota properties (`DAV:quota-available-bytes`, `DAV:quota-used-bytes`) — deferred to v2.

**Who this is for.** Users who want to sync, backup, or edit files on their Cozy
instance using a WebDAV client; operators who need to integrate cozy-stack into an
automation pipeline; and developers building client apps.

## Endpoints

Two URL prefixes give access to the same file tree:

| Prefix | Description |
|--------|-------------|
| `/dav/files/*` | Native cozy-stack WebDAV endpoint (preferred) |
| `/remote.php/webdav/*` | Nextcloud-compatibility alias |

Both prefixes expose identical handlers. The Nextcloud alias exists because many
mobile apps — including OnlyOffice — hardcode `/remote.php/webdav/` as the WebDAV
root when configured as a Nextcloud-compatible server. The alias is served directly
by the same handlers; it is not a redirect. (An earlier 308 redirect was removed in
commit `7c9ab3a59` because several clients strip the `Authorization` header when
following redirects.)

**Base URL format:**

```
https://<instance-domain>/dav/files/
https://<instance-domain>/remote.php/webdav/
```

For example: `https://myinstance.mycozy.cloud/dav/files/`

The root of both trees is the root of the user's Cozy Drive (equivalent to
`/files/io.cozy.files.root-dir/` in the JSON:API).

### Client compatibility with `/remote.php/webdav/`

The Nextcloud-compatibility prefix exposes the **same WebDAV surface** as `/dav/files/`
— same verbs, same XML, same auth, same error codes. Litmus conformance is identical
on both routes (63/63).

However, **the two routes target different client categories**:

| Client behavior | Use `/dav/files/` | Use `/remote.php/webdav/` |
|-----------------|-------------------|---------------------------|
| User types the WebDAV URL manually | ✓ | ✓ |
| Client auto-detects the URL pattern and switches to "Nextcloud mode" | ✓ | ✗ — will fail |

Some clients — notably **OnlyOffice mobile**, the **Nextcloud official desktop sync**,
and the **Nextcloud official mobile apps** — treat `/remote.php/` as a signal that the
server is a full Nextcloud deployment. They then probe for the Nextcloud OCS API:

```
GET /remote.php              → expected: 200 or 302 (login page)
GET /ocs/v1.php/cloud/capabilities   → expected: XML capability document
```

cozy-stack implements **none** of these endpoints (OCS is out of scope for v1 and v1.1).
A client in "Nextcloud mode" will receive 401 or 404 on these probes and conclude that
the URL is invalid — often **before** sending any credentials, so the error surfaces as
"URL / login / password error" immediately after URL entry.

**Recommendation:**

- For clients that let you configure a raw WebDAV URL (rclone, cadaver, iOS Files,
  GNOME/KDE file managers, curl) — both routes work identically; prefer `/dav/files/`.
- For clients that have a "Nextcloud" or "ownCloud" preset mode (OO mobile configured as
  Nextcloud, Nextcloud Files app, Nextcloud desktop sync) — **neither route will work**,
  because the client expects full Nextcloud compatibility (OCS API + DocumentServer).
  Use these clients in their generic WebDAV mode if they have one, pointing at
  `/dav/files/`.

This limitation is documented empirically in
`.planning/phases/03-copy-compliance-and-documentation/03-MANUAL-VALIDATION-OO-MOBILE.md`.

## Authentication

Two authentication mechanisms are accepted on all routes except OPTIONS:

**1. OAuth Bearer token**

Pass the token in the `Authorization` header:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  https://myinstance.mycozy.cloud/dav/files/
```

**2. HTTP Basic Auth with the OAuth token as password**

The username field is ignored. Pass the token in the password field:

```bash
curl -u ":$TOKEN" https://myinstance.mycozy.cloud/dav/files/
```

This is the mode most WebDAV clients use (including rclone, litmus, and many mobile
apps). The username can be any non-empty string, or an empty string as shown above.

**Obtaining a token.** Tokens are OAuth 2 access tokens scoped to `io.cozy.files`.
See [docs/auth.md](./auth.md) for the full OAuth flow. For testing with the CLI:

```bash
cozy-stack instances token-app <domain> drive
```

**OPTIONS does not require authentication.** Per RFC 4918 §9.1, an OPTIONS request
to any URL returns the `DAV:` capability header without credentials:

```bash
curl -I -X OPTIONS https://myinstance.mycozy.cloud/dav/files/
# DAV: 1
# Allow: OPTIONS, PROPFIND, PROPPATCH, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE
```

## Supported methods

The table below summarises all supported methods. Detailed descriptions and curl
examples follow.

| Method | Purpose | Key behavior |
|--------|---------|--------------|
| OPTIONS | Capability discovery | Returns `DAV: 1` and `Allow:` header; no auth required |
| PROPFIND | List collection / stat resource | Depth 0 and 1 supported; `Depth: infinity` rejected with 403 |
| PROPPATCH | Set/remove dead properties | In-memory store; 207 Multi-Status response; does not survive restart |
| GET | Download file | Streams via `vfs.ServeFileContent`; supports Range and If-Modified-Since |
| HEAD | Stat file | Same as GET without body |
| PUT | Upload file | Streaming; supports If-Match / If-None-Match; parent must exist (409 otherwise) |
| DELETE | Trash file or directory | Soft-delete to `.cozy_trash`; permanent delete is not available via WebDAV |
| MKCOL | Create directory | Single level only; 405 if exists; 409 if parent missing; 415 if body present |
| COPY | Copy file or directory | File: `vfs.CopyFile`; dir: recursive walk; 207 on partial failure |
| MOVE | Rename or reparent | Overwrite:T default; 412 on Overwrite:F with existing destination |

---

### OPTIONS

OPTIONS returns the server's capability declaration without requiring authentication.
The response includes `DAV: 1` (RFC 4918 Class 1 compliance) and the full list of
allowed methods.

```bash
curl -i -X OPTIONS https://myinstance.mycozy.cloud/dav/files/
```

Expected response headers:

```
HTTP/1.1 200 OK
DAV: 1
Allow: OPTIONS, PROPFIND, PROPPATCH, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE
```

---

### PROPFIND

PROPFIND lists the children of a collection or returns the properties of a single
resource. `Depth: 0` returns only the target resource; `Depth: 1` returns the target
and its direct children. `Depth: infinity` is rejected with `403 Forbidden` to prevent
resource exhaustion on large trees.

```bash
# List the root collection (one level deep)
curl -u ":$TOKEN" -X PROPFIND \
  -H "Depth: 1" \
  https://myinstance.mycozy.cloud/dav/files/

# Stat a single file
curl -u ":$TOKEN" -X PROPFIND \
  -H "Depth: 0" \
  https://myinstance.mycozy.cloud/dav/files/Documents/report.pdf
```

The response is a `207 Multi-Status` XML body. Each `<D:response>` element contains
the resource href and a `<D:propstat>` block with the live properties:
`getcontentlength`, `getcontenttype`, `getlastmodified`, `getetag`, `resourcetype`.

ETag values are derived from the file's MD5 checksum (not from the CouchDB `_rev`
field, which changes on metadata edits unrelated to content).

---

### PROPPATCH

PROPPATCH sets or removes dead (custom) properties on a resource. The server stores
them in memory per instance domain. Properties are lost on server restart; full
CouchDB persistence is planned for v2 (requirement `ADV-V2-02`).

```bash
curl -u ":$TOKEN" -X PROPPATCH \
  -H "Content-Type: application/xml" \
  -d '<?xml version="1.0"?>
  <D:propertyupdate xmlns:D="DAV:" xmlns:Z="http://example.com/">
    <D:set><D:prop><Z:author>Alice</Z:author></D:prop></D:set>
  </D:propertyupdate>' \
  https://myinstance.mycozy.cloud/dav/files/Documents/report.pdf
```

The response is `207 Multi-Status` with `200 OK` for each processed property.

---

### GET

GET downloads a file. The server streams the file content directly from the VFS
without buffering the entire response in memory. Standard HTTP conditional requests
are supported: `Range`, `If-Modified-Since`, `If-None-Match`.

```bash
# Download a file
curl -u ":$TOKEN" -o report.pdf \
  https://myinstance.mycozy.cloud/dav/files/Documents/report.pdf

# Partial download (bytes 0-1023)
curl -u ":$TOKEN" -H "Range: bytes=0-1023" \
  https://myinstance.mycozy.cloud/dav/files/Documents/report.pdf
```

---

### HEAD

HEAD returns the same headers as GET (Content-Length, Content-Type, ETag,
Last-Modified) without transmitting the response body. Useful for checking whether
a file has changed since the last sync.

```bash
curl -u ":$TOKEN" -I \
  https://myinstance.mycozy.cloud/dav/files/Documents/report.pdf
```

---

### PUT

PUT uploads a file. The body is streamed directly into the VFS; the server does not
buffer the upload in memory. The parent directory must already exist — if it does
not, the server returns `409 Conflict`.

Conditional upload is supported via `If-Match` (update only if ETag matches) and
`If-None-Match: *` (create only if the resource does not exist):

```bash
# Create or replace a file
curl -u ":$TOKEN" -T local-report.pdf \
  https://myinstance.mycozy.cloud/dav/files/Documents/report.pdf

# Conditional replace — only if ETag matches
curl -u ":$TOKEN" -T local-report.pdf \
  -H 'If-Match: "d41d8cd98f00b204e9800998ecf8427e"' \
  https://myinstance.mycozy.cloud/dav/files/Documents/report.pdf

# Create only (fail if already exists)
curl -u ":$TOKEN" -T local-report.pdf \
  -H "If-None-Match: *" \
  https://myinstance.mycozy.cloud/dav/files/Documents/report.pdf
```

A successful upload of a new resource returns `201 Created`; an overwrite returns
`204 No Content`.

---

### DELETE

DELETE moves the target file or directory to `.cozy_trash` (soft-delete). The file
is not permanently destroyed — it can be recovered from the Cozy trash via the
JSON:API. To permanently delete a file, use the Files API directly.

```bash
curl -u ":$TOKEN" -X DELETE \
  https://myinstance.mycozy.cloud/dav/files/Documents/old-report.pdf
```

The `.cozy_trash` directory itself and files inside it are exposed read-only via
WebDAV. A DELETE request targeting a path under `.cozy_trash/` returns `405 Method
Not Allowed`.

---

### MKCOL

MKCOL creates a single new directory. The parent directory must already exist —
if not, the server returns `409 Conflict`. If the collection already exists, the
server returns `405 Method Not Allowed`. MKCOL with a request body returns
`415 Unsupported Media Type`.

```bash
curl -u ":$TOKEN" -X MKCOL \
  https://myinstance.mycozy.cloud/dav/files/Documents/NewFolder
```

---

### COPY

COPY duplicates a file or directory to a new location specified by the `Destination`
header. Both the native `/dav/files/` and the Nextcloud `/remote.php/webdav/`
prefixes are accepted in the `Destination` header.

The `Overwrite` header controls collision behaviour:

- `Overwrite: T` (default when the header is absent) — if the destination exists,
  it is first moved to `.cozy_trash`, then the copy proceeds.
- `Overwrite: F` — if the destination exists, the request fails with
  `412 Precondition Failed`.

For files, `vfs.CopyFile` is used directly. Cozy Notes (`.cozy-note` mime type) use
`note.CopyFile` to preserve attached images.

For directories, the `Depth` header controls how much is copied:

- `Depth: infinity` (default) — full recursive copy of the subtree.
- `Depth: 0` — copies the directory itself as an empty collection.
- `Depth: 1` is not valid for COPY (RFC 4918 §9.8) and returns `400 Bad Request`.

If any individual file or sub-directory fails during a recursive copy, the server
returns `207 Multi-Status` with a `<D:response>` entry for each failure. Successfully
copied items remain in place; there is no rollback.

```bash
# Copy a file
curl -u ":$TOKEN" -X COPY \
  -H "Destination: https://myinstance.mycozy.cloud/dav/files/Archive/report-backup.pdf" \
  https://myinstance.mycozy.cloud/dav/files/Documents/report.pdf

# Copy a directory (full recursive)
curl -u ":$TOKEN" -X COPY \
  -H "Depth: infinity" \
  -H "Destination: https://myinstance.mycozy.cloud/dav/files/Archive/Documents-backup" \
  https://myinstance.mycozy.cloud/dav/files/Documents

# Copy a directory shell only (no contents)
curl -u ":$TOKEN" -X COPY \
  -H "Depth: 0" \
  -H "Destination: https://myinstance.mycozy.cloud/dav/files/Archive/EmptyDocs" \
  https://myinstance.mycozy.cloud/dav/files/Documents
```

---

### MOVE

MOVE renames or reparents a file or directory. The `Destination` header specifies
the new path. Overwrite semantics are the same as COPY: `Overwrite: T` (default)
trashes any existing destination before moving; `Overwrite: F` returns
`412 Precondition Failed` if the destination exists. Cross-host moves (where
`Destination` points to a different origin) return `502 Bad Gateway`.

```bash
# Rename a file
curl -u ":$TOKEN" -X MOVE \
  -H "Destination: https://myinstance.mycozy.cloud/dav/files/Documents/final-report.pdf" \
  https://myinstance.mycozy.cloud/dav/files/Documents/report.pdf

# Move a file to another directory
curl -u ":$TOKEN" -X MOVE \
  -H "Destination: https://myinstance.mycozy.cloud/dav/files/Archive/report.pdf" \
  https://myinstance.mycozy.cloud/dav/files/Documents/report.pdf
```

## Client configuration

### OnlyOffice mobile

OnlyOffice Documents (mobile) connects to a Cozy instance using its **generic WebDAV
mode**, not its Nextcloud mode (see
[Client compatibility with `/remote.php/webdav/`](#client-compatibility-with-remotephpwebdav)
for why).

1. Open the app and tap **Add account**.
2. Select **WebDAV** as the server type (not Nextcloud).
3. Enter the server URL pointing at the native route:
   ```
   https://<instance-domain>/dav/files/
   ```
4. Enter any string as the username (it is ignored by the server).
5. Enter your OAuth access token as the password.

**Known issue in OnlyOffice Documents v9.2.0 through v9.3.1.** A client-side regression
introduced in v9.2.0 breaks WebDAV authentication against servers that don't expose the
Nextcloud OCS API. The upstream error message is *"App token login name does not match"*.
Affects both iOS and Android. Tracked on the ONLYOFFICE community forum; no fix released
as of April 2026.

**Workarounds:**

- **Android** — side-load APK **v9.1.0** from the official GitHub releases
  (`github.com/ONLYOFFICE/documents-app-android/releases/tag/v9.1.0-663`), disable
  auto-update for the app. v9.1.0 predates the regression and is confirmed working
  end-to-end against cozy-stack WebDAV (see validation trace in
  `.planning/phases/03-copy-compliance-and-documentation/03-MANUAL-VALIDATION-OO-MOBILE.md`).
- **iOS** — no simple workaround; the App Store does not permit downgrades. Wait for
  a v9.3.2+ release or use another client.

**Do not configure OnlyOffice mobile as "Nextcloud".** Even though cozy-stack exposes
the `/remote.php/webdav/` compatibility route, selecting Nextcloud mode in OO triggers
a Nextcloud-specific probe (`GET /remote.php`, `GET /ocs/v1.php/...`) that cozy-stack
does not implement. The app will reject the URL immediately with a generic error.

See [docs/office.md](./office.md) for documentation on the server-side OnlyOffice
integration (document editing via the OnlyOffice Document Server, which is a separate
feature from WebDAV).

### rclone

[rclone](https://rclone.org/) connects via the native `/dav/files/` endpoint.

1. Run `rclone config` and create a new remote of type `webdav`.
2. Or add the following to your `~/.config/rclone/rclone.conf`:

```ini
[cozy]
type = webdav
url = https://<instance-domain>/dav/files/
vendor = other
user = any
pass = <obscured-token>
```

Obtain the obscured token:

```bash
rclone obscure "$TOKEN"
```

Replace `<obscured-token>` with the output of that command.

Basic usage:

```bash
# List files
rclone ls cozy:

# Sync a local directory to Cozy
rclone sync /local/path cozy:Backups/laptop

# Copy a single file
rclone copy report.pdf cozy:Documents/
```

### curl / manual

The native endpoint works with any tool that speaks HTTP. Common operations:

```bash
# List the root (PROPFIND Depth:1)
curl -u ":$TOKEN" -X PROPFIND -H "Depth: 1" \
  https://myinstance.mycozy.cloud/dav/files/

# Upload a file
curl -u ":$TOKEN" -T myfile.txt \
  https://myinstance.mycozy.cloud/dav/files/Documents/myfile.txt

# Download a file
curl -u ":$TOKEN" -o myfile.txt \
  https://myinstance.mycozy.cloud/dav/files/Documents/myfile.txt
```

### iOS / iPadOS Files app (best-effort)

Compatibility with the iOS/iPadOS Files app is best-effort in v1 — formal validation
is deferred to v1.1. The server is RFC 4918 Class 1 compliant and verified with the
litmus suite, which should be sufficient for most WebDAV clients.

To connect:

1. In the Files app, tap the `...` button in the Browse sidebar and select
   **Connect to Server**.
2. Enter the server URL:
   ```
   https://<instance-domain>/dav/files/
   ```
3. Select **Registered User**.
4. Enter any string as the name and your OAuth token as the password.

Because cozy-stack does not implement LOCK, write operations from the Files app may
be blocked depending on the iOS version. Read access (browsing and downloading) works
via PROPFIND and GET.

## Compatibility notes & limitations

- **No LOCK / UNLOCK (Class 1 only — RFC 4918 §6).** The server advertises
  `DAV: 1`. macOS Finder requires LOCK tokens to perform write operations; as a
  consequence, Finder mounts are **read-only**. Read access (browsing and previewing
  files) works normally.

- **PROPPATCH with in-memory storage.** Dead properties set via PROPPATCH are stored
  in memory and are lost when the server restarts. Full CouchDB persistence is a v2
  requirement (`ADV-V2-02`).

- **`PROPFIND Depth: infinity` rejected with 403 Forbidden.** This is intentional
  DoS protection. Use `Depth: 1` to list a single directory level.

- **`PROPFIND Depth: 1` has no pagination cap.** Very large directories (thousands
  of files) are returned in a single response. This may take time on large
  collections.

- **`.cozy_trash` is exposed read-only.** The trash directory appears in PROPFIND
  responses so clients can see trashed files. Write operations (PUT, MKCOL, COPY
  into trash, MOVE into trash) are rejected. DELETE on a path inside `.cozy_trash`
  returns `405 Method Not Allowed`.

- **DELETE is a soft-delete.** DELETE moves files and directories to `.cozy_trash`
  rather than destroying them. To permanently remove a file, use the Cozy Files
  JSON:API (`DELETE /files/:id?Force=true`).

- **Streaming I/O.** PUT and GET both stream — neither upload nor download is fully
  buffered in memory. This means the server can handle large files without an
  increased memory footprint.

- **ETag source: content MD5.** ETags are derived from the file's MD5 checksum,
  not from the CouchDB document revision (`_rev`). This means ETags change only when
  file content changes, not when metadata is updated.

- **Date format: RFC 1123 (HTTP-date).** `getlastmodified` property values use
  `Thu, 01 Jan 2026 12:00:00 GMT` format (RFC 1123 / `http.TimeFormat` in Go).
  macOS Finder silently misparsed ISO 8601 dates, so RFC 3339 is intentionally not used.

- **`Depth: 1` on COPY / MOVE is rejected with 400.** RFC 4918 §9.8 explicitly
  forbids `Depth: 1` for COPY on collections. Use `Depth: 0` (empty collection
  copy) or `Depth: infinity` (full recursive copy).

## Troubleshooting

### Common errors

| Status | Situation | Cause |
|--------|-----------|-------|
| 401 Unauthorized | Any request (except OPTIONS) | Token missing, expired, or wrong scope. The `WWW-Authenticate: Basic realm="Cozy"` header is returned. |
| 403 Forbidden | PROPFIND with `Depth: infinity` | By design. Use `Depth: 1`. |
| 403 Forbidden | Write to `.cozy_trash/*` | Trash is read-only via WebDAV. |
| 403 Forbidden | Path traversal attempt | Path contains `..` or escapes the `/files/` root. |
| 404 Not Found | GET / PROPFIND / DELETE | The resource does not exist. Check the path. |
| 405 Method Not Allowed | DELETE on `.cozy_trash` | Trash cannot be deleted via WebDAV. The `Allow:` header lists permitted methods. |
| 409 Conflict | PUT or MKCOL | Parent directory does not exist. Create it first with MKCOL. |
| 412 Precondition Failed | COPY or MOVE with `Overwrite: F` | Destination already exists and Overwrite is F. |
| 412 Precondition Failed | PUT with `If-Match` | ETag mismatch — file was modified since you last read it. |
| 415 Unsupported Media Type | MKCOL with a body | MKCOL must not include a request body (RFC 4918 §9.3). |
| 502 Bad Gateway | MOVE or COPY | `Destination` header points to a different host. |

### Authentication debugging

**Check that your token is valid and scoped to files:**

```bash
# Should return 207 Multi-Status with the root collection
curl -u ":$TOKEN" -X PROPFIND -H "Depth: 0" \
  https://myinstance.mycozy.cloud/dav/files/

# Generate a new token with the CLI
cozy-stack instances token-app <domain> drive
```

**Token in Basic Auth:** The username field is ignored but must be present in some
clients. Use `:token` (colon then token) or `any:token`.

### Logs

cozy-stack emits structured log lines for each WebDAV request. To see them:

```bash
# If running locally
cozy-stack serve --log-level debug 2>&1 | grep webdav

# In production, filter by the component field
journalctl -u cozy-stack | grep '"component":"webdav"'
```

Each write operation (PUT, DELETE, MKCOL, COPY, MOVE, PROPPATCH) also emits an
audit log entry at INFO level containing the method, path, and authenticated
instance domain. Refer to the cozy-stack logging documentation for log format details
and log rotation configuration.

## Compliance testing

The WebDAV implementation is validated against the
[litmus](http://www.webdav.org/neon/litmus/) test suite, the reference compliance
checker for RFC 4918. The repository ships an orchestration script that automates the
full lifecycle: create a disposable instance, generate a CLI token, run litmus against
both routes, and destroy the instance on exit.

### Prerequisites

- cozy-stack running locally (`cozy-stack serve` in another terminal)
- litmus installed (`apt install litmus` on Debian/Ubuntu)
- `cozy-stack` binary on PATH

### Running the tests

```bash
# Run all suites (basic, copymove, props, http, locks) against both routes
make test-litmus

# Or call the script directly
scripts/webdav-litmus.sh

# Run a single suite
LITMUS_TESTS="copymove" scripts/webdav-litmus.sh

# Dry-run — exercise the instance lifecycle without calling litmus
scripts/webdav-litmus.sh --dry-run
```

The script tests both `/dav/files/` and `/remote.php/webdav/` in a single run. It
exits 0 only when both routes have zero failed tests.

### Expected results

| Suite | Tests | Notes |
|-------|-------|-------|
| basic | 16/16 | PUT, GET, DELETE, MKCOL, UTF-8 paths |
| copymove | 13/13 | COPY file, COPY collection, MOVE file, MOVE collection |
| props | 30/30 | PROPFIND, PROPPATCH (in-memory dead properties) |
| http | 4/4 | Expect: 100-continue handling |
| locks | 3/3 (1 skipped) | Locking tests auto-skip — server advertises `DAV: 1` (Class 1 only) |

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Both routes passed (zero failed tests) |
| 1 | One or both routes had failed tests |
| 2 | Setup failure (missing binary, stack not running, litmus not installed) |

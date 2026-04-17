[Table of contents](README.md#table-of-contents)

# Proxy for a remote NextCloud

The nextcloud konnector can be used to create an `io.cozy.account` for a
NextCloud. Then, the stack can be used as a client for this NextCloud account.
Currently, it supports files operations via WebDAV.

## GET /remote/nextcloud/:account/*path

This route can be used to list the files and subdirectories inside a directory
of NextCloud.

With `Dl=1` in the query-string, it can also be used to download a file.

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.
It is available with the `cozyMetadata.sourceAccount` of the shortcut file for
example.

The `*path` parameter is the path of the file/directory on the NextCloud.

**Note:** a permission on `GET io.cozy.files` is required to use this route.

### Request (list)

```http
GET /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/Documents HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response (list)

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": [
    {
      "type": "io.cozy.remote.nextcloud.files",
      "id": "192172",
      "attributes": {
        "type": "directory",
        "name": "Images",
        "path": "/Documents/Images",
        "updated_at": "Thu, 02 May 2024 09:29:53 GMT",
        "etag": "\"66335d11c4b91\""
      },
      "meta": {},
      "links": {
        "self": "https://nextcloud.example.net/apps/files/files/192172?dir=/Documents"
      }
    },
    {
      "type": "io.cozy.remote.nextcloud.files",
      "id": "208937",
      "attributes": {
        "type": "file",
        "name": "BugBounty.pdf",
        "path": "/Documents/BugBounty.pdf",
        "size": 2947,
        "mime": "application/pdf",
        "class": "pdf",
        "updated_at": "Mon, 14 Jan 2019 08:22:21 GMT",
        "etag": "\"dd1a602431671325b7c1538f829248d9\""
      },
      "meta": {},
      "links": {
        "self": "https://nextcloud.example.net/apps/files/files/208937?dir=/Documents"
      }
    },
    {
      "type": "io.cozy.remote.nextcloud.files",
      "id": "615827",
      "attributes": {
        "type": "directory",
        "name": "Music",
        "name": "/Documents/Music",
        "updated_at": "Thu, 02 May 2024 09:28:37 GMT",
        "etag": "\"66335cc55204b\""
      },
      "meta": {},
      "links": {
        "self": "https://nextcloud.example.net/apps/files/files/615827?dir=/Documents"
      }
    },
    {
      "type": "io.cozy.remote.nextcloud.files",
      "id": "615828",
      "attributes": {
        "type": "directory",
        "name": "Video",
        "path": "/Documents/Video",
        "updated_at": "Thu, 02 May 2024 09:29:53 GMT",
        "etag": "\"66335d11c2318\""
      },
      "meta": {},
      "links": {
        "self": "https://nextcloud.example.net/apps/files/files/615828?dir=/Documents"
      }
    }
  ],
  "meta": {
    "count": 5
  }
}
```

### Request (download)

```http
GET /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/Documents/Wallpaper.jpg?Dl=1 HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response (download)

```http
HTTP/1.1 200 OK
Content-Type: image/jpeg
Content-Length: 12345
Content-Disposition: attachment; filename="Wallpaper.jpg"

...
```

#### Status codes

- 200 OK, for a success
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the directory is not found on the NextCloud

## PUT /remote/nextcloud/:account/*path

This route can be used to create a directory, or upload a file, on the
NextCloud. The query-string parameter `Type` should be `file` when uploading a
file.

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.

The `*path` parameter is the path of the file/directory on the NextCloud.

**Note:** a permission on `POST io.cozy.files` is required to use this route.

### Request (directory)

```http
PUT /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/Documents/Images/Clouds?Type=directory HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response (directory)

```http
HTTP/1.1 201 Created
Content-Type: application/json
```

```json
{
  "ok": true
}
```

### Request (file)

```http
PUT /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/Documents/Images/sunset.jpg?Type=file HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
Content-Type: image/jpeg
Content-Length: 54321

...
```

### Response (file)

```http
HTTP/1.1 201 Created
Content-Type: application/json
```

```json
{
  "ok": true
}
```

#### Status codes

- 201 Created, when the directory has been created
- 400 Bad Request, when the account is not configured for NextCloud
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the parent directory is not found on the NextCloud
- 409 Conflict, when a directory or file already exists at this path on the NextCloud.

## DELETE /remote/nextcloud/:account/*path

This route can be used to put a file or directory in the NextCloud trash.

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.

The `*path` parameter is the path of the file/directory on the NextCloud.

**Note:** a permission on `DELETE io.cozy.files` is required to use this route.

### Request

```http
DELETE /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/Documents/Images/Clouds HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response

```http
HTTP/1.1 204 No Content
```

#### Status codes

- 204 No Content, when the file/directory has been put in the trash
- 400 Bad Request, when the account is not configured for NextCloud, or the `To` parameter is missing
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the file/directory is not found on the NextCloud

## POST /remote/nextcloud/:account/move/*path

This route can be used to move or rename a file/directory on the NextCloud.
The new path must be given with the `To` parameter in the query-string.

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.

The `*path` parameter is the path of the file on the NextCloud.

**Note:** a permission on `POST io.cozy.files` is required to use this route.

### Request

```http
POST /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/move/Documents/wallpaper.jpg?To=/Wallpaper.jpg HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response

```http
HTTP/1.1 204 No Content
```

#### Status codes

- 204 No Content, when the file/directory has been moved
- 400 Bad Request, when the account is not configured for NextCloud
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the file/directory is not found on the NextCloud
- 409 Conflict, when a file already exists with the new name on the NextCloud.

## POST /remote/nextcloud/:account/copy/*path

This route can be used to create a copy of a file in the same directory, with a
copy suffix in its name. The new name can be optionaly given with the `Name`
parameter in the query-string, or the full path can be given with `Path`
parameter.

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.

The `*path` parameter is the path of the file on the NextCloud.

**Note:** a permission on `POST io.cozy.files` is required to use this route.

### Request

```http
POST /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/copy/Documents/wallpaper.jpg?Path=/Images/beach.jpg HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response

```http
HTTP/1.1 201 Created
Content-Type: application/json
```

```json
{
  "ok": true
}
```

#### Status codes

- 201 Created, when the file has been copied
- 400 Bad Request, when the account is not configured for NextCloud
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the file/directory is not found on the NextCloud
- 409 Conflict, when a file already exists with the new name on the NextCloud.

## POST /remote/nextcloud/:account/downstream/*path

This route can be used to move/copy a file from the NextCloud to the Cozy.

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.

The `*path` parameter is the path of the file on the NextCloud.

The `To` parameter in the query-string must be given, as the ID of the
directory on the Cozy where the file will be put.

By default, the file will be moved, but using `Copy=true` in the query-string
will makes a copy.

By default, if a file with the same name already exists in the destination
directory, a new name will be automatically generated (e.g., "file (2).txt").
Using `FailOnConflict=true` in the query-string will make the route return a
409 Conflict error instead of auto-renaming the file.

**Note:** a permission on `POST io.cozy.files` is required to use this route.

### Request

```http
POST /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/downstream/Documents/Images/sunset.jpg?To=b3ecbc00f4ba013c2bf418c04daba326 HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

#### Query parameters

- `To` (required): The ID of the directory on the Cozy where the file will be put.
- `Copy` (optional): Set to `true` to make a copy instead of moving the file. Default is `false` (move).
- `FailOnConflict` (optional): Set to `true` to return a 409 Conflict error if a file with the same name already exists, instead of auto-renaming. Default is `false` (auto-rename).

#### Example with FailOnConflict

```http
POST /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/downstream/Documents/Images/sunset.jpg?To=b3ecbc00f4ba013c2bf418c04daba326&FailOnConflict=true HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.files",
    "id": "7b41fb7c31e87eeaf13a54bc32001830",
    "attributes": {
      "type": "file",
      "name": "sunset.jpg",
      "dir_id": "b3ecbc00f4ba013c2bf418c04daba326",
      "created_at": "2024-05-15T09:24:39.460655706+02:00",
      "updated_at": "2024-05-15T09:24:39.460655706+02:00",
      "size": "54321",
      "md5sum": "1B2M2Y8AsgTpgAmY7PhCfg==",
      "mime": "image/jpeg",
      "class": "image",
      "executable": false,
      "trashed": false,
      "encrypted": false,
      "cozyMetadata": {
        "doctypeVersion": "1",
        "metadataVersion": 1,
        "createdAt": "2024-05-15T09:24:38.971901347+02:00",
        "updatedAt": "2024-05-15T09:24:38.971901347+02:00",
        "createdOn": "https://cozy.example.net/",
        "uploadedAt": "2024-05-15T09:24:38.971901347+02:00",
        "uploadedOn": "https://cozy.example.net/"
      }
    },
    "meta": {
      "rev": "1-cfed435c4ad72b911b31ed775e3024df"
    },
    "links": {
      "self": "/files/7b41fb7c31e87eeaf13a54bc32001830"
    },
    "relationships": {
      "parent": {
        "links": {
          "related": "/files/b3ecbc00f4ba013c2bf418c04daba326"
        },
        "data": {
          "id": "b3ecbc00f4ba013c2bf418c04daba326",
          "type": "io.cozy.files"
        }
      },
      "referenced_by": {
        "links": {
          "self": "/files/7b41fb7c31e87eeaf13a54bc32001830/relationships/references"
        }
      }
    }
  }
}
```

#### Status codes

- 201 Created, when the file has been moved from the NextCloud to the Cozy
- 400 Bad Request, when the account is not configured for NextCloud or the `To` parameter is missing
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the file is not found on the NextCloud
- 409 Conflict, when a file with the same name already exists in the destination directory and `FailOnConflict=true` is set

## POST /remote/nextcloud/:account/upstream/*path

This route can be used to move/copy a file from the Cozy to the NextCloud.

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.

The `*path` parameter is the path of the file on the NextCloud.

The `From` parameter in the query-string must be given, as the ID of the
file on the Cozy that will be moved.

By default, the file will be moved, but using `Copy=true` in the query-string
will makes a copy.

**Note:** a permission on `POST io.cozy.files` is required to use this route.

### Request

```http
POST /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/upstream/Documents/Images/sunset2.jpg?From=7b41fb7c31e87eeaf13a54bc32001830 HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response

```http
HTTP/1.1 204 No Content
```

#### Status codes

- 204 No Content, when the file has been moved from the Cozy to the NextCloud
- 400 Bad Request, when the account is not configured for NextCloud
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the file is not found on the Cozy

## GET /remote/nextcloud/:account/trash/*

This route can be used to list the files and directories inside the trashbin
of NextCloud.

### Request (list)

```http
GET /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/trash/ HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response (list)

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": [
    {
      "type": "io.cozy.remote.nextcloud.files",
      "id": "613281",
      "attributes": {
        "type": "directory",
        "name": "Old",
        "path": "/trash/Old.d93571568",
        "updated_at": "Tue, 25 Jun 2024 14:31:44 GMT",
        "etag": "1719326384",
        "restore_path": "/Old"
      },
      "meta": {},
      "links": {
        "self": "https://nextcloud.example.net/apps/files/trashbin/613281?dir=/Old"
      }
    }
  ]
}
```

#### Status codes

- 200 OK, for a success
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the directory is not found on the NextCloud

## POST /remote/nextcloud/:account/restore/*path

This route can be used to restore a file/directory from the trashbin on the
NextCloud.

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.

The `*path` parameter is the path of the file on the NextCloud.

**Note:** a permission on `POST io.cozy.files` is required to use this route.

### Request

```http
POST /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/restore/trash/Old.d93571568 HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response

```http
HTTP/1.1 204 No Content
```

#### Status codes

- 204 No Content, when the file/directory has been restored
- 400 Bad Request, when the account is not configured for NextCloud
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the file/directory is not found on the NextCloud
- 409 Conflict, when a directory or file already exists where the file/directory should be restored on the NextCloud.

## DELETE /remote/nextcloud/:account/trash/*

This route can be used to delete a file in the trash.

### Request

```http
DELETE /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/trash/document-v1.docx.d64283654 HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response

```http
HTTP/1.1 204 No Content
```

#### Status codes

- 204 No Content, when the file/directory has been put in the trash
- 400 Bad Request, when the account is not configured for NextCloud, or the `To` parameter is missing
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the file/directory is not found on the NextCloud

## DELETE /remote/nextcloud/:account/trash

This route can be used to empty the trash bin on NextCloud.

### Request

```http
DELETE /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/trash HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response

```http
HTTP/1.1 204 No Content
```

## GET /remote/nextcloud/:account/size/*path

This route returns the recursive size in bytes of a folder (or a single
file) on the NextCloud account. It is backed by a single Depth:0 PROPFIND
asking for the `oc:size` property that NextCloud maintains in its metadata
table, so it runs in constant time regardless of how many files are in the
tree. Pass the account root by using an empty `*path` (just `/size/`).

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.

**Note:** a permission on `GET io.cozy.files` is required to use this route.

### Request (sub-folder)

```http
GET /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/size/Photos HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "size": 5656463
}
```

### Request (account root)

```http
GET /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/size/ HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

#### Status codes

- 200 OK, with the recursive byte total of the target path
- 401 Unauthorized, when the NextCloud credentials are rejected
- 404 Not Found, when the account or the target path does not exist

## POST /remote/nextcloud/migration

This route triggers a one-shot bulk migration of a user's Nextcloud files into
their Cozy. The Stack validates the credentials, persists an
`io.cozy.accounts` document, creates an `io.cozy.nextcloud.migrations`
tracking document in `pending` state, and publishes a
`nextcloud.migration.requested` command to the `migration` RabbitMQ exchange.
The actual transfer is performed by an external migration service that
consumes the command and drives the existing `/remote/nextcloud/:account/*`
routes, updating the tracking document as it progresses.

Before persisting anything, the Stack probes the supplied credentials against
the Nextcloud instance via the OCS `user_status` endpoint, so wrong passwords
and unreachable hosts surface synchronously instead of being deferred to the
migration service. The probe also resolves the WebDAV user ID, which is
cached on the account document so the migration service does not need to
re-fetch it.

When an existing `io.cozy.accounts` document for the same `account_type:
"nextcloud"` + `auth.url` + `auth.login` triplet is found, it is reused with
its stored password and `webdav_user_id` refreshed from the request. Only one
migration can be in flight per instance at a time: if a `pending` or `running`
tracking document already exists, the Stack returns `409 Conflict`. Failed
migrations do not block new attempts.

**Note:** a permission on `POST io.cozy.nextcloud.migrations` is required to
use this route.

### Request

```http
POST /remote/nextcloud/migration HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
Content-Type: application/json
```

```json
{
  "nextcloud_url": "https://nextcloud.example.com",
  "nextcloud_login": "alice",
  "nextcloud_app_password": "xxxxx-xxxxx-xxxxx-xxxxx-xxxxx",
  "source_path": "/"
}
```

`source_path` is optional and defaults to `/`. The `nextcloud_app_password`
should be a Nextcloud app password, not the user's main account password.

### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "id": "d4e5f6a7b8c94d0ea1b2c3d4e5f6a7b8",
    "type": "io.cozy.nextcloud.migrations",
    "attributes": {
      "status": "pending",
      "target_dir": "/Nextcloud",
      "progress": {
        "files_imported": 0,
        "files_total": 0,
        "bytes_imported": 0,
        "bytes_total": 0
      },
      "errors": [],
      "skipped": [],
      "started_at": null,
      "finished_at": null
    }
  }
}
```

#### Status codes

- 201 Created, when the migration has been queued and the tracking document is returned
- 401 Unauthorized, when the Nextcloud credentials are rejected by the remote host
- 409 Conflict, when a `pending` or `running` migration already exists
- 500 Internal Server Error, when the conflict check, account upsert, or tracking document creation fails
- 502 Bad Gateway, when the Nextcloud instance is unreachable
- 503 Service Unavailable, when the migration command cannot be published to RabbitMQ. The tracking document is marked `failed` before returning

## POST /remote/nextcloud/migration/:id/cancel

This route asks the migration service to stop an in-flight migration. The
Stack publishes a `nextcloud.migration.canceled` command to the `migration`
RabbitMQ exchange; the migration service then stops the migration between
files and transitions the tracking document to a new terminal state
`canceled` with progress up to the last completed file preserved.

The Stack does not write to the tracking document on cancel: that terminal
state transition is owned by the migration service so there is a single
writer for it. A `202 Accepted` means "cancel requested, poll the tracking
document for the terminal state", not "migration stopped". Worst-case
delay from request to observed terminal state is roughly one file transfer.

**Note:** a permission on `POST io.cozy.nextcloud.migrations` is required
to use this route.

### Request

```http
POST /remote/nextcloud/migration/d4e5f6a7b8c94d0ea1b2c3d4e5f6a7b8/cancel HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

No request body.

### Response

```http
HTTP/1.1 202 Accepted
```

Empty body.

#### Status codes

- 202 Accepted, when the cancel command has been published
- 401 Unauthorized, when the bearer token is missing or invalid
- 403 Forbidden, when the token lacks the `POST io.cozy.nextcloud.migrations` permission
- 404 Not Found, when no tracking document with the given id exists on the instance
- 409 Conflict, when the migration has already reached a terminal state (`completed`, `failed`, or `canceled`)
- 503 Service Unavailable, when the cancel command cannot be published to RabbitMQ. Unlike the trigger endpoint, the tracking document is **not** marked `failed`: a broker glitch must not invalidate a migration that is still running. Retry the cancel.

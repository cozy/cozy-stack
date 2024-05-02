[Table of contents](README.md#table-of-contents)

# Proxy for a remote NextCloud

The nextcloud konnector can be used to create an `io.cozy.account` for a
NextCloud. Then, the stack can be used as a client for this NextCloud account.
Currently, it supports files operations via WebDAV.

## GET /remote/nextcloud/:account/*path

This route can be used to list the files and subdirectories inside a directory
of NextCloud.

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.
It is available with the `cozyMetadata.sourceAccount` of the shortcut file for
example.

The `*path` parameter is the path of the directory on the NextCloud.

**Note:** a permission on `GET io.cozy.files` is required to use this route.

### Request

```http
GET /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/Documents HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response

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
        "updated_at": "Thu, 02 May 2024 09:29:53 GMT",
        "etag": "\"66335d11c4b91\""
      },
      "meta": {}
    },
    {
      "type": "io.cozy.remote.nextcloud.files",
      "id": "208937",
      "attributes": {
        "type": "file",
        "name": "BugBounty.pdf",
        "size": 2947,
        "mime": "application/pdf",
        "class": "pdf",
        "updated_at": "Mon, 14 Jan 2019 08:22:21 GMT",
        "etag": "\"dd1a602431671325b7c1538f829248d9\""
      },
      "meta": {}
    },
    {
      "type": "io.cozy.remote.nextcloud.files",
      "id": "615827",
      "attributes": {
        "type": "directory",
        "name": "Music",
        "updated_at": "Thu, 02 May 2024 09:28:37 GMT",
        "etag": "\"66335cc55204b\""
      },
      "meta": {}
    },
    {
      "type": "io.cozy.remote.nextcloud.files",
      "id": "615828",
      "attributes": {
        "type": "directory",
        "name": "Video",
        "updated_at": "Thu, 02 May 2024 09:29:53 GMT",
        "etag": "\"66335d11c2318\""
      },
      "meta": {}
    }
  ],
  "meta": {
    "count": 5
  }
}
```

#### Status codes

- 200 OK, for a success
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the directory is not found on the NextCloud

## PUT /remote/nextcloud/:account/*path

This route can be used to create a directory on the NextCloud.

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.

The `*path` parameter is the path of the directory on the NextCloud.

**Note:** a permission on `POST io.cozy.files` is required to use this route.

### Request

```http
PUT /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/Documents/Images/Clouds HTTP/1.1
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

- 201 Created, when the directory has been created
- 400 Bad Request, when the account is not configured for NextCloud
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the parent directory is not found on the NextCloud
- 409 Conflict, when a directory or file already exists at this path on the NextCloud.

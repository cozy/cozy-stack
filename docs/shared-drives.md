[Table of contents](README.md#table-of-contents)

# Shared drives

A shared drive is a folder that is shared between several cozy instances. A
member doesn't have the files in their Cozy, but can access them via the stack
playing a proxy role.

To create a shared drive (typically on the organization Cozy), we need the
following steps:

1. Ensure that the `/Drives` folder exists in the cozy instance with the
   [`POST /files/shared-drives`](https://docs.cozy.io/en/cozy-stack/files/#post-filesshared-drives)
   route.
2. Create a folder inside it, with the name of shared drive.
3. Create a sharing with the `drive: true` attribute, and one rule for
   shared folder (with `none` for `add`, `update` and `remove` attributes).

## Managing shared drives

### GET /sharings/drives

The `GET /sharings/drives` route returns the list of shared drives.

#### Request

```http
GET /sharings/drives HTTP/1.1
Host: acme.example.net
Accept: application/vnd.api+json
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": [
    {
      "type": "io.cozy.sharings",
      "id": "aae62886e79611ef8381fb83ff72e425",
      "attributes": {
        "drive": true,
        "owner": true,
        "description": "Drive for the product team",
        "app_slug": "drive",
        "created_at": "2025-02-10T11:08:08Z",
        "updated_at": "2025-02-10T12:10:43Z",
        "members": [
          {
            "status": "owner",
            "public_name": "ACME",
            "email": "admin@acme.example.net",
            "instance": "acme.example.net"
          },
          {
            "status": "pending",
            "name": "Alice",
            "email": "alice@example.net"
          },
          {
            "status": "pending",
            "name": "Bob",
            "email": "bob@example.net"
          }
        ],
        "rules": [
          {
            "title": "Product team",
            "doctype": "io.cozy.files",
            "values": [
              "357665ec-e797-11ef-94fb-f3d08ccb3ff5"
            ],
            "add": "none",
            "update": "none",
            "remove": "none"
          }
        ]
      },
      "meta": {
        "rev": "1-272ba74b868f"
      },
      "links": {
        "self": "/sharings/aae62886e79611ef8381fb83ff72e425"
      }
    }
  ]
}
```

## Files and directories

Unless stated otherwise, a permission on the whole `io.cozy.files` doctype is
required to use the following routes.

### GET /sharings/drives/:id/download/:file-id

Download a file via a drive share.

Identical call to [`GET /files/download/:file-id`](files.md#get-filesdownloadfile-id) but over a shared drive.
See there for request and response examples

### GET /sharings/drives/:id/_changes

Get the change feed for a drive.

Identical call to [`GET /files/_changes`](files.md#get-files_changes) but over a shared drive.
See there for request and response examples, differences are the URL and:

- Any item that changed for that owner but isn't under that shared drive
  is presented as a deletion.
- Paths are truncated to the shared drive, and formatted accordingly:

  eg: `//io.cozy.files.shared-drives-dir/1/ba3b516812f636fc022f3968f991357a/Meetings/Checklist.txt`

  Schema and it's version, followed by the shared drive ID, and the path within

### GET /sharings/drives/:id/:file-id

Get a directory or a file informations inside a shared drive. In the case of a
directory, it contains the list of files and sub-directories inside it. For a
note, its images are included.

#### Request

```http
GET /sharings/drives/aae62886e79611ef8381fb83ff72e425/af1e1b66e92111ef8ddd5fbac4938703 HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.files",
    "id": "af1e1b66e92111ef8ddd5fbac4938703",
    "meta": {
      "rev": "1-e36ab092"
    },
    "attributes": {
      "type": "directory",
      "name": "Streams",
      "path": "/Product team/Streams",
      "created_at": "2016-09-19T12:35:00Z",
      "updated_at": "2016-09-19T12:35:00Z",
      "tags": [],
      "cozyMetadata": {
        "doctypeVersion": "1",
        "metadataVersion": 1,
        "createdAt": "2016-09-20T18:32:47Z",
        "createdByApp": "drive",
        "createdOn": "https://cozy.example.com/",
        "updatedAt": "2016-09-20T18:32:47Z"
      },
      "driveId": "aae62886e79611ef8381fb83ff72e425"
    },
    "relationships": {
      "contents": {
        "data": [
          {
            "type": "io.cozy.files",
            "id": "a2843318f52411ef8e7ab79eae2f09ab"
          },
          {
            "type": "io.cozy.files",
            "id": "b1db1642f52411efbe0b3bfc5fc0b437"
          }
        ]
      }
    },
    "links": {
      "self": "/files/af1e1b66e92111ef8ddd5fbac4938703"
    }
  },
  "included": [
    {
      "type": "io.cozy.files",
      "id": "a2843318f52411ef8e7ab79eae2f09ab",
      "meta": {
        "rev": "1-ff3beeb456eb"
      },
      "attributes": {
        "type": "directory",
        "name": "Authentication",
        "path": "/Product team/Streams/Authentication",
        "created_at": "2016-09-19T12:35:08Z",
        "updated_at": "2016-09-19T12:35:08Z",
        "cozyMetadata": {
          "doctypeVersion": "1",
          "metadataVersion": 1,
          "createdAt": "2016-09-20T18:32:47Z",
          "createdByApp": "drive",
          "createdOn": "https://cozy.example.com/",
          "updatedAt": "2016-09-20T18:32:47Z"
        }
      }
    },
    {
      "type": "io.cozy.files",
      "id": "b1db1642f52411efbe0b3bfc5fc0b437",
      "meta": {
        "rev": "1-0e6d5b72"
      },
      "attributes": {
        "type": "file",
        "name": "REAMDE.md",
        "trashed": false,
        "md5sum": "ODZmYjI2OWQxOTBkMmM4NQo=",
        "created_at": "2016-09-19T12:38:04Z",
        "updated_at": "2016-09-19T12:38:04Z",
        "tags": [],
        "size": 12,
        "executable": false,
        "class": "document",
        "mime": "text/plain",
        "cozyMetadata": {
          "doctypeVersion": "1",
          "metadataVersion": 1,
          "createdAt": "2016-09-20T18:32:49Z",
          "createdByApp": "drive",
          "createdOn": "https://cozy.example.com/",
          "updatedAt": "2016-09-20T18:32:49Z",
          "uploadedAt": "2016-09-20T18:32:49Z",
          "uploadedOn": "https://cozy.example.com/",
          "uploadedBy": {
            "slug": "drive"
          }
        }
      }
    }
  ]
}
```

### GET /sharings/drives/:id/:file-id/size

This endpoint returns the size taken by the files in a directory inside a shared
drive, including those in subdirectories.

#### Request

```http
GET /sharings/drives/aae62886e79611ef8381fb83ff72e425/af1e1b66e92111ef8ddd5fbac4938703/size HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.files.sizes",
    "id": "af1e1b66e92111ef8ddd5fbac4938703",
    "attributes": {
      "size": "1234567890"
    },
    "meta": {}
  }
}
```

### POST /sharings/drives/:id/:file-id/copy

Duplicates a file.

Identical call to [`POST /files/:file-id/copy`](files.md#post-filesfile-idcopy) but over a shared drive.
See there for request and response examples, the only difference is the URL.

### PATCH /sharings/drives/:id/:file-id

This endpoint can be used to update the metadata of a file or directory, to
rename it or to move it within the same shared drive.

Some specific attributes of the patch can be used:

- `dir_id` attribute can be updated to move a file or directory (the new
  directory needs to be in the same shared drive as the old one)

#### HTTP headers

It's possible to send the `If-Match` header, with the previous revision of the
file/directory (optional).

#### Request

```http
PATCH /sharings/drives/aae62886e79611ef8381fb83ff72e425/9152d568-7e7c-11e6-a377-37cbfb190b4b HTTP/1.1
Accept: application/vnd.api+json
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.files",
    "id": "9152d568-7e7c-11e6-a377-37cbfb190b4b",
    "attributes": {
      "type": "file",
      "name": "hi.txt",
      "dir_id": "f2f36fec-8018-11e6-abd8-8b3814d9a465",
      "tags": ["poem"]
    }
  }
}
```

#### Status codes

- 200 OK, when the file or directory metadata has been successfully updated
- 400 Bad Request, when a the destination directory does not exist
- 403 Forbidden, when the file or directory cannot be modified or the
- destination directory is not accessible
- 404 Not Found, when the file/directory does not exist
- 412 Precondition Failed, when the `If-Match` header is set and doesn't match
  the last revision of the file/directory
- 422 Unprocessable Entity, when the sent data is invalid (for example, the
  parent doesn't exist)

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
Location: https://cozy.example.com/sharings/drives/aae62886e79611ef8381fb83ff72e425/9152d568-7e7c-11e6-a377-37cbfb190b4b
```

```json
{
  "data": {
    "type": "io.cozy.files",
    "id": "9152d568-7e7c-11e6-a377-37cbfb190b4b",
    "meta": {
      "rev": "1-0e6d5b72"
    },
    "attributes": {
      "type": "file",
      "name": "hi.txt",
      "trashed": false,
      "md5sum": "ODZmYjI2OWQxOTBkMmM4NQo=",
      "created_at": "2016-09-19T12:38:04Z",
      "updated_at": "2016-09-19T12:38:04Z",
      "tags": ["poem"],
      "size": 12,
      "executable": false,
      "class": "document",
      "mime": "text/plain",
      "cozyMetadata": {
        "doctypeVersion": "1",
        "metadataVersion": 1,
        "createdAt": "2016-09-20T18:32:49Z",
        "createdByApp": "drive",
        "createdOn": "https://cozy.example.com/",
        "updatedAt": "2016-09-22T13:32:51Z",
        "uploadedAt": "2016-09-21T04:27:50Z",
        "uploadedOn": "https://cozy.example.com/",
        "uploadedBy": {
          "slug": "drive"
        }
      }
    },
    "relationships": {
      "parent": {
        "links": {
          "related": "/files/f2f36fec-8018-11e6-abd8-8b3814d9a465"
        },
        "data": {
          "type": "io.cozy.files",
          "id": "f2f36fec-8018-11e6-abd8-8b3814d9a465"
        }
      }
    },
    "links": {
      "self": "/files/9152d568-7e7c-11e6-a377-37cbfb190b4b"
    }
  }
}
```

## Versions

The identifier of the `io.cozy.files.versions` is composed of the `file-id` and
another string called the `version-id`, separated by a `/`. So, when a route
makes reference to `/something/:file-id/:version-id`, you can use the identifier
of the version document (without having to prepend the file identifier).

### GET /sharings/drives/:id/download/:file-id/:version-id

Downloads an old version of the file content.

Identical call to [`GET /files/download/:file-id/:version-id`](files.md#get-filesdownloadfile-idversion-id)
but over a shared drive. See there for request and response examples, the only
difference is the URL.

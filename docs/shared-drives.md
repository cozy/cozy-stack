[Table of contents](README.md#table-of-contents)

# Shared drives

A shared drive is a folder that is shared between several cozy instances. A
member doesn't have the files in their Cozy, but can access them via the stack
playing a proxy role.

To create a shared drive (typically on the organization Cozy), we need the
following steps:

1. Ensure that the `/Drive` folder exists in the cozy instance with the
   [`POST /files/shared-drives`](https://docs.cozy.io/en/cozy-stack/files/#post-filesshared-drives)
   route.
2. Create a folder inside it, with the name of shared drive.
3. Create a sharing with the `drive: true` attribute, and one rule for
   shared folder (with `none` for `add`, `update` and `remove` attributes).

# Routes

A permission on the whole `io.cozy.files` doctype is required to use the
following routes.

## GET /sharings/drives

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
      }
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

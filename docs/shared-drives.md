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

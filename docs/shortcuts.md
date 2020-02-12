[Table of contents](README.md#table-of-contents)

# Shortcuts

A shortcut is a file in the VFS with a `.url` extension. It is served with the
`application/internet-shortcut` mime-type. The stack provides a few routes to
help manipulate them.

## POST /shortcuts

This route can be used to create a shortcut. You can create a shortcut using
the `POST /files/:dir-id` route, but the content must respect the `.url` file
format. This route offers an easier way to do that.

**Note:** a permission to create a file is required to use this route.

### Request

```http
POST /shortcuts HTTP/1.1
Host: alice.cozy.example
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.files.shortcuts",
    "attributes": {
      "name": "sunset.jpg.url",
      "dir_id": "629fb233be550a21174ac8e19f003e4a",
      "url": "https://alice-photos.cozy.example/#/photos/629fb233be550a21174ac8e19f0043af",
      "metadata": {
        "target": {
          "cozyMetadata": {
            "instance": "https://alice.cozy.example/"
          },
          "app": "photos",
          "_type": "io.cozy.files",
          "mime": "image/jpg"
        }
      }
    }
  }
}
```

### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
```

```json
{
  "_id": "629fb233be550a21174ac8e19f0043af",
  "_rev": "1-61c7804bdb4f9f8dae5a363cb9a30dd8",
  "type": "file",
  "name": "sunset.jpg.url",
  "dir_id": "629fb233be550a21174ac8e19f003e4a",
  "trashed": false,
  "md5sum": "vfEMDpJShs8QeIlsDmw9VA==",
  "created_at": "2020-02-10T20:38:04Z",
  "updated_at": "2020-02-10T20:38:04Z",
  "tags": [],
  "metadata": {
    "target": {
      "cozyMetadata": {
        "instance": "https://alice.cozy.example/"
      },
      "app": "photos",
      "_type": "io.cozy.files",
      "mime": "image/jpg"
    }
  },
  "size": 62,
  "executable": false,
  "class": "shortcut",
  "mime": "application/shortcut",
  "cozyMetadata": {
    "doctypeVersion": 1,
    "metadataVersion": 1,
    "createdAt": "2020-02-10T20:38:04Z",
    "createdOn": "https://bob.cozy.example/",
    "updatedAt": "2020-02-10T20:38:04Z",
    "uploadedAt": "2020-02-10T20:38:04Z",
    "uploadedOn": "https://bob.cozy.example/"
  }
}
```

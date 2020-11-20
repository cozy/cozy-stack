[Table of contents](README.md#table-of-contents)

# Not synchronized directories

## What we want?

The directories in the Virtual File System can be ignored for synchronization
purpose by some clients. For example, a user may want to not synchronize their
`Videos` directory as it takes too much space for their laptop. Or they may want
to not synchronize a directory with personal documents on the desktop owned by
their employer. By default, all the directories are synchronized everywhere but
it is possible to tell for each directory the devices where the directory won't
be synchronized. The stack tracks this with a `not_synchronized_on` field on
the directory documents.

### Example

```json
{
  "data": {
    "type": "io.cozy.files",
    "id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "meta": {
      "rev": "1-ff3beeb456eb"
    },
    "attributes": {
      "type": "directory",
      "name": "phone",
      "path": "/Documents/phone",
      "created_at": "2016-09-19T12:35:08Z",
      "updated_at": "2016-09-19T12:35:08Z",
      "tags": ["bills", "konnectors"],
      "cozyMetadata": {
        "doctypeVersion": "1",
        "metadataVersion": 1,
        "createdAt": "2016-09-20T18:32:48Z",
        "createdByApp": "drive",
        "createdOn": "https://cozy.example.com/",
        "updatedAt": "2016-09-20T18:32:48Z"
      }
    },
    "relationships": {
      "parent": {
        "links": {
          "related": "/files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81"
        },
        "data": {
          "type": "io.cozy.files",
          "id": "fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81"
        }
      },
      "not_synchronized_on": {
        "links": {
          "self": "/files/6494e0ac-dfcb-11e5-88c1-472e84a9cbee/relationships/not_synchronized_on"
        },
        "data": [
          {
            "type": "io.cozy.oauth.clients",
            "id": "653dfdb0-0595-0139-92df-543d7eb8149c"
          }
        ]
      }
    },
    "links": {
      "self": "/files/6494e0ac-dfcb-11e5-88c1-472e84a9cbee"
    }
  }
}
```

## Routes

### POST /files/:dir-id/relationships/not_synchronized_on

Ask to not synchronize a directory on one or several devices.

#### Request

```http
POST /files/6494e0ac-dfcb-11e5-88c1-472e84a9cbee/relationships/not_synchronized_on HTTP/1.1
Content-Type: application/vnd.api+json
Accept: application/vnd.api+json
```

```json
{
    "data": [
        {
            "type": "io.cozy.oauth.clients",
            "id": "f9ef4dc0-0596-0139-92e0-543d7eb8149c"
        }
    ]
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
    "meta": {
        "rev": "3-485d439530110",
        "count": 2
    },
    "data": [
        {
            "type": "io.cozy.oauth.clients",
            "id": "653dfdb0-0595-0139-92df-543d7eb8149c"
        },
        {
            "type": "io.cozy.oauth.clients",
            "id": "f9ef4dc0-0596-0139-92e0-543d7eb8149c"
        }
    ]
}
```

### DELETE /files/:file-id/relationships/not_synchronized_on

Ask to synchronize again the directory on one or several devices.

#### Request

```http
DELETE /files/6494e0ac-dfcb-11e5-88c1-472e84a9cbee/relationships/not_synchronized_on HTTP/1.1
Content-Type: application/vnd.api+json
Accept: application/vnd.api+json
```

```json
{
    "data": [
        {
            "type": "io.cozy.oauth.clients",
            "id": "f9ef4dc0-0596-0139-92e0-543d7eb8149c"
        }
    ]
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
    "meta": {
        "rev": "4-1f7ef1be3cb",
        "count": 1
    },
    "data": [
        {
            "type": "io.cozy.oauth.clients",
            "id": "653dfdb0-0595-0139-92df-543d7eb8149c"
        }
    ]
}
```

### GET /data/:type/:doc-id/relationships/not_synchronizing

Returns all the directory ids that are not synchronized on the given device.

Contents is paginated following [jsonapi conventions](jsonapi.md#pagination).
The default limit is 100 entries. The maximal number of entries per page is
1000.

It is possible to include the whole documents for the directories by adding
`include=files` to the query string.

#### Request

```http
GET /data/io.cozy.oauth.clients/653dfdb0-0595-0139-92df-543d7eb8149c/relationships/not_synchronizing HTTP/1.1
Content-Type: application/vnd.api+json
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
            "type": "io.cozy.files",
            "id": "11400320-07b7-0139-4fe8-543d7eb8149c"
        },
        {
            "type": "io.cozy.files",
            "id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee"
        }
    ]
}
```

### POST /data/:type/:doc-id/relationships/not_synchronizing

When configuring a device, it's tedious to add the `not_synchronized_on` for
each directory individually. This route allows to make it in bulk.

#### Request

```http
POST /data/io.cozy.oauth.clients/653dfdb0-0595-0139-92df-543d7eb8149c/relationships/not_synchronizing HTTP/1.1
Content-Type: application/vnd.api+json
Accept: application/vnd.api+json
```

```json
{
    "data": [
        {
            "type": "io.cozy.files",
            "id": "38086350-07c0-0139-4fe9-543d7eb8149c"
        },
        {
            "type": "io.cozy.files",
            "id": "3d447470-07c0-0139-4fea-543d7eb8149c"
        }
    ]
}
```

#### Response

```http
HTTP/1.1 204 No Content
Content-Type: application/vnd.api+json
```

**Note**: if one of the id is a file, the response will be a 400 Bad Request.
References are only for directories.

### DELETE /data/:type/:doc-id/relationships/not_synchronizing

This bulk deletion of not_synchronized_on on many directories can be useful
when configuring a device.

#### Request

```http
DELETE /data/io.cozy.oauth.clients/653dfdb0-0595-0139-92df-543d7eb8149c/relationships/not_synchronizing HTTP/1.1
Content-Type: application/vnd.api+json
Accept: application/vnd.api+json
```

```json
{
    "data": [
        {
            "type": "io.cozy.files",
            "id": "38086350-07c0-0139-4fe9-543d7eb8149c"
        },
        {
            "type": "io.cozy.files",
            "id": "3d447470-07c0-0139-4fea-543d7eb8149c"
        }
    ]
}
```

#### Response

```http
HTTP/1.1 204 No Content
Content-Type: application/vnd.api+json
```

## Usage

When an OAuth client makes a request for the changes feed on the
`io.cozy.files` doctype (via `/data/io.cozy.files/_changes`), the output will
be filtered. If a directory or file is inside a directory with the
`not_synchronized_on` attribute set on for this client, the document will be
replaced by a fake entry with `_deleted: true`.

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

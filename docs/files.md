[Table of contents](README.md#table-of-contents)

# Virtual File System

Cozy applications can use files for storing binary content, like photos or bills
in PDF. This service offers a REST API to manipulate easily without having to
know the underlying storage layer. The metadata are kept in CouchDB, but the
binaries can go to the local system, or a Swift instance.

## Directories

A directory is a container for files and sub-directories.

Its path is the path of its parent, a slash (`/`), and its name. It's case
sensitive.

### Root directory

The root of the virtual file system is a special directory with id
`io.cozy.files.root-dir`.

You can use it in any request where you would use a directory, except you cannot
delete it.

### POST /files/:dir-id

Create a new directory. The `dir-id` parameter is optional. When it's not given,
the directory is created at the root of the virtual file system.

#### Query-String

| Parameter | Description        |
| --------- | ------------------ |
| Type      | `directory`        |
| Name      | the directory name |
| Tags      | an array of tags   |

#### HTTP headers

| Parameter | Description                            |
| --------- | -------------------------------------- |
| Date      | The modification date of the directory |

#### Request

```http
POST /files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81?Type=directory&Name=phone&Tags=bills,konnectors HTTP/1.1
Accept: application/vnd.api+json
Date: Mon, 19 Sep 2016 12:35:08 GMT
```

#### Status codes

* 201 Created, when the directory has been successfully created
* 404 Not Found, when the parent directory does not exist
* 409 Conflict, when a directory with the same name already exists
* 413 Payload Too Large, when there is not enough available space on the cozy to upload the file
* 422 Unprocessable Entity, when the `Type` or `Name` parameter is missing or
  invalid

#### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
Location: http://cozy.example.com/files/6494e0ac-dfcb-11e5-88c1-472e84a9cbee
```

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
      "tags": ["bills"]
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
      }
    },
    "links": {
      "self": "/files/6494e0ac-dfcb-11e5-88c1-472e84a9cbee"
    }
  }
}
```

### GET /files/:file-id

Get a directory or a file informations. In the case of a directory, it contains
the list of files and sub-directories inside it.

Contents is paginated following [jsonapi conventions](jsonapi.md#pagination).
The default limit is 30 entries.

#### Request

```http
GET /files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81 HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "links": {
    "next":
      "/files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81?page[cursor]=9152d568-7e7c-11e6-a377-37cbfb190b4b"
  },
  "data": {
    "type": "io.cozy.files",
    "id": "fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81",
    "meta": {
      "rev": "1-e36ab092"
    },
    "attributes": {
      "type": "directory",
      "name": "Documents",
      "path": "/Documents",
      "created_at": "2016-09-19T12:35:00Z",
      "updated_at": "2016-09-19T12:35:00Z",
      "tags": []
    },
    "relationships": {
      "contents": {
        "data": [
          {
            "type": "io.cozy.files",
            "id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee"
          },
          {
            "type": "io.cozy.files",
            "id": "9152d568-7e7c-11e6-a377-37cbfb190b4b"
          }
        ]
      }
    },
    "links": {
      "self": "/files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81"
    }
  },
  "included": [
    {
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
        "tags": ["bills"]
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
        }
      },
      "links": {
        "self": "/files/6494e0ac-dfcb-11e5-88c1-472e84a9cbee"
      }
    },
    {
      "type": "io.cozy.files",
      "id": "9152d568-7e7c-11e6-a377-37cbfb190b4b",
      "meta": {
        "rev": "1-0e6d5b72"
      },
      "attributes": {
        "type": "file",
        "name": "hello.txt",
        "trashed": false,
        "md5sum": "ODZmYjI2OWQxOTBkMmM4NQo=",
        "created_at": "2016-09-19T12:38:04Z",
        "updated_at": "2016-09-19T12:38:04Z",
        "tags": [],
        "size": 12,
        "executable": false,
        "class": "document",
        "mime": "text/plain"
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
        }
      },
      "links": {
        "self": "/files/9152d568-7e7c-11e6-a377-37cbfb190b4b"
      }
    }
  ]
}
```

### DELETE /files/:dir-id

Put a directory and its subtree in the trash.

#### HTTP headers

It's possible to send the `If-Match` header, with the previous revision of the
file/directory (optional).

## Files

A file is a binary content with some metadata.

### POST /files/:dir-id

Upload a file

#### Query-String

| Parameter  | Description                                        |
| ---------- | -------------------------------------------------- |
| Type       | `file`                                             |
| Name       | the file name                                      |
| Tags       | an array of tags                                   |
| Executable | `true` if the file is executable (UNIX permission) |

#### HTTP headers

| Parameter      | Description                                 |
| -------------- | ------------------------------------------- |
| Content-Length | The file size                               |
| Content-MD5    | A Base64-encoded binary MD5 sum of the file |
| Content-Type   | The mime-type of the file                   |
| Date           | The modification date of the file           |

#### Request

```http
POST /files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81?Type=file&Name=hello.txt HTTP/1.1
Accept: application/vnd.api+json
Content-Length: 12
Content-MD5: hvsmnRkNLIX24EaM7KQqIA==
Content-Type: text/plain
Date: Mon, 19 Sep 2016 12:38:04 GMT

Hello world!
```

#### Status codes

* 201 Created, when the file has been successfully created
* 404 Not Found, when the parent directory does not exist
* 409 Conflict, when a file with the same name already exists
* 412 Precondition Failed, when the md5sum is `Content-MD5` is not equal to the
  md5sum computed by the server
* 422 Unprocessable Entity, when the sent data is invalid (for example, the
  parent doesn't exist, `Type` or `Name` parameter is missing or invalid, etc.)

#### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
Location: http://cozy.example.com/files/9152d568-7e7c-11e6-a377-37cbfb190b4b
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
      "name": "sunset.jpg",
      "trashed": false,
      "md5sum": "ODZmYjI2OWQxOTBkMmM4NQo=",
      "created_at": "2016-09-19T12:38:04Z",
      "updated_at": "2016-09-19T12:38:04Z",
      "tags": [],
      "metadata": {
        "datetime": "2016-09-18T20:38:04Z",
        "height": 1080,
        "width": 1920
      },
      "size": 12,
      "executable": false,
      "class": "image",
      "mime": "image/jpg"
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
      "referenced_by": {
        "links": {
          "self":
            "/files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81/relationships/references"
        },
        "data": [
          {
            "type": "io.cozy.albums",
            "id": "94375086-e2e2-11e6-81b9-5bc0b9dd4aa4"
          }
        ]
      }
    },
    "links": {
      "self": "/files/9152d568-7e7c-11e6-a377-37cbfb190b4b",
      "small":
        "/files/9152d568-7e7c-11e6-a377-37cbfb190b4b/thumbnails/0f9cda56674282ac/small",
      "medium":
        "/files/9152d568-7e7c-11e6-a377-37cbfb190b4b/thumbnails/0f9cda56674282ac/medium",
      "large":
        "/files/9152d568-7e7c-11e6-a377-37cbfb190b4b/thumbnails/0f9cda56674282ac/large"
    }
  }
}
```

**Note**: see [references of documents in VFS](references-docs-in-vfs.md) for
more informations about the references field.

### GET /files/download/:file-id

Download the file content.

By default the `content-disposition` will be `inline`, but it will be
`attachment` if the query string contains the parameter `Dl=1`

#### Request

```http
GET /files/download/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81 HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Length: 12
Content-Disposition: inline; filename="hello.txt"
Content-Type: text/plain

Hello world!
```

### GET /files/download

Download the file content from its path.

By default the `content-disposition` will be `inline`, but it will be
`attachment` if the query string contains the parameter `Dl=1`

#### Request

```http
GET /files/download?Path=/Documents/hello.txt&Dl=1 HTTP/1.1
```

### GET /files/:file-id/thumbnails/:secret/:format

Get a thumbnail of a file (for an image only). `:format` can be `small`
(640x480), `medium` (1280x720), or `large` (1920x1080).

### PUT /files/:file-id

Overwrite a file

#### HTTP headers

The HTTP headers are the same than for uploading a file. There is one additional
header, `If-Match`, with the previous revision of the file (optional).

#### Request

```http
PUT /files/9152d568-7e7c-11e6-a377-37cbfb190b4b HTTP/1.1
Accept: application/vnd.api+json
Content-Length: 12
Content-MD5: hvsmnRkNLIX24EaM7KQqIA==
Content-Type: text/plain
Date: Mon, 20 Sep 2016 16:43:12 GMT
If-Match: 1-0e6d5b72

HELLO WORLD!
```

#### Status codes

* 200 OK, when the file has been successfully overwritten
* 404 Not Found, when the file wasn't existing
* 412 Precondition Failed, when the `If-Match` header is set and doesn't match
  the last revision of the file

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.files",
    "id": "9152d568-7e7c-11e6-a377-37cbfb190b4b",
    "meta": {
      "rev": "2-d903b54c"
    },
    "attributes": {
      "type": "file",
      "name": "hello.txt",
      "trashed": false,
      "md5sum": "YjU5YmMzN2Q2NDQxZDk2Nwo=",
      "created_at": "2016-09-19T12:38:04Z",
      "updated_at": "2016-09-19T12:38:04Z",
      "tags": [],
      "size": 12,
      "executable": false,
      "class": "document",
      "mime": "text/plain"
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
      }
    },
    "links": {
      "self": "/files/9152d568-7e7c-11e6-a377-37cbfb190b4b"
    }
  }
}
```

### DELETE /files/:file-id

Put a file in the trash.

## Common

### GET /files/metadata

Same as `/files/:file-id` but to retrieve informations from a path.

#### Request

```http
GET /files/metadata?Path=/Documents/hello.txt HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
Location: http://cozy.example.com/files/9152d568-7e7c-11e6-a377-37cbfb190b4b
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
      "name": "hello.txt",
      "trashed": false,
      "md5sum": "ODZmYjI2OWQxOTBkMmM4NQo=",
      "created_at": "2016-09-19T12:38:04Z",
      "updated_at": "2016-09-19T12:38:04Z",
      "tags": [],
      "size": 12,
      "executable": false,
      "class": "document",
      "mime": "text/plain"
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
      }
    },
    "links": {
      "self": "/files/9152d568-7e7c-11e6-a377-37cbfb190b4b"
    }
  }
}
```

### PATCH /files/:file-id and PATCH /files/metadata

Both endpoints can be used to update the metadata of a file or directory, or to
rename/move it. The difference is the first one uses an id to identify the
file/directory to update, and the second one uses the path.

Some specific attributes of the patch can be used:
  - `dir_id` attribute can be updated to move a file or directory
  - `move_to_trash` boolean to specify that the file needs to be moved to the trash
  - `permanent_delete` boolean to specify that the files needs to be deleted (after being trashed)

#### HTTP headers

It's possible to send the `If-Match` header, with the previous revision of the
file/directory (optional).

#### Request

```http
PATCH /files/9152d568-7e7c-11e6-a377-37cbfb190b4b HTTP/1.1
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

* 200 OK, when the file or directory metadata has been successfully updated
* 400 Bad Request, when a the directory is asked to move to one of its
  sub-directories
* 404 Not Found, when the file/directory wasn't existing
* 412 Precondition Failed, when the `If-Match` header is set and doesn't match
  the last revision of the file/directory
* 422 Unprocessable Entity, when the sent data is invalid (for example, the
  parent doesn't exist)

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
Location: http://cozy.example.com/files/9152d568-7e7c-11e6-a377-37cbfb190b4b
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
      "mime": "text/plain"
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

### PATCH /files/

Endpoint to update the metadata of files and directories in batch. It can be
used, for instance, to move many files in a single request.

#### Request

```http
PATCH /files/
Content-Type: application/vnd.api+json
```

```json
{
  "data": [
    {
      "type": "io.cozy.files",
      "id": "9152d568-7e7c-11e6-a377-37cbfb190b4b",
      "meta": { "rev": "1-0e6d5b72" },
      "attributes": { "dir_id": "f2f36fec-8018-11e6-abd8-8b3814d9a465" }
    },
    {
      "type": "io.cozy.files",
      "id": "9152d568-7e7c-11e6-a377-37cbfb190b4c",
      "meta": { "rev": "2-123123" },
      "attributes": { "move_to_trash": true }
    }
  ]
}
```

#### Status codes

The same status codes can be encountered as the `PATCH /files/:file-id` route.

### POST /files/archive

Create an archive. The body of the request lists the files and directories that
will be included in the archive. For directories, it includes all the files and
sub-directories in the archive.

It's possible to give a file by its id (in the `ids` array) or by its path (in
the `files` array).

#### Request

```http
POST /files/archive HTTP/1.1
Accept: application/zip
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.files.archives",
    "attributes": {
      "name": "project-X",
      "ids": ["a51aeeea-4f79-11e7-9dc4-83f67e9494ab"],
      "files": [
        "/Documents/bills",
        "/Documents/images/sunset.jpg",
        "/Documents/images/eiffel-tower.jpg"
      ]
    }
  }
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "links": {
    "related": "/files/archive/4521DC87/project-X.zip"
  },
  "data": {
    "type": "io.cozy.files.archives",
    "id": "4521DC87",
    "attributes": {
      "href": "/files/archive/4521DC87/project-X.zip"
    }
  }
}
```

### GET /files/archive/:key/:name

Download a previously created archive. The name parameter is not used in the
stack but aims to allow setting a name even for browser / downloader that do not
support Content-Disposition filename.

**This route does not require Basic Authentification**

```http
GET /files/archive/4521DC87/project-X.zip HTTP/1.1
Accept: application/zip
Content-Length: 12345
Content-Disposition: attachment; filename="project-X.zip"
Content-Type: application/zip
```

### POST /files/downloads?Path=file_path

Create a file download. The Path query parameter specifies the file to download.
The response json API links contains a `related` link for downloading the file,
see below.

### POST /files/downloads?Id=file_id

Also create a file download. But it takes the id of the file and not its path.

### GET /files/downloads/:secret/:name

Allows to download a file with a secret created from the route above.

The name parameter is not used in the stack but aims to allow setting a name
even for browser / downloader that do not support Content-Disposition filename.

By default the `content-disposition` will be `inline`, but it will be
`attachment` if the query string contains the parameter `Dl=1`

**This route does not require Basic Authentification**

## Trash

When a file is deleted, it is first moved to the trash. In the trash, it can be
restored. Or, after some time, it will be removed from the trash and permanently
destroyed.

The file `trashed` attribute will be set to true.

### GET /files/trash

List the files inside the trash. It's paginated.

### Query-String

| Parameter    | Description                           |
| ------------ | ------------------------------------- |
| page[cursor] | the last id of the results            |
| page[limit]  | the number of entries (30 by default) |

#### Request

```http
GET /files/trash HTTP/1.1
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
      "id": "df24aac0-7f3d-11e6-81c0-d38812bfa0a8",
      "meta": {
        "rev": "1-3b75377c"
      },
      "attributes": {
        "type": "file",
        "name": "foo.txt",
        "trashed": true,
        "md5sum": "YjAxMzQxZTc4MDNjODAwYwo=",
        "created_at": "2016-09-19T12:38:04Z",
        "updated_at": "2016-09-19T12:38:04Z",
        "tags": [],
        "size": 123,
        "executable": false,
        "class": "document",
        "mime": "text/plain"
      },
      "links": {
        "self": "/files/trash/df24aac0-7f3d-11e6-81c0-d38812bfa0a8"
      }
    },
    {
      "type": "io.cozy.files",
      "id": "4a4fc582-7f3e-11e6-b9ca-278406b6ddd4",
      "meta": {
        "rev": "1-4a09030e"
      },
      "attributes": {
        "type": "file",
        "name": "bar.txt",
        "trashed": true,
        "md5sum": "YWVhYjg3ZWI0OWQzZjRlMAo=",
        "created_at": "2016-09-19T12:38:04Z",
        "updated_at": "2016-09-19T12:38:04Z",
        "tags": [],
        "size": 456,
        "executable": false,
        "class": "document",
        "mime": "text/plain"
      },
      "links": {
        "self": "/files/trash/4a4fc582-7f3e-11e6-b9ca-278406b6ddd4"
      }
    }
  ]
}
```

### POST /files/trash/:file-id

Restore the file with the `file-id` identifiant.

The file's `trashed` attributes will be set to false.

### DELETE /files/trash/:file-id

Destroy the file and make it unrecoverable (it will still be available in
backups).

### DELETE /files/trash

Clear out the trash.

## Trashed attribute

All files that are inside the trash will have a `trashed: true` attribute. This
attribute can be used in mango queries to only get "interesting" files.

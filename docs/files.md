Virtual File System
===================

Cozy applications can use files for storing binary content, like photos or
bills in PDF. This service offers a REST API to manipulate easily without
having to know the underlying storage layer. The metadata are kept in CouchDB,
but the binaries can go to the local system, or a Swift instance.


Folders
-------

A folder is a container for files and sub-folders.

Its path is the path of its parent, a slash (`/`), and its name. It's case
sensitive.

### POST /files/:folder-id

Create a new folder. The `folder-id` parameter is optional. When it's not
given, the folder is created at the root of the virtual file system.

#### Query-String

Parameter | Description
----------|------------------
Type      | `io.cozy.folders`
Name      | the folder name
Tags      | an array of tags

#### Request

```http
POST
/files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81?Type=io.cozy.folders&Name=phone&Tags=bills,konnectors HTTP/1.1
Accept: application/vnd.api+json
```

#### Status codes

* 201 Created, when the folder has been successfully created
* 404 Not Found, when the parent folder does not exist
* 409 Conflict, when a directory with the same name already exists
* 422 Unprocessable Entity, when the `Type` or `Name` parameter is missing or invalid

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
    "rev": "1-ff3beeb456eb",
    "attributes": {
      "type": "directory",
      "name": "phone",
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

### GET /files/:folder-id

Get the folder informations and the list of files and sub-folders inside it.
Contents is paginated. By default, only the 100 first entries are given.

### Query-String

Parameter    | Description
-------------|---------------------------------------
page[cursor] | the last id of the results
page[limit]  | the number of entries (100 by default)

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
    "next": "/files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81?page[cursor]=9152d568-7e7c-11e6-a377-37cbfb190b4b"
  },
  "data": {
    "type": "io.cozy.files",
    "id": "fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81",
    "rev": "1-e36ab092",
    "attributes": {
      "type": "directory",
      "name": "Documents",
      "created_at": "2016-09-19T12:35:00Z",
      "updated_at": "2016-09-19T12:35:00Z",
      "tags": []
    },
    "relationships": {
      "contents": {
        "data": [
          { "type": "io.cozy.files", "id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee" },
          { "type": "io.cozy.files", "id": "9152d568-7e7c-11e6-a377-37cbfb190b4b" }
        ]
      }
    },
    "links": {
      "self": "/files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81"
    }
  },
  "included": [{
    "type": "io.cozy.files",
    "id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "rev": "1-ff3beeb456eb",
    "attributes": {
      "type": "directory",
      "name": "phone",
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
  }, {
    "type": "io.cozy.files",
    "id": "9152d568-7e7c-11e6-a377-37cbfb190b4b",
    "rev": "1-0e6d5b72",
    "attributes": {
      "type": "file",
      "name": "hello.txt",
      "md5sum": "86fb269d190d2c85f6e0468ceca42a20",
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
  }]
}
```

### DELETE /files/:folder-id

Put a folder and its subtree in the trash.


Files
-----

A file is a binary content with some metadata.

### POST /files/:folder-id

Upload a file

#### Query-String

Parameter | Description
----------|---------------------------------------------------
Type      | `io.cozy.files`
Name      | the file name
Tags      | an array of tags
Executable| `true` if the file is executable (UNIX permission)

#### HTTP headers

Parameter     | Description
--------------|--------------------------------------------
Content-Length| The file size
Content-MD5   | A Base64-encoded binary MD5 sum of the file
Content-Type  | The mime-type of the file
Date          | The modification date of the file

#### Request

```http
POST /files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81?Type=io.cozy.files&Name=hello.txt HTTP/1.1
Accept: application/vnd.api+json
Content-Length: 12
Content-MD5: hvsmnRkNLIX24EaM7KQqIA==
Content-Type: text/plain
Date: Mon, 19 Sep 2016 12:38:04 GMT

Hello world!
```

#### Status codes

* 201 Created, when the file has been successfully created
* 404 Not Found, when the parent folder does not exist
* 409 Conflict, when a file with the same name already exists
* 412 Precondition Failed, when the md5sum is `Content-MD5` is not equal to the md5sum computed by the server
* 422 Unprocessable Entity, when the sent data is invalid (for example, the parent doesn't exist, `Type` or `Name` parameter is missing or invalid, etc.)

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
    "rev": "1-0e6d5b72",
    "attributes": {
      "type": "file",
      "name": "hello.txt",
      "md5sum": "86fb269d190d2c85f6e0468ceca42a20",
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

**Note**: for an image, the links section will also include a link called
`thumbnail` to the thumbnail URL of the image.

### GET /files/:file-id

Get the file content

#### Request

```http
GET /files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81 HTTP/1.1
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

Download a file (its content) from its path

#### Request

```http
GET /files/download?Path=/Documents/hello.txt HTTP/1.1
```

### GET /files/:file-id/thumbnail

Get a thumbnail of a file (for an image only).

### PUT /files/:file-id

Overwrite a file

#### HTTP headers

The HTTP headers are the same than for uploading a file. There is one
additional header, `If-Match`, with the previous revision of the file
(optional).

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
* 412 Precondition Failed, when the `If-Match` header is set and doesn't match the last revision of the file

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
    "rev": "2-d903b54c",
    "attributes": {
      "type": "file",
      "name": "hello.txt",
      "md5sum": "b59bc37d6441d96785bda7ab2ae98f75",
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


Common
------

### GET /files/metadata

Get metadata about a file (or folder) from its path

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
    "rev": "1-0e6d5b72",
    "attributes": {
      "type": "file",
      "name": "hello.txt",
      "md5sum": "86fb269d190d2c85f6e0468ceca42a20",
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

Both endpoints can be used to update the metadata of a file or folder, or to
rename/move it. The difference is the first one uses an id to identify the
file/folder to update, and the second one uses the path.

The parent relationship can be updated to move a file or folder.

#### HTTP headers

It's possible to send the `If-Match` header, with the previous revision of the
file/folder (optional).

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
      "tags": ["poem"]
    },
    "relationships": {
      "parent": {
        "data": {
          "type": "io.cozy.files",
          "id": "f2f36fec-8018-11e6-abd8-8b3814d9a465"
        }
      }
    }
  }
}
```

#### Status codes

* 200 OK, when the file or folder metadata has been successfully updated
* 400 Bad Request, when a the folder is asked to move to one of its sub-folders
* 404 Not Found, when the file/folder wasn't existing
* 412 Precondition Failed, when the `If-Match` header is set and doesn't match the last revision of the file/folder
* 422 Unprocessable Entity, when the sent data is invalid (for example, the parent doesn't exist)

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
    "rev": "1-0e6d5b72",
    "attributes": {
      "type": "file",
      "name": "hi.txt",
      "md5sum": "86fb269d190d2c85f6e0468ceca42a20",
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

### POST /files/archive

Create an archive and download it. The body of the request lists the files and
folders that will be included in the archive. For folders, it includes all the
files and sub-folders in the archive.

#### Request

```http
POST /files/archive HTTP/1.1
Accept: application/zip
Content-Type: application/vnd.api+json
```

```json
{
  "data": [
    "/Documents/bills",
    "/Documents/images/sunset.jpg",
    "/Documents/images/eiffel-tower.jpg"
  ]
}
```


Trash
-----

When a file is deleted, it is first moved to the trash. In the trash, it can
be restored. Or, after some time, it will be removed from the trash and
permanently destroyed.

### GET /files/trash

List the files inside the trash. It's paginated.

### Query-String

Parameter    | Description
-------------|---------------------------------------
page[cursor] | the last id of the results
page[limit]  | the number of entries (100 by default)

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
  "data": [{
    "type": "io.cozy.files",
    "id": "df24aac0-7f3d-11e6-81c0-d38812bfa0a8",
    "rev": "1-3b75377c",
    "attributes": {
      "type": "file",
      "name": "foo.txt",
      "md5sum": "b01341e7803c800cc8db4de46f377a87",
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
  }, {
    "type": "io.cozy.files",
    "id": "4a4fc582-7f3e-11e6-b9ca-278406b6ddd4",
    "rev": "1-4a09030e",
    "attributes": {
      "type": "file",
      "name": "bar.txt",
      "md5sum": "aeab87eb49d3f4e0e5625ada9b49f8e1",
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
  }]
}
```

### POST /files/trash/:file-id

Restore the file with the `file-id` identifiant.

### DELETE /files/trash/:file-id

Destroy the file and make it unrecoverable (it will still be available in
backups).

### DELETE /files/trash

Clear out the trash.

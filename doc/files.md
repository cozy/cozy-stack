Virtual File System
===================

Cozy applications can use files for storing binary content, like photos or
bills in PDF. This service offers a REST API to manipulate easily without
having to know the underlying storage layer. The metadata are kept in CouchDB,
but the binaries can go to the local system, or a Swift instance.

**TODO** move/rename files and folders
**TODO** overwrite an existing file
**TODO** update metadata of a file or folder
**TODO** look at [Content-Disposition](https://www.ietf.org/rfc/rfc2183.txt)


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
----------|------------
type      | `folder`
name      | the folder name
tags      | an array of tags

#### Request

```http
POST /files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81?type=folder&name=phone&tags[]=bills HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
Location: http://cozy.example.com/files/6494e0ac-dfcb-11e5-88c1-472e84a9cbee
```

```json
{
  "data": {
    "type": "folder",
    "id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "attributes": {
      "rev": "1-ff3beeb456eb",
      "name": "phone",
      "created_at": "2016-09-19T12:35:08Z",
      "updated_at": "2016-09-19T12:35:08Z",
      "tags": ["bills"]
    },
    "links": {
      "self": "/files/6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
      "parent": "/files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81"
    }
  }
}
```

### GET /files/:folder-id

Get the folder informations and the list of files and sub-folders inside it.

**TODO** example

### DELETE /files/:folder-id

Put a folder and its subtree in the trash.


Files
-----

A file is a binary content with some metadata.

### POST /files/:folder-id

#### Query-String

Parameter | Description
----------|------------
type      | `file`
name      | the file name
tags      | an array of tags
executable| `true` if the file is executable (UNIX permission)

#### HTTP headers

Parameter     | Description
--------------|------------
Content-Length| The file size
Content-MD5   | A Base64-encoded binary MD5 sum of the file
Content-Type  | The mime-type of the file
Date          | The modification date of the file

**Note:** if the md5 sum in `Content-MD5` is not equal to the md5 sum computed
on the server, the server responds with a `412 Precondition Failed`.

#### Request

```http
POST /files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81?type=file&name=hello.txt HTTP/1.1
Accept: application/vnd.api+json
Content-Length: 12
Content-MD5: hvsmnRkNLIX24EaM7KQqIA==
Content-Type: text/plain
Date: Mon, 19 Sep 2016 12:38:04 GMT

Hello world!
```

#### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
Location: http://cozy.example.com/files/9152d568-7e7c-11e6-a377-37cbfb190b4b
```

```json
{
  "data": {
    "type": "file",
    "id": "9152d568-7e7c-11e6-a377-37cbfb190b4b",
    "attributes": {
      "rev": "1-0e6d5b72",
      "name": "hello.txt",
      "created_at": "2016-09-19T12:38:04Z",
      "updated_at": "2016-09-19T12:38:04Z",
      "tags": [],
      "size": 12,
      "executable": false,
      "class": "document",
      "mime": "text/plain"
    },
    "links": {
      "self": "/files/9152d568-7e7c-11e6-a377-37cbfb190b4b",
      "parent": "/files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81"
    }
  }
}
```

### GET /files/:file-id

Get the file content

### GET /files/:file-id/thumbnail

Get a thumbnail of a file (for an image only).

### DELETE /files/:file-id

Put a file in the trash.


Trash
-----

When a file is deleted, it is first moved to the trash. In the trash, it can
be restored. Or, after some time, it will be removed from the trash and
permanently destroyed.

### GET /files/trash

List the files inside the trash.

**TODO** example

### POST /files/trash/:file-id

Restore the file with the `file-id` identifiant.

### DELETE /files/trash/:file-id

Destroy the file and make it unrecoverable (it will still be available in
backups).

### DELETE /files/trash

Clear out the trash.

[Table of contents](README.md#table-of-contents)

# References of documents in the Virtual File System

## What we want?

Cozy applications can use data from the Data System and files from the Virtual
File System. Of course, sometimes a link between data and files can be useful.
For example, the application can have an album with photos. The album will be a
document in CouchDB (with a title and other fields), but il will also list the
files to use as photos.

A direct way to do that is storing the files IDs in the album document. It's
simple and will work pretty well if the files are manipulated only from this
application. But, files are often accessed from other apps, like cozy-desktop
and cozy-drive. To improve the User eXperience, it should be nice to alert the
user when a file in an album is modified or deleted.

When a file is modified, we can offer the user the choice between keeping the
original version in the album, or using the new modified file. When a file is
moved to the trash, we can alert the user and let him/her restore the file.

Cozy-desktop, cozy-drive, and the other apps can't scan all the documents with
many different doctypes to find all the references to a file to detect such
cases. The goal of this document is to offer a way to do that, and it is called
_References_.

The references of a file are listed in its JSON-API representation in the
`references` field, within the `relationships` object of `data`.

### Example

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
            },
            "referenced_by": {
                "links": {
                    "self": "/files/fce1a6c0-dfc5-11e5-8d1a-1f854d4aaf81/relationships/references"
                },
                "data": [
                    {
                        "type": "io.cozy.playlists",
                        "id": "94375086-e2e2-11e6-81b9-5bc0b9dd4aa4"
                    }
                ]
            }
        },
        "links": {
            "self": "/files/9152d568-7e7c-11e6-a377-37cbfb190b4b"
        }
    }
}
```

## Routes

### POST /files/:file-id/relationships/referenced_by

Add on a file one or more references to documents

#### Request

```http
POST /files/9152d568-7e7c-11e6-a377-37cbfb190b4b/relationships/referenced_by HTTP/1.1
Content-Type: application/vnd.api+json
Accept: application/vnd.api+json
```

```json
{
    "data": [
        {
            "type": "io.cozy.playlists",
            "id": "f2625cc0-e2d6-11e6-a0d5-cfbbfb141af0"
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
    "data": [
        {
            "type": "io.cozy.playlists",
            "id": "94375086-e2e2-11e6-81b9-5bc0b9dd4aa4"
        },
        {
            "type": "io.cozy.playlists",
            "id": "f2625cc0-e2d6-11e6-a0d5-cfbbfb141af0"
        }
    ]
}
```

### DELETE /files/:file-id/relationships/referenced_by

Remove one or more references to documents on a file

#### Request

```http
DELETE /files/9152d568-7e7c-11e6-a377-37cbfb190b4b/relationships/referenced_by HTTP/1.1
Content-Type: application/vnd.api+json
Accept: application/vnd.api+json
```

```json
{
    "data": [
        {
            "type": "io.cozy.playlists",
            "id": "f2625cc0-e2d6-11e6-a0d5-cfbbfb141af0"
        }
    ]
}
```

#### Response

```http
HTTP/1.1 204 No Content
Content-Type: application/vnd.api+json
```

### GET /data/:type/:doc-id/relationships/references

Returns all the files associated to an album or playlist.

Contents is paginated following [jsonapi conventions](jsonapi.md#pagination).
The default limit is 100 entries.

It's also possible to sort the files by their datetime (for photos) with the
`sort` query parameter: `?sort=datetime`.

#### Request

```http
GET /data/io.cozy.playlists/e9308dc2-e2e3-11e6-b685-fb88662613d4/relationships/references HTTP/1.1
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
            "id": "417c4e58-e2e4-11e6-b7dc-2b68ed7b77f4"
        },
        {
            "type": "io.cozy.files",
            "id": "4504f55c-e2e4-11e6-88f2-d77aeecab549"
        },
        {
            "type": "io.cozy.files",
            "id": "4587727a-e2e4-11e6-bfe9-ef1be7df7f26"
        },
        {
            "type": "io.cozy.files",
            "id": "45d591d0-e2e4-11e6-ab9a-ff3b218e31cc"
        }
    ]
}
```

### POST /data/:type/:doc-id/relationships/references

When creating an album or a playlist, it's tedious to add the references to it
for each file individually. This route allows to make it in bulk.

#### Request

```http
POST /data/io.cozy.playlists/e9308dc2-e2e3-11e6-b685-fb88662613d4/relationships/references HTTP/1.1
Content-Type: application/vnd.api+json
Accept: application/vnd.api+json
```

```json
{
    "data": [
        {
            "type": "io.cozy.files",
            "id": "417c4e58-e2e4-11e6-b7dc-2b68ed7b77f4"
        },
        {
            "type": "io.cozy.files",
            "id": "4504f55c-e2e4-11e6-88f2-d77aeecab549"
        },
        {
            "type": "io.cozy.files",
            "id": "4587727a-e2e4-11e6-bfe9-ef1be7df7f26"
        },
        {
            "type": "io.cozy.files",
            "id": "45d591d0-e2e4-11e6-ab9a-ff3b218e31cc"
        }
    ]
}
```

#### Response

```http
HTTP/1.1 204 No Content
Content-Type: application/vnd.api+json
```

**Note**: if one of the id is a directory, the response will be a 400 Bad
Request. References are only for files.

### DELETE /data/:type/:doc-id/relationships/references

This bulk deletion of references on many files can be useful when an album or
playlist is deleted.

#### Request

```http
DELETE /data/io.cozy.playlists/e9308dc2-e2e3-11e6-b685-fb88662613d4/relationships/references HTTP/1.1
Content-Type: application/vnd.api+json
Accept: application/vnd.api+json
```

```json
{
    "data": [
        {
            "type": "io.cozy.files",
            "id": "417c4e58-e2e4-11e6-b7dc-2b68ed7b77f4"
        },
        {
            "type": "io.cozy.files",
            "id": "4504f55c-e2e4-11e6-88f2-d77aeecab549"
        },
        {
            "type": "io.cozy.files",
            "id": "4587727a-e2e4-11e6-bfe9-ef1be7df7f26"
        },
        {
            "type": "io.cozy.files",
            "id": "45d591d0-e2e4-11e6-ab9a-ff3b218e31cc"
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

### Modification of a referenced file

Before an application updates a file, it can check if the file has some
references. If it is the case, it may offer to the user two choices:

-   update the file with the new version (the albums and playlists will use the
    new version)
-   save the new version as a new file and preserve the old file (the old file
    may be moved to a `originals` directory).

### Moving a referenced file to the trash

Before an application moves a file to the trash, if the file has some
references, it should ask the user if they really want to trash the file. And
it should also removes the references before trashing the file.

## Implementation

The references are persisted in the `io.cozy.files` documents in CouchDB. A
mango index is used to fetch all the files that are associated to a given
document (for `GET /data/:type/:doc-id/relationships/references`).

For request to update or move to trash a file, it is easy to fetch its CouchDB
document to see if it has a reference. But it is more difficult when moving a
folder to trash. To do that, we need two requests to fetch the number of
references in a folder.

1/ Get all descendant folders from a given folder, with a CouchDB View:

```js
map = function(doc) {
    if (doc.type === "folder") emit(doc.path);
};
query = {
    starkey: parent_folder_path + "/",
    endkey: parent_folder_path + "/\uFFFF"
};
```

2/ Get the total number of "referenced" file for this folders list, with a
map/reduce CouchDB view:

```js
map = function(doc) { if(doc.referenced != null) emit(doc.parentID) }
reduce = count
query = {keys: [list from above]}
```

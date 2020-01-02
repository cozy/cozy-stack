[Table of contents](README.md#table-of-contents)

# Notes for collaborative edition

The cozy-notes application can be used to take notes, and collaborate on them.
The note is persisted as a file in the VFS, but it also has specific routes to
enable the collaborative edition in real-time.

## Routes

### POST /notes

It creates a note: it creates a files with the right metadata for collaborative edition.

**Note:** a permission on `POST io.cozy.files` is required to use this route.

#### Parameter

| Parameter | Description                                                               |
| --------- | ------------------------------------------------------------------------- |
| title     | The title of the note, that will also be used for the filename            |
| dir_id    | The identifier of the directory where the file will be created (optional) |
| schema    | The schema for prosemirror (with OrderedMap transformed as arrays)        |

**Note:** if the `dir_id` is not given, the file will be created in a `Notes`
directory (and this directory will have a referenced_by on the notes apps to
allow to find this directory even if it is renamed or moved later).

#### Request

```http
POST /notes HTTP/1.1
Host: alice.example.net
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.notes.documents",
    "attributes": {
      "title": "My new note",
      "dir_id": "f48d9370-e1ec-0137-8547-543d7eb8149c",
      "schema": {
        "nodes": [
          ["doc", { "content": "block+" }],
          ["paragraph", { "content": "inline*", "group": "block" }],
          ["blockquote", { "content": "block+", "group": "block" }],
          ["horizontal_rule", { "group": "block" }],
          [
            "heading",
            {
              "content": "inline*",
              "group": "block",
              "attrs": { "level": { "default": 1 } }
            }
          ],
          ["code_block", { "content": "text*", "marks": "", "group": "block" }],
          ["text", { "group": "inline" }],
          [
            "image",
            {
              "group": "inline",
              "inline": true,
              "attrs": { "alt": {}, "src": {}, "title": {} }
            }
          ],
          ["hard_break", { "group": "inline", "inline": true }],
          [
            "ordered_list",
            {
              "content": "list_item+",
              "group": "block",
              "attrs": { "order": { "default": 1 } }
            }
          ],
          ["bullet_list", { "content": "list_item+", "group": "block" }],
          ["list_item", { "content": "paragraph block*" }]
        ],
        "marks": [
          ["link", { "attrs": { "href": {}, "title": {} }, "inclusive": false }],
          ["em", {}],
          ["strong", {}],
          ["code", {}]
        ],
        "topNode": "doc"
      }
    }
  }
}
```

#### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.files",
    "id": "bf0dbdb0-e1ed-0137-8548-543d7eb8149c",
    "meta": {
      "rev": "1-f71ee54e2"
    },
    "attributes": {
      "type": "file",
      "name": "My new note.cozy-note",
      "trashed": false,
      "md5sum": "NjhiMzI5ZGE5ODkzZTM0MDk5YzdkOGFkNWNiOWM5NDAgIC0K",
      "created_at": "2019-11-05T12:38:04Z",
      "updated_at": "2019-11-05T12:38:04Z",
      "tags": [],
      "metadata": {
        "title": "My new note",
        "content": { "type": "doc", "content": [{ "type": "paragraph" }] },
        "version": 0,
        "schema": {
          "nodes": [
            ["doc", { "content": "block+" }],
            ["paragraph", { "content": "inline*", "group": "block" }],
            ["blockquote", { "content": "block+", "group": "block" }],
            ["horizontal_rule", { "group": "block" }],
            [
              "heading",
              {
                "content": "inline*",
                "group": "block",
                "attrs": { "level": { "default": 1 } }
              }
            ],
            ["code_block", { "content": "text*", "marks": "", "group": "block" }],
            ["text", { "group": "inline" }],
            [
              "image",
              {
                "group": "inline",
                "inline": true,
                "attrs": { "alt": {}, "src": {}, "title": {} }
              }
            ],
            ["hard_break", { "group": "inline", "inline": true }],
            [
              "ordered_list",
              {
                "content": "list_item+",
                "group": "block",
                "attrs": { "order": { "default": 1 } }
              }
            ],
            ["bullet_list", { "content": "list_item+", "group": "block" }],
            ["list_item", { "content": "paragraph block*" }]
          ],
          "marks": [
            ["link", { "attrs": { "href": {}, "title": {} }, "inclusive": false }],
            ["em", {}],
            ["strong", {}],
            ["code", {}]
          ],
          "topNode": "doc"
        }
      },
      "size": 1,
      "executable": false,
      "class": "text",
      "mime": "text/vnd.cozy.note+markdown",
      "cozyMetadata": {
        "doctypeVersion": "1",
        "metadataVersion": 1,
        "createdAt": "2019-11-05T12:38:04Z",
        "createdOn": "https://alice.example.net/",
        "updatedAt": "2019-11-05T12:38:04Z",
        "uploadedAt": "2019-11-05T12:38:04Z",
        "uploadedOn": "https://alice.example.net/"
      }
    },
    "relationships": {
      "parent": {
        "links": {
          "related": "/files/f48d9370-e1ec-0137-8547-543d7eb8149c"
        },
        "data": {
          "type": "io.cozy.files",
          "id": "f48d9370-e1ec-0137-8547-543d7eb8149c"
        }
      }
    }
  }
}
```

### GET /notes

It returns the list of notes, sorted by last update. It adds the path for the
files in the response, as it can be convient for the notes application.

**Note:** a permission on `GET io.cozy.files` is required to use this route.

#### Request

```http
GET /notes HTTP/1.1
Host: alice.example.net
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
    "next": "/notes?page[cursor]=a078d6f0-04a9-0138-3e03-543d7eb8149c"
  },
  "data": [{
    "type": "io.cozy.files",
    "id": "bf0dbdb0-e1ed-0137-8548-543d7eb8149c",
    "meta": {
      "rev": "1-f71ee54e2"
    },
    "attributes": {
      "type": "file",
      "name": "My new note.cozy-note",
      "path": "/Notes/my new note.cozy-note",
      "trashed": false,
      "md5sum": "NjhiMzI5ZGE5ODkzZTM0MDk5YzdkOGFkNWNiOWM5NDAgIC0K",
      "created_at": "2019-11-05T12:38:04Z",
      "updated_at": "2019-11-05T12:38:04Z",
      "tags": [],
      "metadata": {
        "title": "My new note",
        "content": { "type": "doc", "content": [{ "type": "paragraph" }] },
        "version": 0,
        "schema": {
          "nodes": [
            ["doc", { "content": "block+" }],
            ["paragraph", { "content": "inline*", "group": "block" }],
            ["blockquote", { "content": "block+", "group": "block" }],
            ["horizontal_rule", { "group": "block" }],
            [
              "heading",
              {
                "content": "inline*",
                "group": "block",
                "attrs": { "level": { "default": 1 } }
              }
            ],
            ["code_block", { "content": "text*", "marks": "", "group": "block" }],
            ["text", { "group": "inline" }],
            [
              "image",
              {
                "group": "inline",
                "inline": true,
                "attrs": { "alt": {}, "src": {}, "title": {} }
              }
            ],
            ["hard_break", { "group": "inline", "inline": true }],
            [
              "ordered_list",
              {
                "content": "list_item+",
                "group": "block",
                "attrs": { "order": { "default": 1 } }
              }
            ],
            ["bullet_list", { "content": "list_item+", "group": "block" }],
            ["list_item", { "content": "paragraph block*" }]
          ],
          "marks": [
            ["link", { "attrs": { "href": {}, "title": {} }, "inclusive": false }],
            ["em", {}],
            ["strong", {}],
            ["code", {}]
          ],
          "topNode": "doc"
        }
      },
      "size": 1,
      "executable": false,
      "class": "text",
      "mime": "text/vnd.cozy.note+markdown",
      "cozyMetadata": {
        "doctypeVersion": "1",
        "metadataVersion": 1,
        "createdAt": "2019-11-05T12:38:04Z",
        "createdOn": "https://alice.example.net/",
        "updatedAt": "2019-11-05T12:38:04Z",
        "uploadedAt": "2019-11-05T12:38:04Z",
        "uploadedOn": "https://alice.example.net/"
      }
    },
    "relationships": {
      "parent": {
        "links": {
          "related": "/files/f48d9370-e1ec-0137-8547-543d7eb8149c"
        },
        "data": {
          "type": "io.cozy.files",
          "id": "f48d9370-e1ec-0137-8547-543d7eb8149c"
        }
      }
    }
  }]
}
```


### GET /notes/:id

It fetches the file with the given id. It also includes the changes in the
content that have been accepted by the stack but not yet persisted to the file.

#### Request

```http
GET /notes/bf0dbdb0-e1ed-0137-8548-543d7eb8149c HTTP/1.1
Host: alice.example.net
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
    "id": "bf0dbdb0-e1ed-0137-8548-543d7eb8149c",
    "meta": {
      "rev": "4-1482b88a"
    },
    "attributes": {
      "type": "file",
      "name": "My new note.cozy-note",
      "trashed": false,
      "md5sum": "NjhiMzI5ZGE5ODkzZTM0MDk5YzdkOGFkNWNiOWM5NDAgIC0K",
      "created_at": "2019-11-05T12:38:04Z",
      "updated_at": "2019-11-05T12:38:52Z",
      "tags": [],
      "metadata": {
        "title": "My new note",
        "content": { "type": "doc", "content": [{ "type": "horizontal_rule" }] },
        "version": 3,
        "schema": {
          "nodes": [
            ["doc", { "content": "block+" }],
            ["paragraph", { "content": "inline*", "group": "block" }],
            ["blockquote", { "content": "block+", "group": "block" }],
            ["horizontal_rule", { "group": "block" }],
            [
              "heading",
              {
                "content": "inline*",
                "group": "block",
                "attrs": { "level": { "default": 1 } }
              }
            ],
            ["code_block", { "content": "text*", "marks": "", "group": "block" }],
            ["text", { "group": "inline" }],
            [
              "image",
              {
                "group": "inline",
                "inline": true,
                "attrs": { "alt": {}, "src": {}, "title": {} }
              }
            ],
            ["hard_break", { "group": "inline", "inline": true }],
            [
              "ordered_list",
              {
                "content": "list_item+",
                "group": "block",
                "attrs": { "order": { "default": 1 } }
              }
            ],
            ["bullet_list", { "content": "list_item+", "group": "block" }],
            ["list_item", { "content": "paragraph block*" }]
          ],
          "marks": [
            ["link", { "attrs": { "href": {}, "title": {} }, "inclusive": false }],
            ["em", {}],
            ["strong", {}],
            ["code", {}]
          ],
          "topNode": "doc"
        }
      },
      "size": 4,
      "executable": false,
      "class": "text",
      "mime": "text/vnd.cozy.note+markdown",
      "cozyMetadata": {
        "doctypeVersion": "1",
        "metadataVersion": 1,
        "createdAt": "2019-11-05T12:38:04Z",
        "createdOn": "https://alice.example.net/",
        "updatedAt": "2019-11-05T12:38:04Z",
        "uploadedAt": "2019-11-05T12:38:04Z",
        "uploadedOn": "https://alice.example.net/"
      }
    },
    "relationships": {
      "parent": {
        "links": {
          "related": "/files/f48d9370-e1ec-0137-8547-543d7eb8149c"
        },
        "data": {
          "type": "io.cozy.files",
          "id": "f48d9370-e1ec-0137-8547-543d7eb8149c"
        }
      }
    }
  }
}
```

### GET /notes/:id/steps?Version=xxx

It returns the steps since the given version. If the revision is too old, and
the steps are no longer available, it returns a 412 response with the whole
document for the note.

#### Request

```http
GET /notes/bf0dbdb0-e1ed-0137-8548-543d7eb8149c/steps?Version=3 HTTP/1.1
Host: alice.example.net
Accept: application/vnd.api+json
```

#### Response (success)

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": [{
    "type": "io.cozy.notes.steps",
    "attributes": {
      "sessionID": "543781490137",
      "stepType": "replace",
      "from": 1,
      "to": 1,
      "slice": {
        "content": [{ "type": "text", "text": "H" }]
      },
      "version": 4
    }
  }, {
    "type": "io.cozy.notes.steps",
    "attributes": {
      "sessionID": "543781490137",
      "stepType": "replace",
      "from": 2,
      "to": 2,
      "slice": {
        "content": [{ "type": "text", "text": "ello" }]
      },
      "version": 5
    }
  }]
}
```

#### Response (failure)

```http
HTTP/1.1 412 Precondition Failed
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.files",
    "id": "bf0dbdb0-e1ed-0137-8548-543d7eb8149c",
    "meta": {
      "rev": "4-1482b88a"
    },
    "attributes": {
      "type": "file",
      "name": "My new note.cozy-note",
      "trashed": false,
      "md5sum": "NjhiMzI5ZGE5ODkzZTM0MDk5YzdkOGFkNWNiOWM5NDAgIC0K",
      "created_at": "2019-11-05T12:38:04Z",
      "updated_at": "2019-11-05T12:38:52Z",
      "tags": [],
      "metadata": {
        "title": "My new note",
        "content": { "type": "doc", "content": [{ "type": "horizontal_rule" }] },
        "version": 6,
        "schema": {
          "nodes": [
            ["doc", { "content": "block+" }],
            ["paragraph", { "content": "inline*", "group": "block" }],
            ["blockquote", { "content": "block+", "group": "block" }],
            ["horizontal_rule", { "group": "block" }],
            [
              "heading",
              {
                "content": "inline*",
                "group": "block",
                "attrs": { "level": { "default": 1 } }
              }
            ],
            ["code_block", { "content": "text*", "marks": "", "group": "block" }],
            ["text", { "group": "inline" }],
            [
              "image",
              {
                "group": "inline",
                "inline": true,
                "attrs": { "alt": {}, "src": {}, "title": {} }
              }
            ],
            ["hard_break", { "group": "inline", "inline": true }],
            [
              "ordered_list",
              {
                "content": "list_item+",
                "group": "block",
                "attrs": { "order": { "default": 1 } }
              }
            ],
            ["bullet_list", { "content": "list_item+", "group": "block" }],
            ["list_item", { "content": "paragraph block*" }]
          ],
          "marks": [
            ["link", { "attrs": { "href": {}, "title": {} }, "inclusive": false }],
            ["em", {}],
            ["strong", {}],
            ["code", {}]
          ],
          "topNode": "doc"
        }
      },
      "size": 4,
      "executable": false,
      "class": "text",
      "mime": "text/vnd.cozy.note+markdown",
      "cozyMetadata": {
        "doctypeVersion": "1",
        "metadataVersion": 1,
        "createdAt": "2019-11-05T12:38:04Z",
        "createdOn": "https://alice.example.net/",
        "updatedAt": "2019-11-05T12:38:04Z",
        "uploadedAt": "2019-11-05T12:38:04Z",
        "uploadedOn": "https://alice.example.net/"
      }
    },
    "relationships": {
      "parent": {
        "links": {
          "related": "/files/f48d9370-e1ec-0137-8547-543d7eb8149c"
        },
        "data": {
          "type": "io.cozy.files",
          "id": "f48d9370-e1ec-0137-8547-543d7eb8149c"
        }
      }
    }
  }
}
```

### PUT /notes/:id/title

It updates the title.

#### Request

```http
PUT /notes/bf0dbdb0-e1ed-0137-8548-543d7eb8149c/title HTTP/1.1
Host: alice.example.net
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.notes.documents",
    "id": "bf0dbdb0-e1ed-0137-8548-543d7eb8149c",
    "attributes": {
      "sessionID": "543781490137",
      "title": "A new title for my note"
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
  "data": {
    "type": "io.cozy.files",
    "id": "bf0dbdb0-e1ed-0137-8548-543d7eb8149c",
    "meta": {
      "rev": "4-1482b88a"
    },
    "attributes": {
      "type": "file",
      "name": "A new title for my note.cozy-note",
      "trashed": false,
      "md5sum": "NjhiMzI5ZGE5ODkzZTM0MDk5YzdkOGFkNWNiOWM5NDAgIC0K",
      "created_at": "2019-11-05T12:38:04Z",
      "updated_at": "2019-11-05T12:39:37Z",
      "tags": [],
      "metadata": {
        "title": "A new title for my note",
        "content": { "type": "doc", "content": [{ "type": "horizontal_rule" }] },
        "version": 3,
        "schema": {
          "nodes": [
            ["doc", { "content": "block+" }],
            ["paragraph", { "content": "inline*", "group": "block" }],
            ["blockquote", { "content": "block+", "group": "block" }],
            ["horizontal_rule", { "group": "block" }],
            [
              "heading",
              {
                "content": "inline*",
                "group": "block",
                "attrs": { "level": { "default": 1 } }
              }
            ],
            ["code_block", { "content": "text*", "marks": "", "group": "block" }],
            ["text", { "group": "inline" }],
            [
              "image",
              {
                "group": "inline",
                "inline": true,
                "attrs": { "alt": {}, "src": {}, "title": {} }
              }
            ],
            ["hard_break", { "group": "inline", "inline": true }],
            [
              "ordered_list",
              {
                "content": "list_item+",
                "group": "block",
                "attrs": { "order": { "default": 1 } }
              }
            ],
            ["bullet_list", { "content": "list_item+", "group": "block" }],
            ["list_item", { "content": "paragraph block*" }]
          ],
          "marks": [
            ["link", { "attrs": { "href": {}, "title": {} }, "inclusive": false }],
            ["em", {}],
            ["strong", {}],
            ["code", {}]
          ],
          "topNode": "doc"
        }
      },
      "size": 4,
      "executable": false,
      "class": "text",
      "mime": "text/vnd.cozy.note+markdown",
      "cozyMetadata": {
        "doctypeVersion": "1",
        "metadataVersion": 1,
        "createdAt": "2019-11-05T12:38:04Z",
        "createdOn": "https://alice.example.net/",
        "updatedAt": "2019-11-05T12:39:37Z",
        "uploadedAt": "2019-11-05T12:38:04Z",
        "uploadedOn": "https://alice.example.net/"
      }
    },
    "relationships": {
      "parent": {
        "links": {
          "related": "/files/f48d9370-e1ec-0137-8547-543d7eb8149c"
        },
        "data": {
          "type": "io.cozy.files",
          "id": "f48d9370-e1ec-0137-8547-543d7eb8149c"
        }
      }
    }
  }
}
```

### PATCH /notes/:id

It sends some steps to apply on the document. The last known version of the
note must be sent in the `If-Match` header to avoid conflicts.

#### Request

```http
PATCH /notes/bf0dbdb0-e1ed-0137-8548-543d7eb8149c HTTP/1.1
Host: alice.example.net
If-Match: 3
Content-Type: application/vnd.api+json
```

```json
{
  "data": [{
    "type": "io.cozy.notes.steps",
    "attributes": {
      "sessionID": "543781490137",
      "stepType": "replace",
      "from": 1,
      "to": 1,
      "slice": {
        "content": [{ "type": "text", "text": "H" }]
      }
    }
  }, {
    "type": "io.cozy.notes.steps",
    "attributes": {
      "sessionID": "543781490137",
      "stepType": "replace",
      "from": 2,
      "to": 2,
      "slice": {
        "content": [{ "type": "text", "text": "ello" }]
      }
    }
  }]
}
```

#### Response (success)

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.files",
    "id": "bf0dbdb0-e1ed-0137-8548-543d7eb8149c",
    "meta": {
      "rev": "4-1482b88a"
    },
    "attributes": {
      "type": "file",
      "name": "A new title for my note.cozy-note",
      "trashed": false,
      "md5sum": "MDlmN2UwMmYxMjkwYmUyMTFkYTcwN2EyNjZmMTUzYjMgIC0K",
      "created_at": "2019-11-05T12:38:04Z",
      "updated_at": "2019-11-05T12:39:37Z",
      "tags": [],
      "metadata": {
        "title": "A new title for my note",
        "content": {
          "type": "doc",
          "content": [{ "type": "paragraph", "text": "Hello" }]
        },
        "version": 5,
        "schema": {
          "nodes": [
            ["doc", { "content": "block+" }],
            ["paragraph", { "content": "inline*", "group": "block" }],
            ["blockquote", { "content": "block+", "group": "block" }],
            ["horizontal_rule", { "group": "block" }],
            [
              "heading",
              {
                "content": "inline*",
                "group": "block",
                "attrs": { "level": { "default": 1 } }
              }
            ],
            ["code_block", { "content": "text*", "marks": "", "group": "block" }],
            ["text", { "group": "inline" }],
            [
              "image",
              {
                "group": "inline",
                "inline": true,
                "attrs": { "alt": {}, "src": {}, "title": {} }
              }
            ],
            ["hard_break", { "group": "inline", "inline": true }],
            [
              "ordered_list",
              {
                "content": "list_item+",
                "group": "block",
                "attrs": { "order": { "default": 1 } }
              }
            ],
            ["bullet_list", { "content": "list_item+", "group": "block" }],
            ["list_item", { "content": "paragraph block*" }]
          ],
          "marks": [
            ["link", { "attrs": { "href": {}, "title": {} }, "inclusive": false }],
            ["em", {}],
            ["strong", {}],
            ["code", {}]
          ],
          "topNode": "doc"
        }
      },
      "size": 6,
      "executable": false,
      "class": "text",
      "mime": "text/vnd.cozy.note+markdown",
      "cozyMetadata": {
        "doctypeVersion": "1",
        "metadataVersion": 1,
        "createdAt": "2019-11-05T12:38:04Z",
        "createdOn": "https://alice.example.net/",
        "updatedAt": "2019-11-05T12:39:37Z",
        "uploadedAt": "2019-11-05T12:38:04Z",
        "uploadedOn": "https://alice.example.net/"
      }
    },
    "relationships": {
      "parent": {
        "links": {
          "related": "/files/f48d9370-e1ec-0137-8547-543d7eb8149c"
        },
        "data": {
          "type": "io.cozy.files",
          "id": "f48d9370-e1ec-0137-8547-543d7eb8149c"
        }
      }
    }
  }
}
```

#### Response (failure)

If at least one step can't be applied, they will all be discarded, and the
response will be this error:

```http
HTTP/1.1 409 Conflict
Content-Type: application/vnd.api+json
```

```json
{
  "status": 409,
  "Title": "Conflict",
  "Detail": "Cannot apply the steps"
}
```

### PUT /notes/:id/telepointer

It updates the position of the pointer.

#### Request

```http
PUT /notes/f48d9370-e1ec-0137-8547-543d7eb8149c/telepointer HTTP/1.1
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.notes.telepointers",
    "id": "f48d9370-e1ec-0137-8547-543d7eb8149c",
    "attributes": {
      "sessionID": "543781490137",
      "anchor": 7,
      "head": 12,
      "type": "textSelection"
    }
  }
}
```

#### Response

```http
HTTP/1.1 204 No Content
```

### POST /notes/:id/sync

It forces writing the note to the virtual file system. It may be used after the
title has been changed, or when the user quits the note.

#### Request

```http
POST /notes/f48d9370-e1ec-0137-8547-543d7eb8149c/sync HTTP/1.1
```

#### Response

```http
HTTP/1.1 204 No Content
```

## Real-time via websockets

You can subscribe to the [realtime](realtime.md) API for a document with the
`io.cozy.notes.events` doctype, and the id of a note file. It requires a permission
on this file, and it will send the events for this notes: changes of the title, the
steps applied, and the telepointer updates.

### Example

```
client > {"method": "AUTH",
          "payload": "xxAppOrAuthTokenxx="}
client > {"method": "SUBSCRIBE",
          "payload": {"type": "io.cozy.notes.events",
                      "id": "f48d9370-e1ec-0137-8547-543d7eb8149c"}}
server > {"event": "UPDATED",
          "payload": {"id": "f48d9370-e1ec-0137-8547-543d7eb8149c",
                      "type": "io.cozy.notes.events",
                      "doc": {"doctype": "io.cozy.notes.documents",
                              "sessionID": "543781490137",
                              "title": "this is the new title of this note"}}}
server > {"event": "CREATED",
          "payload": {"id": "f48d9370-e1ec-0137-8547-543d7eb8149c",
                      "type": "io.cozy.notes.events",
                      "doc": {"doctype": "io.cozy.notes.steps",
                              "sessionID": "543781490137",
                              "version": 6,
                              "stepType": "replace",
                              "from": 1,
                              "to": 1,
                              "slice": {"content": [{"type": "text", "text": "H"}]}}}}
server > {"event": "UPDATED",
          "payload": {"id": "f48d9370-e1ec-0137-8547-543d7eb8149c",
                      "type": "io.cozy.notes.events",
                      "doc": {"doctype": "io.cozy.notes.telepointers", "sessionID": "543781490137", "anchor": 7, "head": 12, "type": "textSelection"}}}
```

[Table of contents](README.md#table-of-contents)

# Notes for collaborative edition

The cozy-notes application can be used to take notes, and collaborate on them.
The note is persisted as a file in the VFS, but it also has specific routes to
enable the collaborative edition in real-time.

## Routes

### POST /notes

It creates a note: it creates a files with the right metadata for collaborative edition.

#### Parameter

| Parameter | Description                                                               |
| --------- | ------------------------------------------------------------------------- |
| title     | The title of the note, that will also be used for the filename            |
| dir_id    | The identifier of the directory where the file will be created (optional) |
| schema    | The schema for prosemirror                                                |

#### Request

```http
POST /notes HTTP/1.1
Host: alice.example.net
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.notes",
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
      "rev": "4-1482b88a"
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
        "content": {},
        "revision": 0,
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
      "mime": "text/markdown",
      "cozyMetadata": {
        "doctypeVersion": "1",
        "metadataVersion": 1,
        "createdAt": "2019-11-05T12:38:04Z",
        "createdByApp": "note",
        "createdOn": "https://alice.example.net/",
        "updatedAt": "2019-11-05T12:38:04Z",
        "uploadedAt": "2019-11-05T12:38:04Z",
        "uploadedOn": "https://alice.example.net/",
        "uploadedBy": {
          "slug": "note"
        }
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

### GET /notes/:id

It fetches the information about a note.

### GET /notes/:id/steps?revision=xxx

It returns the steps since the given revision.

### PUT /notes/:id/title

It updates the title.

### POST /notes/:id/steps

It sends some steps to apply on the document.

### PUT /notes/:id/telepointer

It updates the position of the pointer.

## Real-time via websockets

TODO:

- title
- steps
- telepointers

# General

### Typing

The notion of document type does not exist in couchdb.
Cozy-stack introduce this notion with a special `_type` field. This type cannot contain "/", it should be unique among all developers, it is recommend to use the Java naming convention with a domain you own.
All CozyCloud types will be prefixed by io.cozy

------------------------------------------------------------------------------

# Access a document

#### Request
```http
GET /data/:type/:id
```
```http
GET /data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee
```

#### Response OK
```http
HTTP/1.1 200 OK
Date: Mon, 27 Sept 2016 12:28:53 GMT
Content-Length: ...
Content-Type: application/json
Etag: "3-6494e0ac6494e0ac"
```
```json
{
    "_id": "io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "_rev": "3-6494e0ac6494e0ac",
    "_type": "io.cozy.events",
    "startdate": "20160823T150000Z",
    "enddate": "20160923T160000Z",
    "summary": "A long month",
    "description": "I could go on and on and on ....",
}
```

### Response Error
```http
HTTP/1.1 404 Not Found
Content-Length: ...
Content-Type: application/json
```
```json
{
  "status": 404,
  "error": "not_found",
  "reason": "deleted",
  "title": "Event deleted",
  "details": "Event 6494e0ac-dfcb-11e5-88c1-472e84a9cbee was deleted",
  "links": {"about": "https://cozy.github.io/cozy-stack/errors.md#deleted"}
}
```

### possible errors :
- 403 unauthenticated
- 401 unauthorized
- 404 not_found
  - reason: missing
  - reason: deleted
- 500 unkown

--------------------------------------------------------------------------------

# Create a document

### Request
```http
POST /data/:type/
```
```http
POST /data/io.cozy.events/
Content-Length: ...
Content-Type: application/json
Accept: application/json
```
```json
{
    "_type": "io.cozy.events",
    "startdate": "20160712T150000",
    "enddate": "20160712T150000",
}
```

### Response OK
```http
201 Created
Content-Length: ...
Content-Type: application/json
```
```json
{
  "id": "io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
  "ok": true,
  "rev": "1-6494e0ac6494e0ac",
  "data": {
    "_id": "io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "_rev": "1-6494e0ac6494e0ac",
    "_type": "io.cozy.events",
    "startdate": "20160712T150000",
    "enddate": "20160712T150000"
  }
}
```

### possible errors :
- 403 unauthenticated
- 401 unauthorized
- 500 unkown

### Details

- A doc cannot contain an `_id` field, if so an error 400 is returned
- A doc cannot contain a `_type` different from the URL one, if so an error 400 is returned
- A doc cannot contain any field starting with `_`, those are reserved for future cozy & couchdb api evolution


--------------------------------------------------------------------------------

# Delete a document

### Request
```http
DELETE /data/:type/:id
```
```http
DELETE /data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee
Accept: application/json

```

### Response OK
```http
200 OK
Content-Length: ...
Content-Type: application/json
```
```json
{
    "id": "io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "ok": true,
    "rev": "2-056f5f44046ecafc08a2bc2b9c229e20",
    "_deleted": true
}
```
### Possible errors :
- 400 bad request
- 403 unauthenticated
- 401 unauthorized
- 404 not_found
  - reason: missing
  - reason: deleted
- 500 unkown

### Conflict prevention

It is possible to use either a `rev` query string parameter or a HTTP `If-Match` header to prevent conflict on deletion:
- If both are passed and are different, an error 400 is returned
- If only one is passed or they are equals, the document will only be deleted if its `_rev` match the passed one. Otherwise, an error 412  is returned.
- If none is passed, the document will be force-deleted.

**Why shouldn't you force delete** (contrieved example) the user is syncing contacts from his mobile, a contact is created with name but no number, the user see it in the contact app. In parallel, the number is added by sync and the user click "delete" because a contact with no number is useless. The decision to delete is based on outdated data state and should therefore be aborted.
Couchdb will prevent this, the stack API allow it for fast prototyping but it should be avoided for serious applications.

### Binary attachments

When a document is deleted and it was the last reference to a binary, said binary is deleted as well.

If you are moving binary from one document to another, you will need to create the new document with binary link first, and only afterward delete the previous document.

### Details

- If no id is provided in URL, an error 400 is returned
-

# General

### Typing

The notion of document type does not exist in Couchdb.

Cozy-stack introduce this notion through a special `_type` field.

This type name cannot contain `/`, and it should be unique among all developers, it is recommended to use the Java naming convention with a domain you own.

All CozyCloud types will be prefixed by io.cozy and be pluralized.
Example : `/data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee`
Where, `io.cozy.` is the developer specific prefix, `events` the actual type, and `6494e0ac-dfcb-11e5-88c1-472e84a9cbee` the document's unique id .

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
    "_id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "_type": "io.cozy.events",
    "_rev": "3-6494e0ac6494e0ac",
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
- 401 unauthorized (no authentication has been provided)
- 403 forbidden (the authentication does not provide permissions for this action)
- 404 not_found
  - reason: missing
  - reason: deleted
- 500 internal server error

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
  "id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
  "type": "io.cozy.events",
  "ok": true,
  "rev": "1-6494e0ac6494e0ac",
  "data": {
    "_id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "_type": "io.cozy.events",
    "_rev": "1-6494e0ac6494e0ac",
    "startdate": "20160712T150000",
    "enddate": "20160712T150000"
  }
}
```

### possible errors :
- 400 bad request
- 401 unauthorized (no authentication has been provided)
- 403 forbidden (the authentication does not provide permissions for this action)
- 500 internal server error

### Details

- A doc cannot contain an `_id` field, if so an error 400 is returned
- A doc cannot contain any field starting with `_`, those are reserved for future cozy & couchdb api evolution


--------------------------------------------------------------------------------

# Update an existing document

### Request
```http
PUT /data/:type/:id
```
```http
PUT /data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee
Content-Length: ...
Content-Type: application/json
Accept: application/json
```
```json
{
    "_id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "_type": "io.cozy.events",
    "_rev": "1-6494e0ac6494e0ac",
    "startdate": "20160712T150000",
    "enddate": "20160712T200000",
}
```

### Response OK
```http
200 OK
Content-Length: ...
Content-Type: application/json
```
```json
{
    "id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "type": "io.cozy.events",
    "ok": true,
    "rev": "2-056f5f44046ecafc08a2bc2b9c229e20",
    "data": {
        "_id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
        "_type": "io.cozy.events",
        "_rev": "2-056f5f44046ecafc08a2bc2b9c229e20",
        "startdate": "20160712T150000",
        "enddate": "20160712T200000",
    }
}
```

### Possible errors :
- 400 bad request
- 401 unauthorized (no authentication has been provided)
- 403 forbidden (the authentication does not provide permissions for this action)
- 404 not_found
  - reason: missing
  - reason: deleted
- 409 Conflict (see Conflict prevention section below)
- 500 internal server error

### Conflict prevention

The client MUST give a `_rev` field in the document. If this field is different from the one in the current version of the document, an error 409 Conflict will be returned.

### Details

- If no id is provided in URL, an error 400 is returned
- If the id provided in URL is not the same than the one in document, an error 400 is returned.

--------------------------------------------------------------------------


# Create a document with a fixed id

### Request
```http
PUT /data/:type/:id
```
```http
PUT /data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee
Content-Length: ...
Content-Type: application/json
Accept: application/json
```
```json
{
    "startdate": "20160712T150000",
    "enddate": "20160712T200000",
}
```

### Response OK
```http
200 OK
Content-Length: ...
Content-Type: application/json
```
```json
{
    "id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "type": "io.cozy.events",
    "ok": true,
    "rev": "1-056f5f44046ecafc08a2bc2b9c229e20",
    "data": {
        "_id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
        "_type": "io.cozy.events",
        "_rev": "1-056f5f44046ecafc08a2bc2b9c229e20",
        "startdate": "20160712T150000",
        "enddate": "20160712T200000",
    }
}
```

### Possible errors :
- 400 bad request
- 401 unauthorized (no authentication has been provided)
- 403 forbidden (the authentication does not provide permissions for this action)
- 404 not_found
  - reason: missing
  - reason: deleted
- 409 Conflict (see Conflict prevention section below)
- 500 internal server error

### Details

- No id should be provide in the document itself

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
    "id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "type": "io.cozy.events",
    "ok": true,
    "rev": "2-056f5f44046ecafc08a2bc2b9c229e20",
    "_deleted": true
}
```
### Possible errors :
- 400 bad request
- 401 unauthorized (no authentication has been provided)
- 403 forbidden (the authentication does not provide permissions for this action)
- 404 not_found
  - reason: missing
  - reason: deleted
- 409 Conflict (see Conflict prevention section below)
- 500 internal server error

### Conflict prevention

It is possible to use either a `rev` query string parameter or a HTTP `If-Match` header to prevent conflict on deletion:
- If none is passed or they are different, an error 400 is returned
- If only one is passed or they are equals, the document will only be deleted if its `_rev` match the passed one. Otherwise, an error 409  is returned.

### Details

- If no id is provided in URL, an error 400 is returned

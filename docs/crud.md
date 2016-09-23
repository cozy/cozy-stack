# Access a document

#### Request
```http
GET /data/:type/:id
```
```http
GET /data/types.cozy.io/events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee
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
    "_id": "types.cozy.io/events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "_rev": "3-6494e0ac6494e0ac",
    "_type": "types.cozy.io/events",
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
POST /data/types.cozy.io/events/
Content-Length: ...
Content-Type: application/json
Accept: application/json
```
```json
{
    "_type": "types.cozy.io/events",
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
  "id": "types.cozy.io/events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
  "ok": true,
  "rev": "1-6494e0ac6494e0ac",
  "data": {
    "_id": "types.cozy.io/events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
    "_rev": "1-6494e0ac6494e0ac",
    "_type": "types.cozy.io/events",
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

[Table of contents](README.md#table-of-contents)

# Data System

## Typing

The notion of document type does not exist in Couchdb.

Cozy-stack introduce this notion through a special `_type` field.

This type name cannot contain `/`, and it should be unique among all developers,
it is recommended to use the Java naming convention with a domain you own.

All CozyCloud types will be prefixed by io.cozy and be pluralized. Example :
`/data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee` Where, `io.cozy.` is
the developer specific prefix, `events` the actual type, and
`6494e0ac-dfcb-11e5-88c1-472e84a9cbee` the document's unique id .

## Access a document

### Request

```http
GET /data/:type/:id HTTP/1.1
```

```http
GET /data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee HTTP/1.1
```

### Response OK

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
  "description": "I could go on and on and on ...."
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
  "links": { "about": "https://cozy.github.io/cozy-stack/errors.md#deleted" }
}
```

### possible errors :

* 401 unauthorized (no authentication has been provided)
* 403 forbidden (the authentication does not provide permissions for this
  action)
* 404 not_found
  * reason: missing
  * reason: deleted
* 500 internal server error

## Access multiple documents at once

### Request

```http
POST /data/:type/_all_docs HTTP/1.1
```

```http
POST /data/io.cozy.files/_all_docs?include_docs=true HTTP/1.1
Content-Length: ...
Content-Type: application/json
Accept: application/json
```

```json
{
  "keys": [
    "7f46ed4ed2a775494da3b0b44e00314f",
    "7f46ed4ed2a775494da3b0b44e003b18"
  ]
}
```

### Response OK

```http
HTTP/1.1 200 OK
Date: Mon, 27 Sept 2016 12:28:53 GMT
Content-Length: ...
Content-Type: application/json
Etag: "3-6494e0ac6494e0ac"
```

```json
{
  "total_rows": 11,
  "rows": [
    {
      "id": "7f46ed4ed2a775494da3b0b44e00314f",
      "key": "7f46ed4ed2a775494da3b0b44e00314f",
      "value": {
        "rev": "1-870e58f8a1b2130c3a41e767f9c7d93a"
      },
      "doc": {
        "_id": "7f46ed4ed2a775494da3b0b44e00314f",
        "_rev": "1-870e58f8a1b2130c3a41e767f9c7d93a",
        "type": "directory",
        "name": "Uploaded from Cozy Photos",
        "dir_id": "7f46ed4ed2a775494da3b0b44e0027df",
        "created_at": "2017-07-04T06:49:12.844631837Z",
        "updated_at": "2017-07-04T06:49:12.844631837Z",
        "tags": [],
        "path": "/Photos/Uploaded from Cozy Photos"
      }
    },
    {
      "key": "7f46ed4ed2a775494da3b0b44e003b18",
      "error": "not_found"
    }
  ]
}
```

### possible errors :

* 401 unauthorized (no authentication has been provided)
* 403 forbidden (the authentication does not provide permissions for this
  action)
* 500 internal server error

### Details

When some keys don't match an existing document, the response still has a status
200 and the errors are included in the `rows` field (see above, same behavior as
[CouchDB](http://docs.couchdb.org/en/stable/api/database/bulk-api.html#post--db-_all_docs)).

## Create a document

### Request

```http
POST /data/:type/ HTTP/1.1
```

```http
POST /data/io.cozy.events/ HTTP/1.1
Content-Length: ...
Content-Type: application/json
Accept: application/json
```

```json
{
  "startdate": "20160712T150000",
  "enddate": "20160712T150000"
}
```

### Response OK

```http
HTTP/1.1 201 Created
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

* 400 bad request
* 401 unauthorized (no authentication has been provided)
* 403 forbidden (the authentication does not provide permissions for this
  action)
* 500 internal server error

### Details

* A doc cannot contain an `_id` field, if so an error 400 is returned
* A doc cannot contain any field starting with `_`, those are reserved for
  future cozy & couchdb api evolution

## Update an existing document

### Request

```http
PUT /data/:type/:id HTTP/1.1
```

```http
PUT /data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee HTTP/1.1
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
  "enddate": "20160712T200000"
}
```

### Response OK

```http
HTTP/1.1 200 OK
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
    "enddate": "20160712T200000"
  }
}
```

### Possible errors :

* 400 bad request
* 401 unauthorized (no authentication has been provided)
* 403 forbidden (the authentication does not provide permissions for this
  action)
* 404 not_found
  * reason: missing
  * reason: deleted
* 409 Conflict (see Conflict prevention section below)
* 500 internal server error

### Conflict prevention

The client MUST give a `_rev` field in the document. If this field is different
from the one in the current version of the document, an error 409 Conflict will
be returned.

### Details

* If no id is provided in URL, an error 400 is returned
* If the id provided in URL is not the same than the one in document, an error
  400 is returned.

## Create a document with a fixed id

### Request

```http
PUT /data/:type/:id HTTP/1.1
```

```http
PUT /data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee HTTP/1.1
Content-Length: ...
Content-Type: application/json
Accept: application/json
```

```json
{
  "startdate": "20160712T150000",
  "enddate": "20160712T200000"
}
```

### Response OK

```http
HTTP/1.1 200 OK
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
    "enddate": "20160712T200000"
  }
}
```

### Possible errors :

* 400 bad request
* 401 unauthorized (no authentication has been provided)
* 403 forbidden (the authentication does not provide permissions for this
  action)
* 404 not_found
  * reason: missing
  * reason: deleted
* 409 Conflict (see Conflict prevention section below)
* 500 internal server error

### Details

* No id should be provide in the document itself

## Delete a document

### Request

```http
DELETE /data/:type/:id?rev=:rev HTTP/1.1
```

```http
DELETE /data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee?rev=1-82a7144c9ec228c9a851b8a1c1aa225b HTTP/1.1
Accept: application/json
```

### Response OK

```http
HTTP/1.1 200 OK
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

* 400 bad request
* 401 unauthorized (no authentication has been provided)
* 403 forbidden (the authentication does not provide permissions for this
  action)
* 404 not_found
  * reason: missing
  * reason: deleted
* 409 Conflict (see Conflict prevention section below)
* 500 internal server error

### Conflict prevention

It is possible to use either a `rev` query string parameter or a HTTP `If-Match`
header to prevent conflict on deletion:

* If none is passed or they are different, an error 400 is returned
* If only one is passed or they are equals, the document will only be deleted if
  its `_rev` match the passed one. Otherwise, an error 409 is returned.

### Details

* If no id is provided in URL, an error 400 is returned

## List all the documents

### Request

```http
GET /data/io.cozy.events/_all_docs?include_docs=true HTTP/1.1
Accept: application/json
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "offset": 0,
  "rows": [
    {
      "id": "16e458537602f5ef2a710089dffd9453",
      "key": "16e458537602f5ef2a710089dffd9453",
      "value": {
        "rev": "1-967a00dff5e02add41819138abb3284d"
      },
      "doc": {
        "field": "value"
      }
    },
    {
      "id": "f4ca7773ddea715afebc4b4b15d4f0b3",
      "key": "f4ca7773ddea715afebc4b4b15d4f0b3",
      "value": {
        "rev": "2-7051cbe5c8faecd085a3fa619e6e6337"
      },
      "doc": {
        "field": "other-value"
      }
    }
  ],
  "total_rows": 2
}
```

### Details

See
[`_all_docs` in couchdb docs](http://docs.couchdb.org/en/stable/api/database/bulk-api.html#db-all-docs)

## List all the documents (alternative)

The `_all_docs` endpoint sends the design docs in the response. It makes it
hard to use pagination on it. We have added a non-standard `_normal_docs`
endpoint. This new endpoint skip the design docs (and does not count them in
the `total_rows`). It only accepts two parameters in the query string: `skip`
(default: 0) and `limit` (default: 100).

Note that the response format is a bit different, it looks more like a `_find`
response with mango.

### Request

```http
GET /data/io.cozy.events/_normal_docs?skip=200&limit=100 HTTP/1.1
Accept: application/json
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "rows": [
    {
      "_id": "16e458537602f5ef2a710089dffd9453",
      "_rev": "1-967a00dff5e02add41819138abb3284d",
      "field": "value"
    },
    {
      "_id": "f4ca7773ddea715afebc4b4b15d4f0b3",
      "_rev": "2-7051cbe5c8faecd085a3fa619e6e6337",
      "field": "other-value"
    }
  ],
  "total_rows": 202
}
```

## List the known doctypes

### Request

```http
GET /data/_all_doctypes HTTP/1.1
Accept: application/json
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
["io.cozy.files", "io.cozy.jobs", "io.cozy.triggers", "io.cozy.settings"]
```

## Others

* The creation and usage of [Mango indexes](mango.md) is possible.
* CouchDB behaviors are not always straight forward: see [some
  quirks](couchdb-quirks.md) for more details.

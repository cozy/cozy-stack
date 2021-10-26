[Table of contents](README.md#table-of-contents)

# Using the HTTP API of cozy-stack

## HTTP Codes

The developers of cozy-stack are trying to respect the semantics of HTTP, and
the HTTP response code is symbolic of that. The stack uses a large range of
HTTP codes, but the most common ones are:

- `200 OK` for a successful operation, with information in the HTTP body
- `201 Created` for a document successfully created
- `204 No Content` for a success with no relevant information to send (like a successful deletion)
- `400 Bad Request` when the request is incorrect
- `401 Unauthorized` when no authorization has been sent, or the token is expired
- `402 Payment Required` when the instance has been blocked and a [user action is required](./user-action-required.md) (payment, or validating the Terms of Services)
- `403 Forbidden` when the authorization has been sent, is valid, but does not grant access to the resource
- `404 Not Found` when the resource is not found
- `409 Conflict` when there is a conflict on the managed resource
- `410 Gone` when a Cozy instance has been moved to a new address
- `412 Precondition Failed` when a parameter from the HTTP headers or query string is invalid
- `422 Unprocessable entity` when an attribute in the HTTP request body is invalid
- `500 Internal Server Error` when something went wrong on the server (bug, network issue, unavailable database)
- `502 Bad Gateway` when an HTTP service used by the stack is not available (apps registry, OIDC provider)

## JSON-API

### Introduction

Except for the routes in `/data`, which imitate couchdb, most of the stack
exposes a JSON-API interface.

See [JSON-API specification](http://jsonapi.org/format/) for more information.

### Pagination

All routes that return a list are (or will be) paginated.

As recommended for couchdb, we use **cursor-based** pagination.

The default page limit is determined on a by-route basis. The client can require
a different limit using `page[limit]` query parameter. If the client does not
specify a limit, default limit will be used instead.

If there is more docs after the limit, the response will contain a `next` key in
its links section, with a `page[cursor]` set to fetch docs starting after the
last one from current request.

Alternatively, the client can opt in for skip mode by using `page[skip]`. When
using skip, the number given in `page[skip]` is number of element ignored before
returning value. Similarly, the response will contain a next link with a
`page[skip]` set for next page (skip + limit).

#### Example

The `/relationships/references` as a default limit of 100.

```http
GET /data/some-type/some-id/relationships/references HTTP/1.1
```

```json
{
    "data": ["... 100 docs ..."],
    "links": {
        "next": "/data/some-type/some-id/relationships/references?page[limit]=100&page[cursor]=7845122548848454212"
    }
}
```

```http
GET /data/some-type/some-id/relationships/references?page[limit]=10 HTTP/1.1
```

```json
{
    "data": ["... 10 docs ..."],
    "links": {
        "next": "/data/some-type/some-id/relationships/references?page[limit]=10&page[cursor]=5487ba7596"
    }
}
```

```http
GET /data/some-type/some-id/relationships/references?page[limit]=100&page[cursor]=7845122548848454212 HTTP/1.1
```

```json
{
    "data": ["... 20 docs ..."]
}
```

```http
GET /data/some-type/some-id/relationships/references?page[limit]=10&page[skip]=0 HTTP/1.1
```

```json
{
    "data": ["... 10 docs ..."],
    "links": {
        "next": "/data/some-type/some-id/relationships/references?page[limit]=10&page[skip]=10"
    }
}
```

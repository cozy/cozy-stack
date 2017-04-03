[Table of contents](README.md#table-of-contents)

# JSON-API

## Introduction

Except for the routes in `/data`, which imitate couchdb, most of the stack exposes a JSON-API interface.

See [JSON-API specification](http://jsonapi.org/format/) for more information.



## Pagination

All routes that return a list are (or will be) paginated.

As recommended for couchdb, we use **cursor-based** pagination.

The default page limit is determined on a by-route basis. The client can require a different limit using `page[limit]` query parameter. If the client does not specify a limit, default limit will be used instead.

If there is more docs after the limit, the response will contain a `next` key in its links section, with a `page[cursor]` set to fetch docs starting after the last one from current request.

### Example


The `/relationships/references` as a default limit of 100.

```http
GET /data/some-type/some-id/relationships/references
```
```json
{
  "data": [ "... 100 docs ..." ],
  "links": {
    "next": "/data/some-type/some-id/relationships/references?page[limit]=100&page[cursor]=7845122548848454212"
  }
}
```

```http
GET /data/some-type/some-id/relationships/references?page[limit]=10
```
```json
{
  "data": [ "... 10 docs ..." ],
  "links": {
    "next": "/data/some-type/some-id/relationships/references?page[limit]=10&page[cursor]=5487ba7596"
  }
}
```

```http
GET /data/some-type/some-id/relationships/references?page[limit]=100&page[cursor]=7845122548848454212
```
```json
{
  "data": [ "... 20 docs ..." ],
}
```

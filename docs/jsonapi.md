[Table of contents](README.md#table-of-contents)

# JSON-API

## Introduction

Except for the routes in `/data`, which imitates couchdb, most of the stack exposes a JSON-API interface.

See [JSON-API specification](http://jsonapi.org/format/) for more information.



## Pagination

All routes that returns a list are (or will be) paginated.

As recommended for couchdb, we use **cursor-based** pagination.

The default page limit is determined on a by-route basis. The client can require a lower limit using `page[limit]` query parameter. If the client does not specify a limit or if the limit is higher than default, default limit will be used instead.

If there is more docs after the limit, the response will contain a `next` key in its links section, with a `page[cursor]` set to fetch docs starting after the last one from current request.

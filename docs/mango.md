[Table of contents](README.md#table-of-contents)

# Mango

## Create an index for some documents

The body should contain a `index` JSON field containing a `fields` which is an
ordered array of fields to index.

### Request

```http
POST /data/:doctype/_index HTTP/1.1
```

```http
POST /data/io.cozy.events/_index HTTP/1.1
Content-Type: application/json
```

```json
{
    "index": {
        "fields": ["calendar", "date"]
    }
}
```

### Response OK

```http
HTTP/1.1 200 OK
Date: Mon, 27 Sept 2016 12:28:53 GMT
Content-Length: ...
Content-Type: application/json
```

```json
{
    "result": "created",
    "id": "_design/a5f4711fc9448864a13c81dc71e660b524d7410c",
    "name": "a5f4711fc9448864a13c81dc71e660b524d7410c"
}
```

### Details

-   if the doctype does not exist, the database is created.
-   if the index already exists, a `{result: "exists"}` is returned, but the
    response code is still 200
-   design doc & name can be provided in request. **This is not recommended**,
    let couchdb handle naming and deduplication.

```json
{
  "name": "by-calendar-and-date",
  "ddoc": "_design/some-ddoc-name",
  "index": { "fields": ... }
}
```

### possible errors :

-   401 unauthorized (no authentication has been provided)
-   403 forbidden (the authentication does not provide permissions for this
    action)
-   500 internal server error

## Find documents

Find allows to find documents using a mango selector. You can read more about
mango selectors
[here](http://docs.couchdb.org/en/stable/api/database/find.html#selector-syntax)

### Request

```http
POST /data/:doctype/_find HTTP/1.1
```

```http
POST /data/io.cozy.events/_find HTTP/1.1
Content-Type: application/json
```

```json
{
    "selector": {
        "calendar": "perso",
        "date": { "$gt": "20161001T00:00:00" }
    },
    "limit": 2,
    "skip": 3,
    "sort": ["calendar", "date"],
    "fields": ["_id", "_type", "_date"],
    "use_index": "_design/a5f4711fc9448864a13c81dc71e660b524d7410c"
}
```

### Response OK

```http
HTTP/1.1 200 OK
Date: Mon, 27 Sept 2016 12:28:53 GMT
Content-Length: ...
Content-Type: application/json
```

```json
{
    "docs": [
        {
            "_id": "6494e0ac-dfcb-11e5-88c1-472e84a9cbee",
            "_type": "io.cozy.events",
            "date": "20161023T160000Z"
        },
        {
            "_id": "6494e0ac-dfcb-472e84a9cbee",
            "_type": "io.cozy.events",
            "date": "20161013T160000Z"
        }
    ]
}
```

### Details

-   If an index does not exist for the selector, an error 400 is returned
-   The sort field must contains all fields used in selector
-   The sort field must match an existing index
-   It is possible to sort in reverse direction
    `sort:[{"calendar":"desc"}, {"date": "desc"}]` but **all fields** must be
    sorted in same direction.
-   `use_index` is optional but recommended.

## Pagination cookbook

Pagination of mango query should be handled by the client. The stack will limit
query results to a maximum of 100 documents. This limit can be raised up to
1000 documents per page with the `limit` parameter, but not further.

The limit applied to a query is visible in the HTTP response.

If the limit cause some docs to not be returned, the response will have a
`next=true` top level values.

```json
{
    "limit": 100,
    "next": true,
    "docs": ["... first hundred docs ..."]
}
```

If the number of docs is lower or equal to the limit, next will be false

```json
{
    "limit": 100,
    "next": false,
    "docs": ["... less than a hundred docs ..."]
}
```

To paginate, the client should keep track of the value of the last index field.

### Example:

Index on io.cozy.events with fields `["calendar", "date"]`

Try to get all events for a month :

```json
selector: {
  "calendar": "my-calendar",
  "date": { "$gt": "20161001", "$lt": "20161030" }
}
```

If there is less than 100 events, the response `next` field will be false and
there is nothing more to do. If there is more than 100 events for this month, we
have a `next=true` in the response.

To keep iterating, we can take the date from the last item we received in the
results and use it as next request `$gte`

```json
selector: {
  "calendar": "my-calendar",
  "date": { "$gte" :"20161023", "$lt": "20161030" }
}
```

# Mango

# Create an index for some documents

The body should contain a `index` JSON field containing a `fields` which is an ordered array of fields to index.

#### Request
```http
POST /data/:doctype/_index
```
```http
POST /data/io.cozy.events/_index
Content-Type: application/json
```
```json
{
  "index": {
    "fields": ["calendar", "date"]
  }
}
```

#### Response OK
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
- if the doctype does not exist, the database is created.
- if the index already exists, a `{result: "exists"}` is returned, but the response code is still 200
- design doc & name can be provided in request. **This is not recommended**, let couchdb handle naming and deduplication.

```json
{
  "name": "by-calendar-and-date",
  "ddoc": "_design/some-ddoc-name",
  "index": { "fields": ... }
}
```

### possible errors :
- 401 unauthorized (no authentication has been provided)
- 403 forbidden (the authentication does not provide permissions for this action)
- 500 internal server error

# Find documents

Find allows to find documents using a mango selector.
You can read more about mango selectors [here](http://docs.couchdb.org/en/2.0.0/api/database/find.html#selector-syntax)


#### Request
```http
POST /data/:doctype/_find
```
```http
POST /data/io.cozy.events/_find
Content-Type: application/json
```
```json
{
  "selector": {
    "calendar": "perso",
    "date": {"$gt": "20161001T00:00:00"}
  },
  "limit": 2,
  "skip": 3,
  "sort": "date",
  "fields": ["_id", "_type", "_date"]
}
```

#### Response OK
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
          "date": "20161023T160000Z",
      },
      {
          "_id": "6494e0ac-dfcb-472e84a9cbee",
          "_type": "io.cozy.events",
          "date": "20161013T160000Z",
      }
  ]
}
```

### Details
- You can use the `{}` empty selector to get all docs for a db (beware it will also includes `_design/` docs)
- If an index does not exist for the selector, an error 400 is returned

# Swift

## Swift API

This API provides several features to interact with the Swift VFS.

### GET /swift/list-layouts

Count swift layouts by type

#### Request

```http
GET /swift/list-layouts HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "total": 3,
  "unknown": {
    "counter": 0
  },
  "v1": {
    "counter": 1
  },
  "v2a": {
    "counter": 0
  },
  "v2b": {
    "counter": 0
  },
  "v3": {
    "counter": 2
  }
}
```

The `show_domains=true` query parameter provides the domain names if needed


#### Request

```http
GET /swift/list-layouts?show_domains=true HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "total": 3,
  "unknown": {
    "counter": 0
  },
  "v1": {
    "counter": 1,
    "domains": [
      "bob.cozy.tools:8081"
    ]
  },
  "v2a": {
    "counter": 0
  },
  "v2b": {
    "counter": 0
  },
  "v3": {
    "counter": 2,
    "domains": [
      "alice.cozy.tools:8081",
      "ru.cozy.tools:8081"
    ]
  }
}
```

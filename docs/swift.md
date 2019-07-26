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

### GET /swift/get/:domain/:object

Retrieves a Swift object

#### Request

```http
GET /swift/get/alice.cozy.tools/67a88b22520680b1fae840%2F9a8a0%2F18d02%2FiYbkfuCDEMaVoIXg HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
 "content": "foobar"
}
```

### POST /swift/put

Put an object in Swift

Parameters:
- `instance`
- `object_name`
- `content`
- `content_type`

#### Request

```http
POST /swift/put HTTP/1.1
Accept: application/vnd.api+json
```

```json
{
  "instance": "alice.cozy.tools",
  "object_name": "67a88b22520680b1fae840/9a8a0/18d02/iYbkfuCDEMaVoIXg",
  "content": "this is my content",
  "content-type": "text/plain"
}
```

### DELETE /swift/:domain/:object

Removes an object from Swift

#### Request

```http
POST /swift/alice.cozy.tools/67a88b22520680b1fae840%2F9a8a0%2F18d02%2FiYbkfuCDEMaVoIXg HTTP/1.1
Accept: application/vnd.api+json
```

### GET /swift/ls/:domain

List Swift objects of an instance

#### Request

```http
POST /swift/ls/alice.cozy.tools HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "objects_names": [
    "67a88b22520680b1fae840/9a8a0/17264/AxfGhAiWVRhPufKK",
    "67a88b22520680b1fae840/9a8a0/18d02/iYbkfuCDEMaVoIXg",
    "thumbs/67a88b22520680b1fae840/9a8a0/17264-large",
    "thumbs/67a88b22520680b1fae840/9a8a0/17264-medium",
    "thumbs/67a88b22520680b1fae840/9a8a0/17264-small"
  ]
}
```

[Table of contents](README.md#table-of-contents)

# Connection check URI

## Connection-check

This endpoint respond with HTTP 204 No Content and can be used
to test connectivity to cozy instance. It is primarily used by the
flagship mobile application.

### Request

```http
GET /connection_check HTTP/1.1
Host: alice.cozy.example.net
```

### Response

```http
HTTP/1.1 204

```

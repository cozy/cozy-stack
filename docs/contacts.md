[Table of contents](README.md#table-of-contents)

# Contacts

## Routes

### POST /contacts/myself

This endpoint returns the information about the io.cozy.contacts document for
the owner of this instance, the "myself" contact. If the document does not
exist, it is recreated with some basic fields.

A permission on the `io.cozy.contacts` document is required.

#### Request

```http
POST /contacts/myself HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.contacts",
    "id": "bf91cce0-ef48-0137-2638-543d7eb8149c",
    "attributes": {
      "fullname": "Alice",
      "email": [
        { "address": "alice@example.com", "primary": true }
      ]
    },
    "meta": {
      "rev": "1-6516671ec"
    }
  }
}
```

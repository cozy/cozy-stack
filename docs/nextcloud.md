[Table of contents](README.md#table-of-contents)

# Proxy for a remote NextCloud

The nextcloud konnector can be used to create an `io.cozy.account` for a
NextCloud. Then, the stack can be used as a client for this NextCloud account.
Currently, it supports files operations via WebDAV.

## PUT /remote/nextcloud/:account/*path

This route can be used to create a directory on the NextCloud.

The `:account` parameter is the identifier of the NextCloud `io.cozy.account`.
It is available with the `cozyMetadata.sourceAccountIdentifier` of the shortcut
file for example.

The `*path` parameter is the path of the directory on the NextCloud.

**Note:** a permission on `POST io.cozy.files` is required to use this route.

### Request

```http
PUT /remote/nextcloud/4ab2155707bb6613a8b9463daf00381b/Documents/Images/Clouds HTTP/1.1
Host: cozy.example.net
Authorization: Bearer eyJhbG...
```

### Response

```http
HTTP/1.1 201 Created
Content-Type: application/json
```

```json
{
  "ok": true
}
```

#### Status codes

- 201 Created, when the directory has been created
- 400 Bad Request, when the account is not configured for NextCloud
- 401 Unauthorized, when authentication to the NextCloud fails
- 404 Not Found, when the account is not found or the parent directory is not found on the NextCloud
- 409 Conflict, when a directory or file already exists at this path on the NextCloud.

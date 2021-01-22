[Table of contents](README.md#table-of-contents)

# Well-known URIs

## Change-password

This endpoint redirects to the settings page where the user can change their
password.

See https://w3c.github.io/webappsec-change-password-url/

### Request

```http
GET /.well-known/change-password HTTP/1.1
Host: alice.cozy.example.net
```

### Response

```http
HTTP/1.1 302 Found
Location: https://alice-settings.cozy.example.net/#/profile/password
```

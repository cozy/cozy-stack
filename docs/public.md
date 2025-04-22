[Table of contents](README.md#table-of-contents)

# Public routes

These routes are public: no authentication is required for them.

## Avatar

### GET /public/avatar

Returns an image chosen by the user as their avatar. If no image has been
chosen, a fallback will be used, depending of the `fallback` parameter in the
query-string:

- `default` or no `fallback` parameter: a default image that shows the Cozy
  Cloud logo, but it can be overriden by dynamic assets per context (always a png)
- `404`: just a HTTP 404 - Not found error.
- `anonymous`: a generic user icon without initials visible
- `initials`: a generated image with the initials of
  the owner's public name, with the following special arguments:
    - `format=png`: request a PNG response, otherwise defaults to SVG
    - `fx=translucent`: if SVG, make the output partially transparent
    - `as=unconfirmed`: if SVG, make the output grayscale

## Prelogin

### GET /public/prelogin

This route returns information that could be useful to show a login page (like
in the flagship app).

#### Request

```http
GET /public/prelogin HTTP/1.1
Host: cozy.localhost:8080
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "Kdf": 0,
  "KdfIterations": 100000,
  "OIDC": false,
  "FranceConnect": false,
  "locale": "en",
  "magic_link": false,
  "name": "Claude"
}
```

#### Response when the instance has not been onboarded

```http
HTTP/1.1 412 Precondition failed
Content-Type: application/json
```

```json
{
  "error": "the instance has not been onboarded"
}
```

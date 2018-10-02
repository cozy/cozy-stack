[Table of contents](README.md#table-of-contents)

# User action is required

This document explains how the stack can alert applications that an user action
is required to perform the expected services. For example, the stack can block
most of its API until the user read and sign new terms of services.

## HTTP 402 Error

In some cases the stack will return a 402 error to API requests. 402 will be
used to denote error that require an user's action.

For now, the only use case is a Terms Of Services update, but some other use
cases might appear in the future. The specific cause of this error will be
provided within the body of the response (see examples below).

```http
HTTP/1.1 402 Payment Required
Content-Length: ...
Content-Type: application/vnd.api+json
```

```json
{
    "errors": [
        {
            "status": "402",
            "title": "TOS Updated",
            "code": "tos-updated",
            "detail": "Terms of services have been updated",
            "links": {
                "self": "https://manager.cozycloud.cc/cozy/tos?domain=..."
            }
        }
    ]
}
```

If they receive such a code, the clients should block any further action on the
stack, warn the user with the necessary message and provide a button allowing
the user to perform the required action.

-   If the client knows the specific `error` code, display a beautiful message.
-   Otherwise, display the message provided by the stack and use the
    links.action on the button.

Possible other codes in the future: `payment_required` for functions requiring a
premium account, etc.

# Anticipating these errors and warning the user

An enpoints exists to get the list of warnings that the user can anticipate. For
applications these warnings are included directly into the HTML of the index
page, as follow:

```html
<meta name="user-action-required"
  data-title="{{ .Title }}"
  data-code="{{ .Code }}"
  data-detail="{{ .Detail }}"
  data-links="{{ .Links.Self }}"} />
```

## Request

```http
GET /settings/warnings HTTP/1.1
```

## Response

```http
HTTP/1.1 402 Payment Required
Content-Length: ...
Content-Type: application/vnd.api+json
```

```json
{
    "errors": [
        {
            "status": "402",
            "title": "TOS Updated",
            "code": "tos-updated",
            "detail": "Terms of services have been updated",
            "links": {
                "self": "https://manager.cozycloud.cc/cozy/tos?domain=..."
            }
        }
    ]
}
```

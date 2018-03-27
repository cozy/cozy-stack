[Table of contents](README.md#table-of-contents)

# HTTP 402 Error

In some cases the stack will return a 402 error to API requests. 
402 will be used to denote error that require an user's action.

For now, the only use case is a `Condition Générale d'Utilisation` (Term Of Service) update,
but some other use cases might appear in the future. The specific cause of this error will 
be provided within the body of the response (see examples below)

```http
HTTP/1.1 402 Payment Required
Content-Length: ...
Content-Type: application/json

{
  "status": 402,
  "error": "tos_updated",
  "title": "TOS Updated",
  "details": "TOS have been updated",
  "links": { "action": "https://manager.cozycloud.cc/cozy/tos?domain=..." }
}

```

If they receive such a code, the clients should block any further action on the stack, warn the user with 
the necessary message and provide a button allowing the user to perform the required action.
- If the client knows the specific `error` code, display a beautiful message
- Otherwise, display the message provided by the stack and use the links.action on the button.


Possible other codes in the future : `payment_required` for functions requiring a premium account, ...


# Anticipating these errors and warning the user.

```
GET /settings/warnings

[
{
  "error": "tos_updated",
  "title": "TOS Updated",
  "details": "TOS have been updated",
  "links": { "action": "https://manager.cozycloud.cc/cozy/tos?domain=..." }
},
]

```

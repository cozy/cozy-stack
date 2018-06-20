[Table of contents](README.md#table-of-contents)

# Settings

## Disk usage

### GET /settings/disk-usage

Says how many bytes are available and used to store files. When not limited the
`quota` field is omitted.

#### Request

```http
GET /settings/disk-usage HTTP/1.1
Host: alice.example.com
Accept: application/vnd.api+json
Authorization: Bearer ...
```

#### Response

```http
HTTP/1.1 200 OK
Content-type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.settings",
    "id": "io.cozy.settings.disk-usage",
    "attributes": {
      "is_limited": true,
      "quota": "123456789",
      "used": "12345678"
    }
  }
}
```

## Passphrase

### POST /settings/passphrase

The onboarding application can send a request to this endpoint to register the
passphrase of the user. The registration token can only be used once.

#### Request

```http
POST /settings/passphrase HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
  "register_token": "37cddf40d7724988860fa0e03efd30fe",
  "passphrase": "ThisIsTheNewShinnyPassphraseChoosedByAlice"
}
```

#### Response

```http
HTTP/1.1 204 No Content
Set-Cookie: cozysessid=AAAAAFhSXT81MWU0ZTBiMzllMmI1OGUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa; Path=/; Domain=alice.example.com; Max-Age=604800; HttpOnly; Secure
```

### PUT /settings/passphrase (without two-factor authentication)

The user can change its passphrase with this route.

For users with two-factor authentication activated, a second step on the same
route is necessary to actually update the passphrase. See below.

#### Request

```http
PUT /settings/passphrase HTTP/1.1
Host: alice.example.com
Content-Type: application/json
Cookie: cozysessid=AAAAAFhSXT81MWU0ZTBiMzllMmI1OGUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa
```

```json
{
  "current_passphrase": "ThisIsTheNewShinnyPassphraseChoosedByAlice",
  "new_passphrase": "AliceHasChangedHerPassphraseAndThisIsTheNewPassphrase"
}
```

#### Response

```http
HTTP/1.1 204 No Content
Set-Cookie: cozysessid=AAAAShoo3uo1Maic4VibuGohlik2eKUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa; Path=/; Domain=alice.example.com; Max-Age=604800; HttpOnly; Secure
```

### PUT /settings/passphrase (with two-factor authentication)

The user can change its passphrase with this route, with two-factor
authentication to verify the user with more than its passphrase.

#### Request (first step)

```http
PUT /settings/passphrase HTTP/1.1
Host: alice.example.com
Content-Type: application/json
Cookie: cozysessid=AAAAAFhSXT81MWU0ZTBiMzllMmI1OGUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa
```

```json
{
  "current_passphrase": "ThisIsTheNewShinnyPassphraseChoosedByAlice"
}
```

#### Response (first step)

```http
HTTP/1.1 200 OK
Set-Cookie: cozysessid=AAAAShoo3uo1Maic4VibuGohlik2eKUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa; Path=/; Domain=alice.example.com; Max-Age=604800; HttpOnly; Secure
```

```json
{
  "two_factor_token": "YxOSUjxd0SNmuwEEDRHXfw=="
}
```

At this point, the current passphrase has been exchanged against a token, and
a passcode should have been sent to the user to authenticate on the second
step.

The token/passcode pair can be used on the second step to update the
passphrase.

#### Request (second step)

```http
PUT /settings/passphrase HTTP/1.1
Host: alice.example.com
Content-Type: application/json
Cookie: cozysessid=AAAAAFhSXT81MWU0ZTBiMzllMmI1OGUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa
```

```json
{
  "new_passphrase": "AliceHasChangedHerPassphraseAndThisIsTheNewPassphrase",
  "two_factor_token": "YxOSUjxd0SNmuwEEDRHXfw==",
  "two_factor_passcode": "4947178"
}
```

#### Response (second step)

```http
HTTP/1.1 204 No Content
Set-Cookie: cozysessid=AAAAShoo3uo1Maic4VibuGohlik2eKUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa; Path=/; Domain=alice.example.com; Max-Age=604800; HttpOnly; Secure
```

#### Response

```http
HTTP/1.1 204 No Content
```

## Instance

### GET /settings/instance

If the user is logged in, display all instance settings. If the user is not
logged in, the register token can be used to read the informations.

#### Request

```http
GET /settings/instance HTTP/1.1
Host: alice.example.com
Accept: application/vnd.api+json
Cookie: sessionid=xxxx
```

#### Response

```json
{
  "data": {
    "type": "io.cozy.settings",
    "id": "io.cozy.settings.instance",
    "meta": {
      "rev": "3-56521545485448482"
    },
    "attributes": {
      "locale": "fr",
      "auto_update": true,
      "email": "alice@example.com",
      "public_name": "Alice Martin",
      "auth_mode": "basic"
    }
  }
}
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.settings` for the verb `GET`.

### PUT /settings/instance

If the user is logged in, allow to set the instance fields

#### Request

```http
POST /settings/instance HTTP/1.1
Host: alice.example.com
Accept: application/vnd.api+json
Content-type: application/vnd.api+json
Cookie: sessionid=xxxxx
Authorization: Bearer settings-token
```

```json
{
  "data": {
    "type": "io.cozy.settings",
    "id": "io.cozy.settings.instance",
    "meta": {
      "rev": "3-56521545485448482"
    },
    "attributes": {
      "locale": "fr",
      "email": "alice@example.com",
      "public_name": "Alice Martin",
      "timezone": "Europe/Berlin",
      "auth_mode": "two_factor_mail"
    }
  }
}
```

#### Response

```
HTTP/1.1 200 OK
Content-type: application/json
```

```json
{
  "data": {
    "type": "io.cozy.settings",
    "id": "io.cozy.settings.instance",
    "meta": {
      "rev": "4-5a3e315e"
    },
    "attributes": {
      "locale": "fr",
      "email": "alice@example.com",
      "public_name": "Alice Martin",
      "timezone": "Europe/Berlin",
      "auth_mode": "two_factor_mail"
    }
  }
}
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.settings` for the verb `PUT`.

### PUT /settings/instance/auth_mode

With this route, the user can ask for the activation of different
authentication modes, like two-factor authentication.

Available authentication modes:

* `basic`: basic authentication only with passphrase
* `two_factor_mail`: authentication with passphrase and validation with a
  code sent via email to the user.

When asking for activation of the two-factor authentication, a side-effect can
be triggered to send the user its code (via email for instance), and the
activation not being effective. This side-effect should provide the user with
a code that can be used to finalize the activation of the two-factor
authentication.

Hence, this route has two behaviors:

* the code is not provided: the route is a side effect to ask for the
  activation of 2FA, and a code is sent
* the code is provided, and valid: the two-factor authentication is actually
  activated.

Status codes:

* `204 No Content`: when the mail has been confirmed and two-factor authentication is activated
* `422 Unprocessable Entity`: when the given confirmation code is not good.

#### Request

```http
PUT /settings/instance/auth_mode HTTP/1.1
Host: alice.example.com
Content-Type: application/json
Cookie: cozysessid=AAAAAFhSXT81MWU0ZTBiMzllMmI1OGUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa
```

```json
{
  "auth_mode": "two_factor_mail",
  "two_factor_activation_code": "12345678"
}
```

### PUT /settings/instance/sign_tos

With this route, an OAuth client can sign the new TOS version.

Status codes:

* `204 No Content`: when the mail has been confirmed and two-factor authentication is activated

#### Request

```http
PUT /settings/instance/sign_tos HTTP/1.1
Host: alice.example.com
Content-Type: application/json
Authorization: Bearer ...
```

### GET /settings/sessions

This route allows to get all the currently active sessions.

```
GET /settings/sessions HTTP/1.1
Host: cozy.example.org
Cookie: ...
Authorization: Bearer ...
```

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "data": [
    {
      "id": "...",
      "attributes": {
        "last_seen": ""
      },
      "meta": {
        "rev": "..."
      }
    }
  ]
}
```

#### Permissions

This route requires the application to have permissions on the
`io.cozy.sessions` doctype with the `GET` verb.

## OAuth 2 clients

### GET /settings/clients

Get the list of the registered clients

#### Request

```http
GET /settings/clients HTTP/1.1
Host: alice.example.com
Accept: application/vnd.api+json
Cookie: sessionid=xxxxx
Authorization: Bearer oauth2-clients-token
```

#### Response

```http
HTTP/1.1 200 OK
Content-type: application/json
```

```json
{
  "data": [
    {
      "type": "io.cozy.oauth.clients",
      "id": "30e84c10-e6cf-11e6-9bfd-a7106972de51",
      "attributes": {
        "redirect_uris": ["http://localhost:4000/oauth/callback"],
        "client_name": "Cozy-Desktop on my-new-laptop",
        "client_kind": "desktop",
        "client_uri": "https://docs.cozy.io/en/mobile/desktop.html",
        "logo_uri": "https://docs.cozy.io/assets/images/cozy-logo-docs.svg",
        "policy_uri": "https://cozy.io/policy",
        "software_id": "/github.com/cozy-labs/cozy-desktop",
        "software_version": "0.16.0",
        "synchronized_at": "2017-09-05T16:23:04Z"
      },
      "links": {
        "self": "/settings/clients/30e84c10-e6cf-11e6-9bfd-a7106972de51"
      }
    }
  ]
}
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.oauth.clients` for the verb `GET` (only client-side apps).

### DELETE /settings/clients/:client-id

#### Request

```http
DELETE /settings/clients/30e84c10-e6cf-11e6-9bfd-a7106972de51 HTTP/1.1
Host: alice.example.com
Authorization: Bearer oauth2-clients-token
```

#### Response

```http
HTTP/1.1 204 No Content
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.oauth.clients` for the verb `DELETE` (only client-side apps).

### POST /settings/synchronized

Any OAuth2 client can make a request to this endpoint with its token, no
permission is needed. It will update the date of last synchronization for this
device.

#### Request

```http
POST /settings/synchronized HTTP/1.1
Host: alice.example.com
Authorization: Bearer oauth2-access-token
```

#### Response

```http
HTTP/1.1 204 No Content
```

## Context

### GET /settings/onboarded

It redirects the user to an application after the onboarding. The application is
selected according to the context of the instance and the configuration of the
stack.

### GET /settings/context

It gives the keys/values from the config for the context of the instance.

#### Request

```http
GET /settings/context HTTP/1.1
Host: alice.example.com
Accept: application/vnd.api+json
Cookie: sessionid=xxxx
```

#### Response

```json
{
  "data": {
    "type": "io.cozy.settings",
    "id": "io.cozy.settings.context",
    "attributes": {
      "default_redirection": "drive/#/files",
      "help_link": "https://forum.cozy.io/",
      "onboarded_redirection": "collect/#/discovery/?intro"
    },
    "links": {
      "self": "/settings/context"
    }
  }
}
```

#### Permissions

To use this endpoint, an application needs a valid token, but no explicit
permission is required.

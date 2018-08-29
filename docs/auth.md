[Table of contents](README.md#table-of-contents)

# Authentication and access delegations

## Introduction

In this document, we will cover how to protect the usage of the cozy-stack. When
the cozy-stack receives a request, it checks that the request is authorized, and
if yes, it processes it and answers it.

## What about OAuth2?

OAuth2 is about delegating an access to resources on a server to another party.
It is a framework, not a strictly defined protocol, for organizing the
interactions between these 4 actors:

* the resource owner, the "user" that can click on buttons
* the client, the website or application that would like to access the resources
* the authorization server, whose role is limited to give tokens but is central
  in OAuth2 interactions
* the resources server, the server that controls the resources.

For cozy, both the authorization server and the resources server roles are
played by the cozy-stack. The resource owner is the owner of a cozy instance.
The client can be the cozy-desktop app, cozy-mobile, or many other applications.

OAuth2, and its extensions, is a large world. At its core, there is 2 things:
letting the client get a token issued by the authorization server, and using
this token to access to the resources. OAuth2 describe 4 flows, called grant
types, for the first part:

* Authorization code
* Implicit grant type
* Client credentials grant type
* Resource owner credentials grant type.

On cozy, only the most typical one is used: authorization code. To start this
flow, the client must have a `client_id` and `client_secret`. The Cozy stack
implements the OAuth2 Dynamic Client Registration Protocol (an extension to
OAuth2) to allow the clients to obtain them.

OAuth2 has also 3 ways to use a token:

* in the query-string (even if the spec does not recommended it)
* in the POST body
* in the HTTP Authorization header.

On cozy, only the HTTP header is supported.

OAuth2 has a lot of assumptions. Let's see some of them and their consequences
on Cozy:

* TLS is very important to secure the communications. in OAuth 1, there was a
  mechanism to sign the requests. But it was very difficult to get it right for
  the developers and was abandonned in OAuth2, in favor of using TLS. The Cozy
  instance are already accessible only in HTTPS, so there is nothing particular
  to do for that.

* There is a principle called TOFU, Trust On First Use. It said that if the user
  will give his permission for delegating access to its resources when the
  client will try to access them for the first time. Later, the client will be
  able to keep accessing them even if the user is no longer here to give his
  permissions.

* The client can't make the assumptions about when its tokens will work. The
  tokens have no meaning for him (like cookies in a browser), they are just
  something it got from the authorization server and can send with its request.
  The access token can expire, the user can revoke them, etc.

* OAuth 2.0 defines no cryptographic methods. But a developer that want to use
  it will have to put her hands in that.

If you want to learn OAuth 2 in details, I recommend the
[OAuth 2 in Action book](https://www.manning.com/books/oauth-2-in-action).

## The cozy stack as an authorization server

### GET /auth/login

Display a form with a password field to let the user authenticates herself to
the cozy stack.

This endpoint accepts a `redirect` parameter. If the user is already logged in,
she will be redirected immediately. Else, the parameter will be transfered in
the POST. This parameter can only contain a link to an application installed on
the cozy (thus to a subdomain of the cozy instance). To protect against stealing
authorization code with redirection, the fragment is always overriden:

```http
GET /auth/login?redirect=https://contacts.cozy.example.org/foo?bar#baz HTTP/1.1
Host: cozy.example.org
Cookie: ...
```

**Note**: the redirect parameter should be URL-encoded. We haven't done that to
make it clear what the path (`foo`), the query-string (`bar`), and the fragment
(`baz`) are.

```http
HTTP/1.1 302 Moved Temporarily
Location: https://contacts.cozy.example.org/foo?bar#
```

If the `redirect` parameter is invalid, the response will be `400 Bad Request`.
Same for other parameters, the redirection will happen only on success (even if
OAuth2 says the authorization server can redirect on errors, it's very
complicated to do it safely, and it is better to avoid this trap).

### POST /auth/login

After the user has typed her passphrase and clicked on `Login`, a request is
made to this endpoint.

The `redirect` parameter is passed inside the body. If it is missing, the
redirection will be made against the default target: the home application of
this cozy instance.

```http
POST /auth/login HTTP/1.1
Host: cozy.example.org
Content-Type: application/x-www-form-urlencoded

passphrase=p4ssw0rd&redirect=https%3A%2F%2Fcontacts.cozy.example.org
```

```http
HTTP/1.1 302 Moved Temporarily
Set-Cookie: ...
Location: https://contacts.cozy.example.org/foo
```

When two-factor authentication (2FA) authentication is activated, this endpoint
will not directly sent a redirection after this first passphrase step. In such
case, a `200 OK` response is sent along with a token value in the response
(either in JSON if requested or directly in a new HTML form).

Along with this token, on 2FA passcode is sent to the user via another transport
(email for instance, depending on the user's preferences). Another request
should be sent to the same endpoint with a valid pair `(token, passcode)`,
ensuring that the user correctly entered its passphrase _and_ received a fresh
passcode by another mean.

```http
POST /auth/login HTTP/1.1
Host: cozy.example.org
Content-Type: application/x-www-form-urlencoded

two-factor-token=123123123123&passcode=678678&redirect=https%3A%2F%2Fcontacts.cozy.example.org
```

```http
HTTP/1.1 302 Moved Temporarily
Set-Cookie: ...
Location: https://contacts.cozy.example.org/foo
```

### DELETE /auth/login

This can be used to log-out the user. An app token must be passed in the
`Authorization` header, to protect against CSRF attack on this (this can part of
bigger attacks like session fixation).

```http
DELETE /auth/login HTTP/1.1
Host: cozy.example.org
Cookie: seesioncookie....
Authorization: Bearer app-token
```

### DELETE /auth/login/others

This can be used to log-out all active sessions except the one used by the
request. This allow to disconnect any other users currenctly authenticated on
the system. An app token must be passed in the `Authorization` header, to
protect against CSRF attack on this (this can part of bigger attacks like
session fixation).

```http
DELETE /auth/login/others HTTP/1.1
Host: cozy.example.org
Cookie: seesioncookie....
Authorization: Bearer app-token
```

### GET /auth/passphrase_reset

Display a form for the user to reset its password, in case he has forgotten it
for example. If the user is connected, he won't be shown this form and he will
be directly redirected to his cozy.

```http
GET /auth/passphrase_reset HTTP/1.1
Host: cozy.example.org
Content-Type: text/html
Cookie: ...
```

### POST /auth/passphrase_reset

After the user has clicked on the reset button of the passphrase reset form, it
will execute a request to this endpoint.

This endpoint will create a token for the user to actually renew his passphrase.
The token has a short-live duration of about 15 minutes. After the token is
created, it is sent to the user on its mailbox.

This endpoint will redirect the user on the login form page.

```http
POST /auth/passphrase_reset HTTP/1.1
Host: cozy.example.org
Content-Type: application/x-www-form-urlencoded

csrf_token=123456890
```

### GET /auth/passphrase_renew

Display a form for the user to enter a new password. This endpoint should be
used with a `token` query parameter. This token makes sure that the user has
actually reset its passphrase and should have been sent via its mailbox.

```http
GET /auth/passphrase_renew?token=123456789 HTTP/1.1
Host: cozy.example.org
Content-Type: text/html
Cookie: ...
```

### POST /auth/passphrase_renew

After the user has entered its new passphrase in the renew passphrase form, a
request is made to this endpoint to renew the passphrase.

This endpoint requires a valid token to actually work. In case of a success, the
user is redirected to the login form.

```http
POST /auth/passphrase_reset HTTP/1.1
Host: cozy.example.org
Content-Type: application/x-www-form-urlencoded

csrf_token=123456890&passphrase_reset_token=123456789&passphrase=mynewpassphrase
```

### POST /auth/register

This route is used by OAuth2 clients to dynamically register them-selves.

See
[OAuth 2.0 Dynamic Client Registration Protocol](https://tools.ietf.org/html/rfc7591)
for the details.

The client must send a JSON request, with at least:

* `redirect_uris`, an array of strings with the redirect URIs that the client
  will use in the authorization flow
* `client_name`, human-readable string name of the client to be presented to the
  end-user during authorization
* `software_id`, an identifier of the software used by the client (it should
  remain the same for all instances of the client software, whereas `client_id`
  varies between instances).

It can also send the optional fields:

* `client_kind` (possible values: web, desktop, mobile, browser, etc.)
* `client_uri`, URL string of a web page providing information about the client
* `logo_uri`, to display an icon to the user in the authorization flow
* `policy_uri`, URL string that points to a human-readable privacy policy
  document that describes how the deployment organization collects, uses,
  retains, and discloses personal data
* `software_version`, a version identifier string for the client software.
* `notification_platform`, to activate notifications on the associated
  device, this field specify the platform used to send notifications:
  * `"android"`: for Android devices with notifications via Firebase Cloud
    Messageing
  * `"ios"`: for iOS devices with notifications via APNS/2.
* `notification_device_token`, the token used to identify the mobile device
  for notifications

The server gives to the client the previous fields and these informations:

* `client_id`
* `client_secret`
* `registration_access_token`

Example:

```http
POST /auth/register HTTP/1.1
Host: cozy.example.org
Content-Type: application/json
Accept: application/json
```

```json
{
  "redirect_uris": ["https://client.example.org/oauth/callback"],
  "client_name": "Client",
  "software_id": "github.com/example/client",
  "software_version": "2.0.1",
  "client_kind": "web",
  "client_uri": "https://client.example.org/",
  "logo_uri": "https://client.example.org/logo.svg",
  "policy_uri": "https://client/example.org/policy"
}
```

```http
HTTP/1.1 201 Created
Content-Type: application/json
```

```json
{
  "client_id": "64ce5cb0-bd4c-11e6-880e-b3b7dfda89d3",
  "client_secret": "eyJpc3Mi[...omitted for brevity...]",
  "client_secret_expires_at": 0,
  "registration_access_token": "J9l-ZhwP[...omitted for brevity...]",
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "redirect_uris": ["https://client.example.org/oauth/callback"],
  "client_name": "Client",
  "software_id": "github.com/example/client",
  "software_version": "2.0.1",
  "client_kind": "web",
  "client_uri": "https://client.example.org/",
  "logo_uri": "https://client.example.org/logo.svg",
  "policy_uri": "https://client/example.org/policy"
}
```

### GET /auth/register/:client-id

This route is used by the clients to get informations about them-selves. The
client has to send its registration access token to be able to use this
endpoint.

See
[OAuth 2.0 Dynamic Client Registration Management Protocol](https://tools.ietf.org/html/rfc7592)
for more details.

```http
GET /auth/register/64ce5cb0-bd4c-11e6-880e-b3b7dfda89d3 HTTP/1.1
Host: cozy.example.org
Accept: application/json
Authorization: Bearer J9l-ZhwP...
```

```http
HTTP/1.1 201 Created
Content-Type: application/json
```

```json
{
  "client_id": "64ce5cb0-bd4c-11e6-880e-b3b7dfda89d3",
  "client_secret": "eyJpc3Mi[...omitted for brevity...]",
  "client_secret_expires_at": 0,
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "redirect_uris": ["https://client.example.org/oauth/callback"],
  "client_name": "Client",
  "software_id": "github.com/example/client",
  "software_version": "2.0.1",
  "client_kind": "web",
  "client_uri": "https://client.example.org/",
  "logo_uri": "https://client.example.org/logo.svg",
  "policy_uri": "https://client/example.org/policy"
}
```

### PUT /auth/register/:client-id

This route is used by the clients to update informations about them-selves. The
client has to send its registration access token to be able to use this
endpoint.

**Note:** the client can ask to change its `client_secret`. To do that, it must
send the current `client_secret`, and the server will respond with the new
`client_secret`.

```http
PUT /auth/register/64ce5cb0-bd4c-11e6-880e-b3b7dfda89d3 HTTP/1.1
Host: cozy.example.org
Accept: application/json
Content-Type: application/json
Authorization: Bearer J9l-ZhwP...
```

```json
{
  "client_id": "64ce5cb0-bd4c-11e6-880e-b3b7dfda89d3",
  "client_secret": "eyJpc3Mi[...omitted for brevity...]",
  "redirect_uris": ["https://client.example.org/oauth/callback"],
  "client_name": "Client",
  "software_id": "github.com/example/client",
  "software_version": "2.0.2",
  "client_kind": "web",
  "client_uri": "https://client.example.org/",
  "logo_uri": "https://client.example.org/client-logo.svg",
  "policy_uri": "https://client/example.org/policy",
  "notification_platform": "android",
  "notification_device_token": "XXXXxxxx..."
}
```

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "client_id": "64ce5cb0-bd4c-11e6-880e-b3b7dfda89d3",
  "client_secret": "IFais2Ah[...omitted for brevity...]",
  "client_secret_expires_at": 0,
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "redirect_uris": ["https://client.example.org/oauth/callback"],
  "client_name": "Client",
  "software_id": "github.com/example/client",
  "software_version": "2.0.2",
  "client_kind": "web",
  "client_uri": "https://client.example.org/",
  "logo_uri": "https://client.example.org/client-logo.svg",
  "policy_uri": "https://client/example.org/policy"
}
```

### DELETE /auth/register/:client-id

This route is used by the clients to unregister them-selves. The client has to
send its registration access token to be able to use this endpoint.

```http
DELETE /auth/register/64ce5cb0-bd4c-11e6-880e-b3b7dfda89d3 HTTP/1.1
Host: cozy.example.org
Authorization: Bearer J9l-ZhwP...
```

```http
HTTP/1.1 204 No Content
Content-Type: application/json
```

### GET /auth/authorize

When an OAuth2 client wants to get access to the data of the cozy owner, it
starts the OAuth2 dance with this step. The user is shown what the client asks
and has an accept button if she is OK with that.

The parameters are:

* `client_id`, that identify the client
* `redirect_uri`, it has to be exactly the same as the one used in registration
* `state`, it's a protection against CSRF on the client (a random string
  generated by the client, that it can check when the user will be redirected
  with the authorization code. It can be used as a key in local storage for
  storing a state in a SPA).
* `response_type`, only `code` is supported
* `scope`, a space separated list of the [permissions](permissions.md) asked
  (like `io.cozy.files:GET` for read-only access to files).

```http
GET /auth/authorize?client_id=oauth-client-1&response_type=code&scope=io.cozy.files:GET%20io.cozy.contacts&state=Eh6ahshepei5Oojo&redirect_uri=https%3A%2F%2Fclient.org%2F HTTP/1.1
Host: cozy.example.org
```

**Note** we warn the user that he is about to share his data with an application
which only the callback URI is guaranteed.

### POST /auth/authorize

When the user accepts, her browser send a request to this endpoint:

```http
POST /auth/authorize HTTP/1.1
Host: cozy.example.org
Content-Type: application/x-www-form-urlencoded

state=Eh6ahshepei5Oojo&client_id=oauth-client-1&scope=io.cozy.files:GET%20io.cozy.contacts&csrf_token=johw6Sho
```

**Note**: this endpoint is protected against CSRF attacks.

The user is then redirected to the original client, with an access code in the
URL:

```http
HTTP/1.1 302 Moved Temporarily
Location: https://client.org/?state=Eh6ahshepei5Oojo&code=Aih7ohth#
```

### GET /auth/authorize/sharing & POST /auth/authorize/sharing

They are similar to `/auth/authorize`: they also make the user accept an OAuth
thing, but it is specialized for sharing. They are a few differences, like the
scope format (sharing rules, not permissions) and the redirection after the
POST.

### POST /auth/access_token

Now, the client can check that the state is correct, and if it is the case, ask
for an `access_token`. It can use this route with the `code` given above.

This endpoint is also used to refresh the access token, by sending the
`refresh_token` instead of the `code`.

The parameters are:

* `grant_type`, with `authorization_code` or `refresh_token` as value
* `code` or `refresh_token`, depending on which grant type is used
* `client_id`
* `client_secret`

Example:

```http
POST /auth/access_token HTTP/1.1
Host: cozy.example.org
Content-Type: application/x-www-form-urlencoded
Accept: application/json

grant_type=authorization_code&code=Aih7ohth&client_id=oauth-client-1&client_secret=Oung7oi5
```

```http
HTTP/1.1 200 OK
Content-type: application/json

{
  "access_token": "ooch1Yei",
  "token_type": "bearer",
  "refresh_token": "ui0Ohch8",
  "scope": "io.cozy.files:GET io.cozy.contacts"
}
```

### FAQ

> What format is used for tokens?

The access tokens are formatted as [JSON Web Tokens (JWT)](https://jwt.io/),
like this:

| Claim   | Fullname  | What it identifies                                                 |
| ------- | --------- | ------------------------------------------------------------------ |
| `aud`   | Audience  | Identify the recipient where the token can be used (like `access`) |
| `iss`   | Issuer    | Identify the Cozy instance (its domain in fact)                    |
| `iat`   | Issued At | Identify when the token was issued (Unix timestamp)                |
| `sub`   | Subject   | Identify the client that can use the token                         |
| `scope` | Scope     | Identify the scope of actions that the client can accomplish       |

The `scope` is used for [permissions](permissions.md).

Other tokens can be JWT with a similar formalism, or be a simple random value
(when we want to have a clear revocation process).

> What happens when the user has lost her passphrase?

She can reset it from the command-line, like this:

```sh
$ cozy-stack instances reset-passphrase cozy.example.org
ek0Jah1R
```

A new password is generated and print in the console.

> Is two-factor authentication (2FA) possible?

Yes, it's possible. Via the cozy-settings application, the two-factor
authentication can be activated.

Here is how it works in more details:

On each connection, when the 2FA is activated, the user is asked for its
passphrase first. When entering correct passphrase, the user is then asked for:

* a TOTP (Timebased One-Time password, RFC 6238) derived from a secret
  associated with the instance.
* a short term timestamped MAC with the same validity time-range and also
  derived from the same secret.

The TOTP is valid for a time range of about 5 minutes. When sending a correct
and still-valid pair `(passcode, token)`, the user is granted with
authentication cookie.

The passcode can be sent to the instance's owner via email — more transport
shall be added later.

## Client-side apps

**Important**: OAuth2 is not used here! The steps looks similar (like obtaining
a token), but when going in the details, it doesn't match.

### How to register the application?

The application is registered at install. See [app management](apps.md) for
details.

### How to get a token?

When a user access an application, she first loads the HTML page. Inside this
page, a token specific to this app is injected (only for private routes), via a
templating method.

We have prefered our custom solution to the implicit grant type of OAuth2 for 2
reasons:

1. It has a better User Experience. The implicit grant type works with 2
   redirections (the application to the stack, and then the stack to the
   application), and the first one needs JS to detect if the token is present or
   not in the fragment hash. It has a strong impact on the time to load the
   application.

2. The implicit grant type of OAuth2 has a severe drawback on security: the
   token appears in the URL and is shown by the browser. It can also be leaked
   with the HTTP `Referer` header.

The token will be given only for the authenticated user. For nested subdomains
(like `calendar.joe.example.net`), the session cookie from the stack is enough
(it is for `.joe.example.net`).

But for flat subdomains (like `joe-calendar.example.net`), it's more
complicated. On the first try of the user, she will be redirected to the stack.
As she is already logged-in, she will be redirected to the app with a session
code (else she can login). This session code can be exchanged to a session
cookie. A redirection will still happen to remove the code from the URL (it
helps to avoid the code being saved in the browser history). For security
reasons, the session code have the following properties:

* It can only be used once.
* It is tied to an application (`calendar` in our example).
* It has a very short time span of validity (1 minute).

### How to use a token?

The token can be sent to the cozy-stack as a `Bearer` token in the
`Authorization` header, like this:

```http
GET /data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee HTTP/1.1
Host: cozy.example.org
Authorization: Bearer application-token
```

If the user is authenticated, her cookies will be sent automatically. The
cookies are needed for a token to be valid.

### How to refresh a token?

The token is valid only for 24 hours. If the application is opened for more than
that, it will need to get a new token. But most applications won't be kept open
for so long and it's okay if they don't try to refresh tokens. At worst, the
user just had to reload its page and it will work again.

The app can know it's time to get a new token when the stack starts sending 401
Unauthorized responses. In that case, it can fetches the same html page that it
was loaded initially, parses it and extracts the new token.

## Third-party websites

### How to register the application?

If a third-party websites would like to access a cozy, it had to register first.
For example, a big company can have data about a user and may want to offer her
a way to get her data back in her cozy. When the user is connected on the
website of this company, she can give her cozy address. The website will then
register on this cozy, using the OAuth2 Dynamic Client Registration Protocol, as
explained [above](#post-authregister).

### How to get a token?

To get an access token, it's enough to follow the authorization code flow of
OAuth2:

* sending the user to the cozy, on the authorize page
* if the user approves, she is then redirected back to the client
* the client gets the access code and can exchange it to an access token.

### How to use a token?

The access token can be sent as a bearer token, in the Authorization header of
HTTP:

```http
GET /data/io.cozy.contacts/6494e0ac-dfcb-11e5-88c1-472e84a9cbee HTTP/1.1
Host: cozy.example.org
Accept: application/json
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWV9.TJVA95OrM7E2cBab30RMHrHDcEfxjoYZgeFONFh7HgQ
```

### How to refresh a token?

The access token will be valid only for 24 hours. After that, a new access token
must be asked. To do that, just follow the refresh token flow, as explained
[above](#post-authaccess_token).

## Devices and browser extensions

For devices and browser extensions, it is nearly the same than for third-party
websites. The main difficulty is the redirect_uri. In OAuth2, the access code is
given to the client by redirecting the user to an URL controlled by the client.
But devices and browser extensions don't have an obvious URL for that.

The IETF has published an RFC called
[OAuth 2.0 for Native Apps](https://tools.ietf.org/html/draft-ietf-oauth-native-apps-05).

### Native apps on desktop

A desktop native application can start an embedded webserver on localhost. The
redirect_uri will be something like `http://127.0.0.1:19856/callback`.

### Native apps on mobile

On mobile, the native apps can often register a custom URI scheme, like
`com.example.oauthclient:/`. Just be sure that no other app has registered
itself with the same URI.

### Chrome extensions

Chrome extensions can use URL like
`https://<extension-id>.chromiumapp.org/<anything-here>` for their usage. See
https://developer.chrome.com/apps/app_identity#non for more details. It has also
a method to simplify the creation of such an URL:
[`chrome.identity.getRedirectURL`](https://developer.chrome.com/apps/identity#method-getRedirectURL).

### Firefox extensions

It is possible to use an _out of band_ URN: `urn:ietf:wg:oauth:2.0:oob:auto`.
The token is then extracted from the title of the page. See
[this addon for google oauth2](https://github.com/AdrianArroyoCalle/firefox-addons/blob/master/addon-google-oauth2/addon-google-oauth2.js)
as an example.

## Security considerations

The password will be stored in a secure fashion, with a password hashing
function. The hashing function and its parameter will be stored with the hash,
in order to make it possible to change the algorithm and/or the parameters later
if we had any suspicion that it became too weak. The initial algorithm is
[scrypt](https://godoc.org/golang.org/x/crypto/scrypt).

The access code is valid only once, and will expire after 5 minutes

Dynamically registered applications won't have access to all possible scopes.
For example, an application that has been dynamically registered can't ask the
cozy owner to give it the right to install other applications. This limitation
should improve security, as avoiding too powerful scopes to be used with unknown
applications.

The cozy stack will apply rate limiting to avoid brute-force attacks.

The cozy stack offers
[CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/Access_control_CORS)
for most of its services. But it's disabled for `/auth` (it doesn't make sense
here) and for the client-side applications (to avoid leaking their tokens).

The client should really use HTTPS for its `redirect_uri` parameter, but it's
allowed to use HTTP for localhost, as in the native desktop app example.

OAuth2 says that the `state` parameter is optional in the authorization code
flow. But it is mandatory to use it with Cozy.

For more on this subject, here is a list of links:

* https://www.owasp.org/index.php/Authentication_Cheat_Sheet
* https://tools.ietf.org/html/rfc6749#page-53
* https://tools.ietf.org/html/rfc6819
* https://tools.ietf.org/html/draft-ietf-oauth-closing-redirectors-00
* http://www.oauthsecurity.com/

## Conclusion

Security is hard. If you want to share some concerns with us, do not hesitate to
send us an email to security AT cozycloud.cc.

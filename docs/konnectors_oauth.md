# Doctypes

`io.cozy.konnectors` gives their desiderata for an account

```json
{
  "fields": {
    "account": {
      "doctype": "io.cozy.accounts",
      "account_type": "maif",
      "scope": "openid profile offline_access"
    }
  }
}
```

`io.cozy.accounts` contains authentication information for an account, as well
as the associated scope

```json
{
  "name": "Mon Compte Maif",
  "account_type": "maif",
  "status": "connected",
  "oauth": {
    "access_token": "akosaksoakso",
    "refresh_token": "okoakozkaozk",
    "scope": "openid profile offline_access"
  }
}
```

`io.cozy.account_types` contains the oauth configuration

```json
{
  "_id": "service.example",
  "grant_mode": "authorization_code",
  "client_id": "the registered client id",
  "client_secret": "client_secret is necessary for server-flow",
  "auth_endpoint": "https://service.example/auth",
  "token_endpoint": "https://api.service.example/token"
}
```

io.cozy.account_types should not be accessible to the applications. They will be
loaded into the stack by infra and will be configurable through config files for
self-hosters.

# Reminder OAuth flow

**Service** is the website from which konnector aims to retrieve informations.
**Stack** is the cozy stack

Oauth is divided in 3 steps

* Client Registration: the client application (the Stack) needs to be registered
  with the Service.
* Obtaining & Refreshing Authorization: all the steps from client_id to
  access_token, the Stack will handle those
* Using the access_token: Ideally, the konnector should only concern itself with
  this part; it receives an access_token and uses it.

# Client Registration

Before beginning the Grant process, most Services require the application to be
registered with a defined redirect_uri.

**TL:DR** Lot of options, which we will choose from when actually implementing
konnectors using them.

## Manually

Most services requires a human developer to create the client manually and
define its redirect_uri. However each instance has its own domain, so for these
services, we will need to:

**A. Register a "proxy" client**, which is a static page performing redirections
as needed, as was done for Facebook events in v2 konnectors. We will register a
well known cozy domain, like `oauth-proxy.cozy.io` and registers it with all
providers. The use and risks associated with this domain should be made clear to
the user.

**B. Register each Stack** with a redirect_uri on the main stack domain, if we
go this way, the register_uri below moves from
`bob.cozy.rocks/accounts/redirect` to `_cozy_oauth.cozy.rocks/redirect` and the
domain will be prepended to the state. This is feasible at cozy scale, but
requires more knowledge and work for self-hosters.

## Dynamic Registration Protocol

A few services allows client to register programaticaly through the Dynamic
Client Registration Protocol [RFC7591](https://tools.ietf.org/html/rfc7591), we
should allow the Stack to register using this protocol when we first need to
implement a Konnector connecting to such a Service.

## No redirect_url enforcement

A few services allows to specify arbitrary redirect_url without registering
beforehand as a client.

# Authorization Grant flows

## webserver flow (Authorization Code Grant)

A. In SettingsApp give a link

```html
  <a href="https://bob.cozy.rocks/accounts/service-name/start? (url)
      scope=photos&
      state=1234zyx">
```

**NOTE** the scope may depends on other fields being configured (checkboxes),
this will be described in json in the konnectors manifest. The format will be
determined upon implementation.

**NOTE** To limit bandwidth and risk of state corruption, SettingsApp should
save its state under a random key into localStorage, the key is then passed as
the state in this query.

B. Service let the user login, allow or deny scope Then redirect to

```http
https://bob.cozy.rocks/accounts/service-name/redirect? (url)
  code=AUTH_CODE_HERE&
  state=1234zyx
```

C. The Stack does Server side

```http
POST https://api.service.example/token
Content-Type:
  grant_type=authorization_code&
  code=AUTH_CODE_HERE&
  redirect_uri=https://bob.cozy.rocks/accounts/service-name/redirect&
  client_id=CLIENT_ID&
  client_secret=CLIENT_SECRET
```

D. The Service responds (server side) with (json)

```json
{
  "access_token": "ACCESS_TOKEN",
  "token_type": "bearer",
  "expires_in": 2592000,
  "refresh_token": "REFRESH_TOKEN",
  "scope": "read",
  "uid": 100101,
  "info": { "name": "Claude Douillet", "email": "claude.douillet@example.com" }
}
```

This whole object is saved as-is into a `io.cozy.accounts` 's `extras` field.

The known fields `access_token`, `refresh_token` & `scope` will be **also**
saved on the account's `oauth` itself

E. The Stack redirect the user to SettingsApp

```http
HTTP/1.1 302 Found
Location: https://bob-settings.cozy.rocks/?state=1234zyx&account=accountID
```

SettingsApp check the state is expected and restore its state to the form but
whith account completed.

## SPA flow (Implicit grant)

A. In SettingsApp give a link

```html
<a href="https://service.example/auth? (url)
    response_type=token&
    client_id=CLIENT_ID&
    redirect_uri=https://bob-settings.cozy.rocks/&
    scope=photos&
    state=1234zyx">
```

See server-flow for state rules.

B. Service let the user login, allow or deny scope Then redirects to

```http
https://bob-settings.cozy.rocks/?
access_token=ACCESS_TOKEN&
state=1234zyx
```

C. SettingsApp adds the token to the `io.cozy.accounts` and save it before
starting konnector.

# Accessing data

Once we have an account, SettingsApp starts the konnector with the proper
account id. The konnector can then fetch the account and use its access_token to
performs a request to fetch data

```http
POST https://api.service.com/resource
  Authorization: Bearer ACCESS_TOKEN
```

# Refreshing token

When using the server-flow, we also get a refresh_token. It is used to get a new
access_token when it expires. However, if konnectors are responsible for
refreshing token there is a race condition risk :

```
(konnector A) GET  https://api.service.com/resource TOKEN1     -> expired
(konnector B) GET  https://api.service.com/resource TOKEN1     -> expired
(konnector A) POST https://api.service.com/token REFRESH_TOKEN -> TOKEN2a
(konnector B) POST https://api.service.com/token REFRESH_TOKEN -> TOKEN2b
(konnector A) GET  https://api.service.com/token TOKEN2a       -> invalid
```

To avoid this, the stack will be responsible to perform token refresh. A
konnector can requires the stack to refresh an account token with an HTTP
request.

```
POST https://bob-settings.cozy.rocks/accounts/:accountID/refresh
```

# Konnectors Marketplace Requirements

The following is a few points to be careful for in konnectors when we start
allowing non-cozy developped OAuth konnectors.

* With SPA flow, because of advanced security concerns (confused deputy
  problem), cozy should validate the `access_token`. However, the way to do that
  depends on the provider and cannot be described in json, it is therefore the
  responsibility of the konnector itself.

# Account types security rules

* With server flow, an evil account type with proper `auth_endpoint` but bad
  `token_endpoint` could retrieve a valid token as well as cozy client secret.
  The reviewer of an `account_type` should make sure both these endpoints are on
  domains belonging to the Service provider.

# Notes for MesInfos experiment

* MAIF konnector uses the webserver flow without redirect_uri validation
* Orange konnector uses the client-side proxy but hosted on their own servers
  (/!\ redirect_uri vs redirect_url)

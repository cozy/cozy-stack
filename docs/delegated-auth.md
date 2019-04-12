[Table of contents](README.md#table-of-contents)

# Delegated authentication

In general, the cozy stack manages the authentication itself. In some special
cases, an integration with other softwares can be mandatory. It's possible to
use JWT or OpenID Connect, with a bit of configuration to do that.

## JWT

To enable an external system to create links with a JWT to log in users for
cozy instances in a given context, we just need to add the secret to use for
checking the JWT in the config, like this:

```yaml
authentication:
  the-context-name:
    jwt_secret: s3cr3t
```

The external system can then create a JWT, with the parameter `name` as the
instance domain, and send the user to `https://<instance>/?jwt=...`. The user
will be logged in, and redirected to its default application.

## Open ID Connect

For OpenID Connect, there are more configuration parameters:

```yaml
authentication:
  the-context-name:
    oidc:
      client_id: aClientID
      client_secret: s3cret3
      scope: openid profile
      redirect_uri: https://oauthcallback.mycozy.cloud/oidc/redirect
      authorize_url: https://identity-prodiver/path/to/authorize
      token_url: https://identity-prodiver/path/to/token
      userinfo_url: https://identity-prodiver/path/to/userinfo
      userinfo_instance_field: cozy_number
      userinfo_instance_prefix: name
      userinfo_instance_suffix: .mycozy.cloud
```

Let's see what it means:

- `client_id` and `client_secret` are the OAuth client that will be used to
  talk to the identity provider
- `scope` is the OAuth scope parameter
- `redirect_uri` is where the user will be redirected by the identity provider
  after login (it must often be declared when creating the OAuth client, and we
  have to use a static hostname, not the hostname of a cozy instance)
- `token_url`, `authorize_url`, and `userinfo_url` are the URLs used to talk to
  the identity provider
- `userinfo_instance_field` is the JSON field to use in the UserInfo response
  to know the cozy instance of the logged in user.

Let's see the 3 routes used in this process

### GET /oidc/start

To start the OpenID Connect dance, the user can go to this URL. It will
redirect him/her to the identity provider with the rights parameter.

```http
GET /oidc/start HTTP/1.1
Host: name00001.mycozy.cloud
```

```http
HTTP/1.1 303 See Other
Location: https://identity-prodiver/path/to/authorize?response_type=code&state=9f6873dfce7d&scope=openid+profile&client_id=aClientID&nonce=94246498&redirect_uri=https://oauthcallback.mycozy.cloud/oidc/redirect
```

### GET /oidc/redirect

Then, the user can log in on the identity provider, and then he/she will be
redirected to this URL. Note that the URL is on a generic domain: the stack
will redirect the user to his/her instance (where it's possible to create
cookies to log in the user).

```http
GET /oidc/redirect?state=9f6873dfce7d&code=ccd0032a HTTP/1.1
Host: oauthcallback.mycozy.cloud
```

```http
HTTP/1.1 303 See Other
Location: https://name00001.mycozy.cloud/oidc/login?state=9f6873dfce7d&code=ccd0032a
```

### GET /oidc/login

On this route, the stack can create the session for the user, with the cookies.

```http
GET /oidc/login?state=9f6873dfce7d&code=ccd0032a HTTP/1.1
Host: name00001.mycozy.cloud
```

```http
HTTP/1.1 303 See Other
Set-Cookie: ...
Location: https://name00001-home.mycozy.cloud/
```

### POST /oidc/access_token

This additional route can be used by an OAuth client when delegated
authentication via OpenID Connect is enabled. It allows the client to obtain an
`access_token` for requesting the cozy-stack in exchange of a token valid on the
OpenID Connect Identity Provider.

```http
POST /oidc/access_token HTTP/1.1
Host: name00001.mycozy.cloud
Accept: application/json
Content-Type: application/json
```

```json
{
  "client_id": "55eda056e85468fdfe2c8440d4009cbe",
  "client_secret": "DttCGIUOTniVNkivR_CsZ_xRoME9gghN",
  "scope": "io.cozy.files io.cozy.photos.albums",
  "oidc_token": "769fa760-59de-11e9-a167-9bab3784e3e7"
}
```

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "access_token": "ooch1Yei",
  "token_type": "bearer",
  "refresh_token": "ui0Ohch8",
  "scope": "io.cozy.files io.cozy.photos.albums"
}
```

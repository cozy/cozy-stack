[Table of contents](README.md#table-of-contents)

# Applications

It's possible to manage serverless applications from the cozy stack and serve
them via cozy stack. The stack does the routing and serve the HTML and the
assets for the applications. The assets of the applications are installed in
the virtual file system.


## Install an application

### The manifest

To install an application, cozy needs a manifest. It's a JSON document that
describes the application (its name and icon for example), how to install it
and what it needs for its usage (the permissions in particular). While we have
considered to use the same [manifest format as the W3C for
PWAs](https://www.w3.org/TR/appmanifest/), it didn't match our expectations.
The [manifest format for
FirefoxOS](https://developer.mozilla.org/en-US/docs/Archive/Firefox_OS/Firefox_OS_apps/Building_apps_for_Firefox_OS/Manifest)
is a better fit. We took a lot of inspirations from it, starting with the
filename for this file: `manifest.webapp`.

Field          | Description
---------------|---------------------------------------------------------------------
name           | the name to display on the home
slug           | the default slug (it can be changed at install time)
icon           | an icon for the home
description    | a short description of the application
source         | where the files of the app can be downloaded
developer      | `name` and `url` for the developer
default_locale | the locale used for the name and description fields
locales        | translations of the name and description fields in other locales
version        | the current version number
license        | [the SPDX license identifier](https://spdx.org/licenses/)
permissions    | a map of permissions needed by the app (see [here](permissions.md) for more details)
routes         | a map of routes for the app (see below for more details)

### Routes

A route make the mapping between the requested paths and the files. It can
have an index, which is an HTML file, with a token injected on it that
identify both the application. This token must be used with the user cookies
to use the services of the cozy-stack.

By default, a route can be only visited by the authenticated owner of the
instance where the app is installed. But a route can be marked as public.
In that case, anybody can visit the route.

For example, an application can offer an administration interface on `/admin`,
a public page on `/public`, and shared assets in `/assets`:

```json
{
  "/admin": {
    "folder": "/",
    "index": "admin.html",
    "public": false
  },
  "/public": {
    "folder": "/public",
    "index": "index.html",
    "public": true
  },
  "/assets": {
    "folder": "/assets",
    "public": true
  }
}
```

If an application has no routes in its manifest, the stack will create one
route, this default one:

```json
{
  "/": {
    "folder": "/",
    "index": "index.html",
    "public": false
  }
}
```

**TODO** later, it will be possible to associate an intent /
[activity](https://developer.mozilla.org/en-US/docs/Archive/Firefox_OS/Firefox_OS_apps/Building_apps_for_Firefox_OS/Manifest#activities)
to a route. Probably something like:

```json
{
  "/picker": {
    "folder": "/",
    "index": "picker.html",
    "public": false,
    "intent": {
      "action": "pick",
      "type": "io.cozy.contacts"
    }
  }
}
```

### GET /apps/manifests

Give access to the manifest for an application. It can have several usages,
but the most important one is to display informations about the app to the
user so that she can install the app and give the permissions in full
knowledge of the cause.

#### Query-String

Parameter | Description
----------|-----------------------------------------
Source    | URL from where the app can be downloaded

#### Request

```http
GET /apps/manifests?Source=git://github.com/cozy/cozy-emails.git HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.manifests",
    "id": "git://github.com/cozy/cozy-emails",
    "attributes": {
      "name": "cozy-emails",
      "slug": "emails",
      "icon": "icon.svg",
      "description": "A webmail for Cozy Cloud",
      "source": "git://github.com/cozy/cozy-emails",
      "developer": {
        "name": "Cozy",
        "url": "https://cozy.io/"
      },
      "default_locale": "en",
      "locales": {
        "fr": {
          "description": "Un client web pour les courriels",
          "permissions": {
            "mails": {
              "description": "Requis pour lire et écrire des emails"
            }
          }
        }
      },
      "version": "1.2.3",
      "license": "AGPL-3.0",
      "permissions": {
        "mails": {
          "description": "Required for reading and writing emails",
          "type": "io.cozy.emails"
        }
      }
    }
  }
}
```

#### Notes

- The server should keep the manifest in cache, as it will be probably used in
  the near future to install the application.
- To start, we will implement a git provider to fetch manifest and install
  apps. Later, we will add other providers, like mercurial and npm.
- It's possible to use a branch for git, by putting it the fragment of the
  URL, like `git://github.com/cozy/cozy-emails#develop`.
- To download the manifest with git, we can use [git
  archive](https://www.kernel.org/pub/software/scm/git/docs/git-archive.html),
  except on github (where it's blocked). For github, we can use
  `https://raw.githubusercontent.com/:user/:project/:branch/manifest.webapp`

### POST /apps/:slug

Install or update an application, ie download the files and put them in
`/apps/:slug` in the virtual file system of the user, create an `io.cozy/apps`
document, register the permissions, etc.

This endpoint is asynchronous and returns a successful return as soon as the application installation has started, meaning we have successfully reached the manifest and started to fetch application data.

#### Status codes

* 202 Accepted, when the application installation has been accepted.
* 400 Bad-Request, when the manifest of the application could not be processed (for instance, it is not valid JSON).
* 404 Not Found, when the manifest or the source of the application is not reachable.
* 422 Unprocessable Entity, when the sent data is invalid (for example, the slug is invalid or the Source parameter is not a proper or supported url)

#### Query-String

Parameter | Description
----------|------------------------------------------------------------
Source    | URL from where the app can be downloaded (only for install)

#### Request

```http
POST /apps/emails?Source=git://github.com/cozy/cozy-emails HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```http
HTTP/1.1 202 Accepted
Content-Type: application/vnd.api+json
```

```json
{
  "data": [{
    "id": "4cfbd8be-8968-11e6-9708-ef55b7c20863",
    "type": "io.cozy.applications",
    "attributes": {
      "name": "calendar",
      "state": "installing",
      ...
    }
  }]
}
```


## List installed applications

### GET /apps/

An application can be in one of these states:

- `ready`, the user can use it
- `installing`, the installation is running and the app will soon be usable
- `upgrading`, a new version is being installed
- `uninstalling`, the app will be removed, and will return to the `available` state.
- `errored`, the app is in an error state and can not be used.

#### Request

```http
GET /apps/ HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": [{
    "id": "4cfbd8be-8968-11e6-9708-ef55b7c20863",
    "type": "io.cozy.applications",
    "attributes": {
      "name": "calendar",
      "state": "ready",
      ...
    }
  }]
}
```


## Manage the marketplace

### GET /apps/manifests

List applications in the marketplace.

### POST /apps/manifests

Add an application to the marketplace. The payload is a subset of the
manifest, with at least `name` and `source`. But it's possible to add the
other fields of the manifest to give more informations.

#### Request

```http
GET /apps?filter[state]=installed HTTP/1.1
Accept: application/vnd.api+json
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.manifests",
    "attributes": {
      "name": "cozy-emails",
      "slug": "emails",
      "icon": "/Apps/marketplace/emails.svg",
      "source": "git://github.com/cozy/cozy-emails",
      "default_locale": "en",
      "description": "A webmail for Cozy Cloud",
      "locales": {
        "fr": {
          "name": "courriels",
          "description": "Un client web pour les courriels dans Cozy Cloud"
        }
      }
    }
  }
}
```

#### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "id": "4f6436ce-8967-11e6-b174-ab83adac69f2",
    "type": "io.cozy.manifests",
    "attributes": {
      "name": "cozy-emails",
      "slug": "emails",
      "icon": "/Apps/marketplace/emails.svg",
      "source": "git://github.com/cozy/cozy-emails",
      "default_locale": "en",
      "description": "A webmail for Cozy Cloud",
      "locales": {
        "fr": {
          "name": "courriels",
          "description": "Un client web pour les courriels dans Cozy Cloud"
        }
      }
    }
  }
}
```

### PATCH /apps/manifests/:id

Update an application in the marketplace.

### DELETE /apps/manifests/:id

Remove an application from the marketplace.


## Uninstall an application

### DELETE /apps/:slug

#### Request

```http
DELETE /apps/tasky HTTP/1.1
```

#### Response

```http
HTTP/1.1 204 No Content
```


## Access an application

Each application will run on its sub-domain. The sub-domain is the slug used
when installing the application (`calendar.cozy.example.org` if it was
installed via a POST on /apps/calendar). On the main domain
(`cozy.example.org`), there will be the registration process, the login form,
and it will redirect to `home.cozy.example.org` for logged-in users.

### Rationale

The applications have different roles and permissions. An application is
identified when talking to the stack via a token. The token has to be injected
in the application, one way or another, and it have to be unaccessible from
other apps.

If the applications run on the same domain (for example
`https://cozy.example.org/apps/calendar`), it's nearly impossible to protect
an application to take the token of another application. The security model of
the web is based too heavily on [Same Origin
Policy](https://developer.mozilla.org/en-US/docs/Web/Security/Same-origin_policy)
for that. If the token is put in the html of the index.html page of an app,
another app can use the fetch API to get the html page, parse its content and
extract the token. We can think of other ways to inject the token (indexeddb,
URL, cookies) but none will provide a better isolation. Maybe in a couple
years, when [the origin spec](https://w3c.github.io/webappsec-suborigins/)
will be more advanced.

So, if having the apps on the same origin is not possible, we have to put them
on several origins. One interesting way to do that is using sandboxed iframes.
An iframe with [the sandbox
attribute](https://www.w3.org/TR/html5/embedded-content-0.html#attr-iframe-sandbox),
and not `allow-same-origin` in it, will be assigned to a unique origin. The
W3C warns:

> Potentially hostile files should not be served from the same server as the
> file containing the iframe element. Sandboxing hostile content is of minimal
> help if an attacker can convince the user to just visit the hostile content
> directly, rather than in the iframe. To limit the damage that can be caused
> by hostile HTML content, it should be served from a separate dedicated
> domain. Using a different domain ensures that scripts in the files are
> unable to attack the site, even if the user is tricked into visiting those
> pages directly, without the protection of the sandbox attribute.

It may be possible to disable all html pages to have an html content-type,
except the home, and having the home loading the apps in a sandboxed iframe,
via the `srcdoc` attribute. But, it will mean that we will have to reinvent
nearly everything. Even showing an image can no longer be done via an `<img>`
tag, it will need to use post-message with the home. Such a solution is
difficult to implement, is a very fragile (both for the apps developer than
for security) and is an hell to debug when it breaks. Clearly, it's not an
acceptable solution.

Thus, the only choice is to have several origins, and sub-domains is the best
way for that. Of course, it has the downside to be more complicated to deploy
(DNS and TLS certificates). But, in the tradeoff between security and ease of
administration, we definetively take the security first.

### Routes

> Should we be concerned that all the routes are on the same sub-domain?

No, it's not an issue. There are two types of routes: the ones that are
publics and those reserved to the authenticated user. Public routes have no
token

Private routes are private, they can be accessed only with a valid session
cookie, ie by the owner of the instance. Another application can't use the
user cookies to read the token, because of the restrictions of the same origin
policy (they are on different domains). And the application can't use an open
proxy to read the private route, because it doesn't have the user cookies
for that (the cookie is marked as `httpOnly`).

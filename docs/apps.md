Applications
============

It's possible to manage serverless applications from the cozy stack and serve
them via cozy stack. The stack does the routing and serve the HTML and the
assets for the applications. The assets of the applications are installed in
the virtual file system.


Install an application
----------------------

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
---------------|-----------------------------------------------------------------
name           | the name to display on the home
icon           | an icon for the home
description    | a short description of the application
source         | where the files of the app can be downloaded
developer      | `name` and `url` for the developer
default_locale | the locale used for the name and description fields
locales        | translations of the name and description fields in other locales
version        | the current version number
license        | [the SPDX license identifier](https://spdx.org/licenses/)

**TODO** permissions

**TODO** intents / [activities](https://developer.mozilla.org/en-US/docs/Archive/Firefox_OS/Firefox_OS_apps/Building_apps_for_Firefox_OS/Manifest#activities)

**TODO** Contexts

**TODO** [CSP policy](https://developer.mozilla.org/en-US/docs/Archive/Firefox_OS/Firefox_OS_apps/Building_apps_for_Firefox_OS/Manifest#csp)

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
GET /apps/manifests?Source=git://github.com/cozy/cozy-emails HTTP/1.1
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
      "name": "emails",
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
          "description": "Un client web pour les courriels"
        }
      },
      "version": "1.2.3",
      "license": "AGPL-3.0"
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

### POST /apps/:slug

Install an application, ie download the files and put them in `/apps/:slug` in
the virtual file system of the user, create an `io.cozy/apps` document,
register the permissions, etc.

#### Query-String

Parameter | Description
----------|-----------------------------------------
Source    | URL from where the app can be downloaded

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
      "state": "installing"
    }
  }]
}
```


List installed applications
---------------------------

### GET /apps

An application can be in one of these states:

- `ready`, the user can use it
- `installing`, the installation is running and the app will soon be usable
- `upgrading`, a new version is being installed
- `uninstalling`, the app will be removed, and will return to the `available` state.

#### Request

```http
GET /apps HTTP/1.1
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
      "state": "ready"
    }
  }]
}
```


Manage the marketplace
----------------------

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
      "name": "emails",
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
      "name": "emails",
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


Uninstall an application
------------------------

### DELETE /apps/:slug

#### Request

```http
DELETE /apps/tasky HTTP/1.1
```

#### Response

```http
HTTP/1.1 204 No Content
```


Access an application
---------------------

**TODO**

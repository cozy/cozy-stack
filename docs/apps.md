Applications
============

It's possible to manage serverless applications from the cozy stack and serve
them via cozy stack. The stack does the routing and serve the HTML and the
assets for the applications. The assets of the applications are installed in
the virtual file system.


Install an application
----------------------

**TODO** explain the manifest

### GET /apps/manifest

Give access to the manifest for an application. It can have several usages,
but the most important one is to display informations about the app to the
user so that she can install the app and give the permissions in full
knowledge of the cause.

#### Query-String

Parameter | Description
----------|-----------------------------------------
From      | URL from where the app can be downloaded

#### Request

```http
GET /apps/manifest?From=git://github.com/cozy/cozy-emails HTTP/1.1
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
From      | URL from where the app can be downloaded

#### Request

```http
POST /apps/emails?From=git://github.com/cozy/cozy-emails HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
```


List applications
-----------------

### GET /apps

#### Query-String

Parameter     | Description
--------------|----------------------------------------------------------------------
filter[state] | give only the apps on this state (`installed`, `available`), optional

#### Request

```http
GET /apps?filter[state]=installed HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
```

### POST /apps

**TODO** explain how to add an application to the marketplace


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

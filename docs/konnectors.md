[Table of contents](README.md#table-of-contents)

# Konnectors

It is possible to manage konnectors applications from the stack. The source-code
of the konnector is installed on the cozy and the user can manage its execution
_via_ our [job system](jobs.md) and the [konnector worker](workers.md).

## Install a konnector

## The manifest

An exhaustive manifest specification is available in the [Cozy Apps Registry documentation](https://docs.cozy.io/en/cozy-apps-registry/#properties-meaning-reference)

### POST /konnectors/:slug

Install a konnector, ie download the files and put them in `/konnectors/:slug`
in the virtual file system of the user, create an `io.cozy.konnectors` document,
register the permissions, etc.

This endpoint is asynchronous and returns a successful return as soon as the
konnector installation has started, meaning we have successfully reached the
manifest and started to fetch konnector source code.

To make this endpoint synchronous, use the header `Accept: text/event-stream`.
This will make a eventsource stream sending the manifest and returning when the
konnector has been installed or failed.

#### Status codes

-   202 Accepted, when the konnector installation has been accepted.
-   400 Bad-Request, when the manifest of the konnector could not be processed
    (for instance, it is not valid JSON).
-   404 Not Found, when the manifest or the source of the konnector is not
    reachable.
-   422 Unprocessable Entity, when the sent data is invalid (for example, the
    slug is invalid or the Source parameter is not a proper or supported url)

#### Query-String

| Parameter | Description                                                 |
| --------- | ----------------------------------------------------------- |
| Source    | URL from where the app can be downloaded (only for install) |

#### Request

```http
POST /konnectors/bank101?Source=git://github.com/cozy/cozy-bank101.git HTTP/1.1
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
    "type": "io.cozy.konnectors",
    "meta": {
      "rev": "1-7a1f918147df94580c92b47275e4604a"
    },
    "attributes": {
      "name": "bank101",
      "state": "installing",
      "slug": "bank101",
      ...
    },
    "links": {
      "self": "/konnectors/bank101"
    }
  }]
}
```

**Note**: it's possible to choose a git branch by passing it in the fragment
like this:

```http
POST /konnectors/bank101-dev?Source=git://github.com/cozy/cozy-bank101.git%23dev HTTP/1.1
```

### PUT /konnectors/:slug

Update a konnector source code with the specified slug name.

This endpoint is asynchronous and returns a successful return as soon as the
konnector installation has started, meaning we have successfully reached the
manifest and started to fetch konnector source code.

To make this endpoint synchronous, use the header `Accept: text/event-stream`.
This will make a eventsource stream sending the manifest and returning when the
konnector has been updated or failed.

#### Request

```http
PUT /konnectors/bank101 HTTP/1.1
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
    "type": "io.cozy.konnectors",
    "meta": {
      "rev": "1-7a1f918147df94580c92b47275e4604a"
    },
    "attributes": {
      "name": "bank101",
      "state": "installing",
      "slug": "bank101",
      ...
    },
    "links": {
      "self": "/konnectors/bank101"
    }
  }]
}
```

#### Status codes

-   202 Accepted, when the konnector installation has been accepted.
-   400 Bad-Request, when the manifest of the konnector could not be processed
    (for instance, it is not valid JSON).
-   404 Not Found, when the konnector with the specified slug was not found or
    when the manifest or the source of the konnector is not reachable.
-   422 Unprocessable Entity, when the sent data is invalid (for example, the
    slug is invalid or the Source parameter is not a proper or supported url)

#### Advanced usage

Two optional query parameters are available for a konnector update:
-   `PermissionsAcked`: (defaults to `true`)
    -    Tells that the user accepted the permissions/ToS. It is useful if there are
    newer permissions or Terms Of Service and you want to be sure they were read
    or accepted. If set to `false`, the update will be blocked and the user will
    be told that a new konnector version is available.\
          > Note: `PermissionsAcked` can be skipped. \
          If an instance is in a `context` configured with the parameter
          `permissions_skip_verification` sets to `true`, permissions
          verification will be ignored.

-    `Source` (defaults to source url installation):
       - Use a different source to update this konnector (e.g. to install a `beta` or
    `dev` konnector version)

##### Examples:

- You have a trainline konnector on a `stable` channel, and you want to update
  it to a particular `beta` version:

```http
PUT /konnectors/trainline?Source=https://<konnectors-repository>/trainline/1.0.0-beta HTTP/1.1
Accept: application/vnd.api+json
```

- You want to attempt the trainline konnector update, but prevent it if new
  permissions were added

```http
PUT /konnectors/trainline?PermissionsAcked=false HTTP/1.1
Accept: application/vnd.api+json
```

You can combine these parameters to use a precise konnector version and stay on
another channel (when permissions are different):

  - Install a version (e.g. `1.0.0`).
  - Ask an update to `stable` channel with `PermissionsAcked` to `false`
  - `Source` will be `stable`, and your version remains `1.0.0`

## List installed konnectors

### GET /konnectors/

#### Request

```http
GET /konnectors/ HTTP/1.1
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
    "type": "io.cozy.konnectors",
    "meta": {
      "rev": "1-7a1f918147df94580c92b47275e4604a"
    },
    "attributes": {
      "name": "bank101",
      "state": "installing",
      "slug": "bank101",
      ...
    },
    "links": {
      "self": "/konnectors/bank101"
    }
  }],
  "links": {},
  "meta": {
    "count": 1
  }
}
```

This endpoint is paginated, default limit is currently `100`.
Two flags are available to retreieve the other konnectors if there are more than
`100` konnectors installed:
- `limit`
- `start_key`: The first following doc ID of the next konnectors

The `links` object contains a `ǹext` generated-link for the next docs.

#### Request

```http
GET /konnectors/?limit=50 HTTP/1.1
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
    "type": "io.cozy.konnectors",
    "meta": {
      "rev": "1-7a1f918147df94580c92b47275e4604a"
    },
    "attributes": {
      "name": "bank101",
      "state": "installing",
      "slug": "bank101",
      ...
    },
    "links": {
      "self": "/konnectors/bank101"
    }
  }, {...}],
  "links": {
    "next": "http://alice.example.com/konnectors/?limit=50&start_key=io.cozy.konnectors%2Ffookonnector"
  },
  "meta": {
    "count": 50
  }
}
```

## Get informations about a konnector

### GET /konnectors/:slug

#### Request

```http
GET /konnectors/pajemploi HTTP/1.1
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
    "type": "io.cozy.konnectors",
    "id": "io.cozy.konnectors/pajemploi",
    "attributes": {
      "name": "Pajemploi",
      "editor": "Cozy",
      "icon": "icon.svg",
      "type": "konnector",
      "language": "node",
      ...
    },
    "meta": {
      "rev": "8-1197cab1c9ecc89cef676567410cf7c5"
    },
    "links": {
      "self": "/konnectors/pajemploi",
      "icon": "/konnectors/pajemploi/icon/1.5.2",
      "permissions": "/permissions/konnectors/pajemploi"
    }
  }
}
```

## Uninstall a konnector

### DELETE /konnectors/:slug

#### Request

```http
DELETE /konnectors/bank101 HTTP/1.1
```

#### Response

```http
HTTP/1.1 204 No Content
```

Or, if the konnector has still some accounts configured:

```http
HTTP/1.1 202 Accepted
```

In this case, the stack will accept the uninstall request, then it will clean
the accounts (locally and remotely), and only after that, the konnector will be
removed.

## Add a trigger

### POST /konnectors/:slug/trigger

This endpoint creates a @cron trigger for the given konnector. The manifest is
used to generate the crontab.

#### Query-String

| Parameter | Description                                                                   |
| --------- | ----------------------------------------------------------------------------- |
| AccountID | The identifier of the io.cozy.accounts that will be used to run the konnector |
| ExecNow   | `true` if you want to run a job just after the trigger creation               |


#### Request

```http
POST /konnectors/pajemploi/trigger?AccountID=4eee63e069690139df83543d7eb8149c HTTP/1.1
```

#### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.triggers",
    "id": "8cfeef1069690139df84543d7eb8149c",
    "attributes": {
      "type": "@cron",
      "arguments": "0 9 3 * * 1",
      "worker": "konnector",
      "message": {
        "account": "4eee63e069690139df83543d7eb8149c",
        "konnector": "pajemploi"
      }
    },
    "links": {
      "self": "/jobs/triggers/8cfeef1069690139df84543d7eb8149c"
    }
  }
}
```

## Send konnector logs to cozy-stack

### POST /konnectors/:slug/logs

Send client-side logs to cozy-stack so they can be stored in the server's 
logging system.

#### Status codes

-   204 No Content, when all the log lines have been processed.
-   400 Bad-Request, when the JSON body is invalid.
-   404 Not Found, when no konnectors with the given slug could be found.
-   422 Unprocessable Entity, when the sent data is invalid (for example, the
    slug is invalid or log level does not exist)

#### Request

```http
POST /apps/pajemploi/logs HTTP/1.1
Accept: application/vnd.api+json
```

```json
[
  { "timestamp": "2022-10-27T17:13:37.293Z", "level": "info", "msg": "Connecting to remote site..." },
  { "timestamp": "2022-10-27T17:13:38.382Z", "level": "error", "msg": "LOGIN_FAILED" }
]
```

#### Response

```http
HTTP/1.1 204 No Content
```


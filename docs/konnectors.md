[Table of contents](README.md#table-of-contents)

# Konnectors

It is possible to manage konnectors applications from the stack. The source-code
of the konnector is installed on the cozy and the user can manage its execution
_via_ our [job system](jobs.md) and the [konnector worker](workers.md).

## Install a konnector

## The manifest

| Field             | Description                                                                                                     |
| ----------------- | --------------------------------------------------------------------------------------------------------------- |
| name              | the name to display on the home                                                                                 |
| slug              | the default slug (it can be changed at install time)                                                            |
| editor            | the editor's name to display on the cozy-bar of the app                                                         |
| type              | the type of the konnector source ("node" is the only supported for now)                                         |
| icon              | an icon for the home                                                                                            |
| screenshots       | an array of path to the screenshots of the application                                                          |
| category          | the category of the application                                                                                 |
| short_description | a short description of the application                                                                          |
| long_description  | a long description of the application                                                                           |
| source            | where the files of the app can be downloaded                                                                    |
| developer         | `name` and `url` for the developer (`{"name": "Cozy", "url": "https://cozy.io"}`)                               |
| default_locale    | the locale used for the name and description fields                                                             |
| locales           | translations of the name and description fields in other locales                                                |
| langs             | list of languages tags supported by the application                                                             |
| version           | the current version number                                                                                      |
| parameters        | any json object that will be passed to the konnector on execution in the `COZY_PARAMETERS` environment variable |
| license           | [the SPDX license identifier](https://spdx.org/licenses/)                                                       |
| permissions       | a map of permissions needed by the app (see [here](permissions.md) for more details)                            |

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
  }]
}
```

## Get informations about a konnector

### GET /konnectors/:slug

## Uninstall a konnector

### DELETE /apps/:slug

#### Request

```http
DELETE /konnectors/bank101 HTTP/1.1
```

#### Response

```http
HTTP/1.1 204 No Content
```

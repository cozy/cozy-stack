[Table of contents](README.md#table-of-contents)

# Konnectors

It is possible to manage konnectors applications from the stack. The source-code of the konnector is installed on the cozy and the user can manage its execution *via*  our [job system](jobs.md) and the [konnector worker](workers.md).

## Install a konnector

## The manifest

Field          | Description
---------------|---------------------------------------------------------------------
name           | the name to display on the home
type           | the type of the konnector source ("node" is the only supported for now)
slug           | the default slug (it can be changed at install time)
icon           | an icon for the home
description    | a short description of the konnector
fields         | the list of required fields with their type
source         | where the files of the app can be downloaded
developer      | `name` and `url` for the developer
default_locale | the locale used for the name and description fields
locales        | translations of the name and description fields in other locales
version        | the current version number
license        | [the SPDX license identifier](https://spdx.org/licenses/)
permissions    | a map of permissions needed by the app (see [here](permissions.md) for more details)

For the "fields" field here is an example :
```
{
  "fields": {
    "login": "string",
    "password": "password",
    "folderPath": "path"
  }
}
```

This will allow the "My accounts" application to get the list of fields to display and their type. The list of possible types still needs to be defined.

### POST /konnectors/:slug

Install a konnector, ie download the files and put them in `/konnectors/:slug` in the virtual file system of the user, create an `io.cozy.konnectors` document, register the permissions, etc.

This endpoint is asynchronous and returns a successful return as soon as the konnector installation has started, meaning we have successfully reached the manifest and started to fetch konnector source code.

To make this endpoint synchronous, use the header `Accept: text/event-stream`. This will make a eventsource stream sending the manifest and returning when the konnector has been installed or failed.

#### Status codes

* 202 Accepted, when the konnector installation has been accepted.
* 400 Bad-Request, when the manifest of the konnector could not be processed (for instance, it is not valid JSON).
* 404 Not Found, when the manifest or the source of the konnector is not reachable.
* 422 Unprocessable Entity, when the sent data is invalid (for example, the slug is invalid or the Source parameter is not a proper or supported url)

#### Query-String

Parameter | Description
----------|------------------------------------------------------------
Source    | URL from where the app can be downloaded (only for install)

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

This endpoint is asynchronous and returns a successful return as soon as the konnector installation has started, meaning we have successfully reached the manifest and started to fetch konnector source code.

To make this endpoint synchronous, use the header `Accept: text/event-stream`. This will make a eventsource stream sending the manifest and returning when the konnector has been updated or failed.

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

* 202 Accepted, when the konnector installation has been accepted.
* 400 Bad-Request, when the manifest of the konnector could not be processed (for instance, it is not valid JSON).
* 404 Not Found, when the konnector with the specified slug was not found or when the manifest or the source of the konnector is not reachable.
* 422 Unprocessable Entity, when the sent data is invalid (for example, the slug is invalid or the Source parameter is not a proper or supported url)


## The marketplace

### GET /konnectors/registries

List konnectors in the marketplace (ie the union of the apps declared in the registries of this cozy instance).

#### Query-String

Parameter | Description
----------|------------------------------------------------------
cursor    | the name of the last application on the previous page
limit     | the maximum number of applications to show
filter[]  | a filter to apply on fields of the application
order     | order to apply to the list

#### Request

```http
GET /konnectors/registries?filter[category]=cozy HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
    "data": [{
      "id": "Trainline",
      "type": "io.cozy.registry.konnectors",
      "attributes": {
        "name": "Traineline",
        "editor": "Cozy",
        "category": "transport",
        "description": "Get all the bills from trainline",
        "versions": {
            "stable": ["0.0.1"],
            "beta": ["0.0.2-beta.1"],
            "dev": ["0.0.3-dev.b6785b387d040ceb177870c693df75d9b4c8f1a2"]
        },
        "repository": "https://github.com/cozy/cozy-konnector-trainline",
        "license": "BSD"
      }
    }, {
       // ...
    }],
    "links": {
        "next": "/konnectors/registries?filter[cursor]=photos&filter[limit]=30"
    }
}
```

### GET /konnectors/registries/:name/:version

Get informations about a konnector, for a specified version.
For examples, the permissions can change from one version to another.

#### Request

```http
GET /konnectors/registries/Trainline/0.0.1 HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "data": {
    "id": "Trainline/0.0.1",
    "type": "io.cozy.registry.versions",
    "attributes": {
      "name": "Trainline",
      "version": "0.0.1",
      "url": "http://.../0.0.1",
      "sha256": "f8d191ffc582dd1d628563c677b7d7205adfa9226445507f7a02eae26bcd23f9",
      "size": "1000",
      "created_at": "2017-07-05T07:54:40.982Z",
      "manifest": { ... }
    }
  }
}
```

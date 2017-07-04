# Registry

The registry is a place where applications metadata are stored and versioned. It can be used by a cozy 

We define an application registry as an API. This should allow us to defer the real implementation of the registry storage and allow different store implementations.


## Objects

Two type of objects are managed in the registry: applications and versions.

### Application

An application object contains the following fields:

- `name`: the application name
- `type`: the application type ("webapp" or "konnector")
- `editor`: the application editor name
- `category`: the application category
- `repository`: object with type and url of package repository
- `license`: the application license
- `versions`: a list of all versions from the stable channel
- `versionsDev`: a list of all versions from the dev channel

### Version

An application version object contains the following fields:

- `name`: the application name
- `description`: description from the package.json
- `version`: the version string
- `channel`: the channel of the version (stable or dev)
- `url`: url of the tarball containing the application at specified version
- `size`: the size of the application package (uncompressed) in bytes
- `sha256`: the sha256 checksum of the application content
- `permissions`: the permissions map contained in the manifest

## APIs: Adding to registry

These APIs can be used to add elements to the registry.

### POST /:app

This route adds an application to the registry. The content of the request should be a json object of an application.

#### Status codes

* 201 Created, when the application has been successfully added
* 409 Conflict, when an application with the same name already exists
* 400 Bad request, if the given application data is malformed (bad name, missing editor, ...)

#### Request

```http
POST /drive
X-Cozy-Registry-Key: AbCdE

{
    "name": "drive",
    "editor": "cozy",
    "description": "The drive application",
    "repository": "https://github.com/cozy/cozy-drive",
    "tags": ["foo", "bar", "baz"],
    "license": "BSD"
}
```

### POST /:app/:version

This route adds a version of an application to the registry.

The content of the manifest file extracted from the application data is used to fill the fields of the version. Before adding the application version to the registry, the registry should check the following:

    - the `manifest` file contained in the tarball should be checked and have its fields checked against the application properties
    - the application content should check the sha256 checksum

#### Status codes

* 201 Created, when the version has been successfully added to the registry
* 409 Conflict, when the version already exists
* 404 Not Found, when the application does not exist
* 412 Precondition Failed, when the sent application data is invalid (could not fetch data url, bad checksum, bad manifest in the tarball, ...)
* 400 Bad request, when the request is invalid (bad checksum encoding, bad url, ...)

#### Request

```http
POST /drive/v3.1.2
X-Cozy-Registry-Key: AbCdE

{
    "url": "https://github.com/cozy/cozy-drive/archive/v3.1.2.tar.gz",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
    "channel": "stable",
}
```

#### Response

```http
HTTP/1.1 201 Created
Content-Type: application/json
Location: http://.../v3.1.2
```

```json
{
    "version": "v3.1.2",
    "url": "http://.../v3.1.2",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
    "description": "Description of the 3.1.2 version of drive",
    "permissions": {
        "apps": {
          "description": "Required by the cozy-bar to display the icons of the apps",
          "type": "io.cozy.apps",
          "verbs": ["GET", "POST", "PUT"]
        },
        "settings": {
          "description": "Required by the cozy-bar display Claudy and to know which applications are coming soon",
          "type": "io.cozy.settings",
          "verbs": ["GET"]
        }
    }
}
```


## APIs: Querying registry

These routes define the querying part of a registry to access to the available applications and versions. These APIs are also implemented directly by the cozy-stack.

### GET /

Get the list of all applications.

A simple pagination scheme is available via the `page` and `limit` query parameter. The `filter[???]` query parameters can be used to filter by fields values.

#### Query-String

Parameter | Description
----------|------------------------------------
page      | the page of the list
limit     | the maximum number of applications to show
filter[]  | a filter to apply on fields of the applicaiton
order     | order to apply to the list

#### Request

```http
GET /?filter[category]=cozy&page=0&limit=30
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
[
    {
        "name": "drive",
        "editor": "cozy",
        "description": "The drive application",
        "versions": ["v3.1.1","v3.1.2"],
        "repository": "https://github.com/cozy/cozy-drive",
        "license": "BSD"
    },
    {
        // ...
    }
]
```

### GET /:app

Get an application object by name.

#### Request

```http
GET /drive
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
    "name": "drive",
    "editor": "cozy",
    "description": "The drive application",
    "versions": ["v3.1.1","v3.1.2"],
    "repository": "https://github.com/cozy/cozy-drive",
    "license": "BSD"
}
```

### GET /:app/:version

Get an application version.

#### Request

```http
GET /drive/v3.1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
    "name": "drive",
    "editor": "cozy",
    "description": "The drive application",
    "versions": ["v3.1.1","v3.1.2"],
    "repository": "https://github.com/cozy/cozy-drive",
    "license": "BSD"
}
```


## Attaching a cozy-stack to a registry or a list of registries

In the configuration file of a stack, a `registries` namespace is added. This namespace can contain a list of url for the different registries attached to the stack.

The stack itself implements the querying API of a registry. When querying this API, to ask for an application, the stack uses this hierarchy of registries to proxy or redirect the user.

The hierarchy can also be contextualised to specify different registries to different contexts. The `defaults` context is applied lastly.

### Examples:

```yaml
registries:
  - https://myregistry.home/
  - https://main.registry.cozy.io/
```

```yaml
# In this example, a "context1" instance will have the equivalent of the
# following list of registries:
#
#   - https://context1.registry.cozy.io/
#   - https://myregistry.home/
#   - https://main.registry.cozy.io/
#

registries:
  defaults:
    - https://myregistry.home/
    - https://main.registry.cozy.io/

  context1:
    - https://context1.registry.cozy.io/

  context2:
    - https://context2.registry.cozy.io/
```

# Authentication

For now the authentication scheme to publish an application is kept minimal and simple. Each editor is given an API key that should be present in the header of the http request when creating a application or publishing a new version.

The API key should be present in the `X-Cozy-Registry-Key` header.

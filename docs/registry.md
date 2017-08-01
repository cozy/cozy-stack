# Registry

The registry is a place where developers can submit their applications, both web apps and konnectors. The applications metadata are stored and versioned. It can be used by a cozy to list applications to be installed, and for auto-updating the applications.

We define the application registry as an API. This should allow us to defer the real implementation of the registry storage and allow different store implementations.

## Channels

We differentiate three channels of release for each application:

- stable: for stable releases
- beta: for application that can be tested in advance
- dev: for the latest releases directly from the trunk of the repository

For each of these channels, the version string has a different format which differentiate the version channel:

- stable: `X.Y.Z` where `X`, `Y` and `Z` are positive or null integers.
- beta: `X.Y.Z-beta.M` where `X`, `Y`, `Z` and `M` are positive or null integers
- dev: `X.Y.Z-dev.checksum` where `X`, `Y` and `Z` are positive or null integers and `checksum` is a unique identifier of the dev release (typically a shasum of the git commit)

## Objects

Two types of objects are managed in the registry: applications and versions.

### Application

An application described a specific package. It is linked to multiple versions (releases) of the application.

An application object is **mutable**.

An application object contains the following fields:

- `name`: the application name
- `type`: the application type ("webapp" or "konnector")
- `editor`: the application editor name
- `description`: object containing the description description of the application in multiple languages
- `category`: the application category
- `repository`: object with type and URL of package repository
- `tags`: list of tags associated with the application
- `versions`: an object containing all the channels versions

Example:

```json
{
    "name": "drive",
    "type": "webapp",
    "editor": "cozy",
    "description": {
        "en": "The drive application",
        "fr": "L'application drive gestionnaire de fichier"
    },
    "repository": "https://github.com/cozy/cozy-drive",
    "tags": ["foo", "bar", "baz"],
    "versions": {
        "stable": ["3.1.1"],
        "beta": ["3.1.1-beta.1"],
        "dev": ["3.1.1-dev.7a8354f74b50d7beead7719252a18ed45f55d070"]
    }
}
```

### Version

A version object describe a specific release of an application.

A version object is **immutable**.

An application version object contains the following fields:

- `name`: the application name
- `type`: the application type (webapp, konnector, ...)
- `manifest`: the [entire](./apps.md#the-manifest) [manifest](./konnectors.md#the-manifest) defined in the package
- `created_at`: date of the release creation
- `url`: URL of the tarball containing the application at specified version
- `size`: the size of the application package (uncompressed) in bytes as string
- `sha256`: the sha256 checksum of the application content
- `tar_prefix`: optional tar prefix directory specified to properly extract the application content

The version string should follow the channels rule.

Example:

```json
{
    "name": "drive",
    "type": "webapp",
    "version": "3.1.2",
    "created_at": "2017-07-05T07:54:40.982Z",
    "url": "http://.../3.1.2",
    "size": "1000",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
    "manifest": { /* ... */ }
}
```

## APIs: Adding to registry

These APIs can be used to add elements to the registry.

### POST /apps/:app

This route adds or modify an application to the registry. The content of the request should be a json object of an application.

#### Status codes

* 201 Created, when the application has been successfully added
* 409 Conflict, when an application with the same name already exists
* 400 Bad request, if the given application data is malformed (bad name, missing editor, ...)

#### Request

```http
POST /apps/drive HTTP/1.1
Authorization: AbCdE
```

```json
{
    "name": "drive",
    "editor": "cozy",
    "description": {
        "en": "The drive application"
    },
    "repository": "https://github.com/cozy/cozy-drive",
    "tags": ["foo", "bar", "baz"]
}
```

### POST /apps/:app/:version

This route adds a version of an application to the registry to the specified channel (stable, beta or dev).

The content of the manifest file extracted from the application data is used to fill the fields of the version. Before adding the application version to the registry, the registry should check the following:

- the `manifest` file contained in the tarball should be checked and have its fields checked against the application properties
- the application content should check the sha256 checksum

#### Status codes

* 201 Created, when the version has been successfully added to the registry
* 409 Conflict, when the version already exists
* 404 Not Found, when the application does not exist
* 412 Precondition Failed, when the sent application data is invalid (could not fetch data URL, bad checksum, bad manifest in the tarball...)
* 400 Bad request, when the request is invalid (bad checksum encoding, bad URL...)

#### Request

Request to add a stable release:

```http
POST /apps/drive/3.1.2 HTTP/1.1
Authorization: AbCdE
```

```json
{
    "url": "https://github.com/cozy/cozy-drive/archive/v3.1.2.tar.gz",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f"
}
```

Request to add a development release:

```http
POST /apps/drive/3.1.2-dev.7a1618dff78ba445650f266bbe334cbc9176f03a HTTP/1.1
Authorization: AbCdE
```

```json
{
    "url": "https://github.com/cozy/cozy-photos-v3/archive/7a1618dff78ba445650f266bbe334cbc9176f03a.zip",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f"
}
```

#### Response

```http
HTTP/1.1 201 Created
Content-Type: application/json
Location: http://.../3.1.2
```

```json
{
    "name": "drive",
    "type": "webapp",
    "version": "3.1.2-dev.7a1618dff78ba445650f266bbe334cbc9176f03a",
    "created_at": "2017-07-05T07:54:40.982Z",
    "url": "http://.../7a1618dff78ba445650f266bbe334cbc9176f03a.zip",
    "size": "1000",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
    "manifest": { /* ... */ }
}
```


## APIs: Querying registry

These routes define the querying part of a registry to access to the available applications and versions. These APIs are also implemented directly by the cozy-stack.

### GET /apps

Get the list of all applications.

A pagination scheme is available via the `limit` and `cursor` query parameter. The `filter[???]` query parameters can be used to filter by fields values.

#### Query-String

Parameter | Description
----------|------------------------------------------------------
cursor    | the name of the last application on the previous page
limit     | the maximum number of applications to show
filter[]  | a filter to apply on fields of the application
order     | order to apply to the list

#### Request

```http
GET /apps?filter[category]=cozy&page=0 HTTP/1.1
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
        "type": "webapp",
        "editor": "cozy",
        "category": "files",
        "description": "The drive application",
        "versions": {
            "stable": ["3.1.1"],
            "beta": ["3.1.1-beta.1"],
            "dev": ["3.1.1-dev.7a8354f74b50d7beead7719252a18ed45f55d070"]
        },
        "repository": "https://github.com/cozy/cozy-drive",
        "license": "BSD"
    },
    {
        // ...
    }
]
```

### GET /apps/:app

Get an application object by name.

#### Request

```http
GET /apps/drive HTTP/1.1
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
    "repository": "https://github.com/cozy/cozy-drive",
    "tags": ["foo", "bar", "baz"],
    "versions": {
        "stable": ["3.1.1"],
        "beta": ["3.1.1-beta.1"],
        "dev": ["3.1.1-dev.7a8354f74b50d7beead7719252a18ed45f55d070"]
    }
}
```

### GET /apps/:app/:version

Get an application version.

#### Request

```http
GET /apps/drive/3.1.1 HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
    "name": "drive",
    "type": "webapp",
    "version": "3.1.1",
    "url": "http://.../3.1.1",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
    "size": "1000",
    "created_at": "2017-07-05T07:54:40.982Z",
    "manifest": { /* ... */ }
}
```

### GET /apps/:app/:channel/latest

Get the latest version available on the specified channel.

#### Request

```http
GET /apps/drive/dev/latest HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
    "name": "drive",
    "type": "webapp",
    "version": "3.1.1",
    "url": "http://.../3.1.1",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
    "size": "1000",
    "created_at": "2017-07-05T07:54:40.982Z",
    "manifest": { /* ... */ }
}
```

## Attaching a cozy-stack to a registry or a list of registries

In the configuration file of a stack, a `registries` namespace is added. This namespace can contain a list of URL for the different registries attached to the stack.

The stack itself implements the querying API of a registry. When querying this API, to ask for an application, the stack uses this hierarchy of registries to proxy or redirect the user.

The hierarchy can also be contextualised to specify different registries to different contexts. The `default` context is applied lastly.

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
  context1:
    - https://context1.registry.cozy.io/

  context2:
    - https://context2.registry.cozy.io/

  default:
    - https://myregistry.home/
    - https://main.registry.cozy.io/
```

# Authentication

The definition of the authentication API is not finished yet. It will be released when the first implementation will come out.

For now the authentication scheme to publish an application will be kept minimal and simple. Each editor will be given an API key that should be present in the header of the http request when creating a application or publishing a new version.

The API key should be present in the `Authorization` header.

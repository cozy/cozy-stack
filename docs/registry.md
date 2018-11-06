# Apps registry

The apps registry is a place where developers can submit their applications,
both web apps and konnectors. The applications metadata are stored and
versioned. It can be used by a cozy to list applications to be installed, and
for auto-updating the applications.

We define the applications registry as an API. This should allow us to defer the
real implementation of the registry storage and allow different store
implementations.

The stack itself implement the
[querying part of the registry API](#apis-querying-registry), proxying the
request to
[the registries attached to the instance](#attaching-a-cozy-stack-to-a-registry-or-a-list-of-registries).

## Publishing on our official registries

In order for you to publish on our official registries, please follow
[this howto](./registry-publish.md) describing how to obtain a token and
parameter you repository to automatically publish versions.

## Channels

We differentiate three channels of release for each application:

-   stable: for stable releases
-   beta: for application that can be tested in advance
-   dev: for the latest releases directly from the trunk of the repository

For each of these channels, the version string has a different format which
differentiate the version channel:

-   stable: `X.Y.Z` where `X`, `Y` and `Z` are positive or null integers.
-   beta: `X.Y.Z-beta.M` where `X`, `Y`, `Z` and `M` are positive or null
    integers
-   dev: `X.Y.Z-dev.checksum` where `X`, `Y` and `Z` are positive or null
    integers and `checksum` is a unique identifier of the dev release (typically
    a shasum of the git commit)

## Version order

TLDR: 1.0.0-dev._ < 1.0.0 and 1.0.0-beta._ < 1.0.0, make sure you upgrade your
app version after publishing stable.

The order used to determine the latest version of a channel is the following:

    - `1.0.0-dev.*  < 1.0.0 (dev < stable)`
    - `1.0.0-beta.* < 1.0.0 (beta < stable)`
    - `1.0.0-beta.1 < 1.0.0-beta.2`

To order beta and dev releases, we apply a sort by their creation date.

## Objects

Two types of objects are managed in the registry: applications and versions.

### Application

An application described a specific package. It is linked to multiple versions
(releases) of the application.

An application object is **mutable**.

An application object contains the following fields:

-   `slug`: the application slug (unique)
-   `type`: the application type ("webapp" or "konnector")
-   `editor`: the application editor name
-   `versions`: an object containing all the channels versions
-   `latest_version`: the latest available version
-   `maintenance_activated`: boolean, true when the maintenance mode is
    activated on the application
-   `maintenance_options`: present only if `maintenance_activated` is true,
    object with the following fields:
    -   `flag_infra_maintenance`: bool, true iff the maintenance is internal to
        the cozy infrastructure
    -   `flag_short_maintenance`: bool, true iff the maintenance is a short
        maintenance, waiting for a correction on our side
    -   `flag_disallow_manual_exec`: bool, true iff the maintenance will
        disallow the execution on the application, even when manually executed
    -   `messages`: a list of localized messages containing a short and long
        information messages explaining the maintenance state
-   `label`: integer for a confidence grade from 0 to 5 (A to F), labelling the
    application from a user privacy standpoint. It is calculated from the
    `data_usage_commitment` and `data_usage_commitment_by` fields.
-   `data_usage_commitment`: specify a technical commitment from the application
    editor:
    -   `user_ciphered`: technical commitment that the user's data is encrypted
        and can only be known by him.
    -   `user_reserved`: commitment that the data is only used for the user, to
        directly offer its service.
    -   `none`: no commitment
-   `data_usage_commitment_by`: specify what entity is taking the commitment:
    -   `cozy`: the commitment is taken by cozy
    -   `editor`: the commitment is taken by the application's editor
    -   `none`: no commitment is taken

Example:

```json
{
    "slug": "drive",
    "type": "webapp",
    "editor": "cozy",
    "versions": {
        "stable": ["3.1.1"],
        "beta": ["3.1.1-beta.1"],
        "dev": ["3.1.1-dev.7a8354f74b50d7beead7719252a18ed45f55d070"]
    },
    "latest_version": {
        /* */
    }
}
```

### Version

A version object describe a specific release of an application.

A version object is **immutable**.

An application version object contains the following fields:

-   `slug`: the application slug
-   `type`: the application type (webapp, konnector, ...)
-   `manifest`: the [entire](./apps.md#the-manifest)
    [manifest](./konnectors.md#the-manifest) defined in the package
-   `created_at`: date of the release creation
-   `url`: URL of the tarball containing the application at specified version
-   `size`: the size of the application package (uncompressed) in bytes as
    string
-   `sha256`: the sha256 checksum of the application content
-   `tar_prefix`: optional tar prefix directory specified to properly extract
    the application content

The version string should follow the channels rule.

Example:

```json
{
    "slug": "drive",
    "type": "webapp",
    "version": "3.1.2",
    "created_at": "2017-07-05T07:54:40.982Z",
    "url": "http://.../3.1.2",
    "size": "1000",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
    "manifest": {
        /* ... */
    },
    "maintenance_activated": true,
    "maintenance_options": {
        "flag_infra_maintenance": true,
        "flag_short_maintenance": false,
        "flag_disallow_manual_exec": true,
        "messages": {
            "en": {
                "long_message": "The app is currently in maintenance because of ....",
                "short_message": "The app is currently in maintenance"
            },
            "fr": {
                "long_message": "L'application est en cours de maintenance à cause de ...",
                "short_message": "L'application est en cours de maintenance"
            }
        }
    }
}
```

## APIs: Adding to registry

These APIs can be used to add elements to the registry.

### POST /registry/:app

This route adds or modify an application to the registry. The content of the
request should be a json object of an application.

#### Status codes

-   201 Created, when the application has been successfully added
-   409 Conflict, when an application with the same slug already exists
-   400 Bad request, if the given application data is malformed (bad slug,
    missing editor, ...)

#### Request

```http
POST /registry/drive HTTP/1.1
Authorization: Token AbCdE
```

```json
{
    "slug": "drive",
    "editor": "cozy",
    "name": {
        "en": "Drive",
        "fr": "Drive"
    },
    "description": {
        "en": "The drive application"
    },
    "repository": "https://github.com/cozy/cozy-drive",
    "tags": ["foo", "bar", "baz"]
}
```

### POST /registry/:app/:version or POST /registry/:app/versions

This route adds a version of an application to the registry to the specified
channel (stable, beta or dev).

The content of the manifest file extracted from the application data is used to
fill the fields of the version. Before adding the application version to the
registry, the registry should check the following:

-   the `manifest` file contained in the tarball should be checked and have its
    fields checked against the application properties
-   the application content should check the sha256 checksum

Fields of the object sent to this request:

-   **`url`**: the url where the application tarball is stored
-   **`sha256`**: the sha256 checksum of the tarball
-   **`version`**: the version value (should match the one in the manifest)
-   `parameters?`: an optional json value (any) that will override the
    `parameters` field of the manifest
-   `icon?`: an optional path to override the `icon` field of the manifest
-   `screenshots?`: and optional array of path to override the `screenshots`
    field of the manifest

#### Status codes

-   201 Created, when the version has been successfully added to the registry
-   409 Conflict, when the version already exists
-   404 Not Found, when the application does not exist
-   412 Precondition Failed, when the sent application data is invalid (could
    not fetch data URL, bad checksum, bad manifest in the tarball...)
-   400 Bad request, when the request is invalid (bad checksum encoding, bad
    URL...)

#### Request

Request to add a stable release:

```http
POST /registry/drive/3.1.2 HTTP/1.1
Authorization: Token AbCdE
```

```json
{
    "version": "3.1.2",
    "url": "https://github.com/cozy/cozy-drive/archive/v3.1.2.tar.gz",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f"
}
```

Request to add a development release:

```http
POST /registry/drive/3.1.2-dev.7a1618dff78ba445650f266bbe334cbc9176f03a HTTP/1.1
Authorization: Token AbCdE
```

```json
{
    "version": "3.1.2-dev.7a1618dff78ba445650f266bbe334cbc9176f03a",
    "url": "https://github.com/cozy/cozy-photos-v3/archive/7a1618dff78ba445650f266bbe334cbc9176f03a.zip",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f"
}
```

Request to add a version with optional parameters:

```http
POST /registry/drive/3.1.2 HTTP/1.1
Authorization: Token AbCdE
```

```json
{
    "version": "3.1.2",
    "url": "https://github.com/cozy/cozy-photos-v3/archive/7a1618dff78ba445650f266bbe334cbc9176f03a.zip",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
    "parameters": { "foo": "bar", "baz": 123 }
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
    "slug": "drive",
    "type": "webapp",
    "version": "3.1.2-dev.7a1618dff78ba445650f266bbe334cbc9176f03a",
    "created_at": "2017-07-05T07:54:40.982Z",
    "url": "http://.../7a1618dff78ba445650f266bbe334cbc9176f03a.zip",
    "size": "1000",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
    "manifest": {
        /* ... */
    }
}
```

## APIs: Querying registry

These routes define the querying part of a registry to access to the available
applications and versions. These APIs are also implemented directly by the
cozy-stack.

### GET /registry

Get the list of all applications.

A pagination scheme is available via the `limit` and `cursor` query parameter.
The `filter[???]` query parameters can be used to filter by fields values.

Filtering is allowed on the following fields:

-   `type`
-   `editor`
-   `category`
-   `tags`

Filtering is allowed on multiple tags with the `,` separator. For example:
`filter[tags]=foo,bar` will match the applications with both `foo` and `bar` as
tags.

Sorting is allowed on the following fields:

-   `slug`
-   `type`
-   `editor`
-   `category`
-   `created_at`
-   `updated_at`

#### Query-String

| Parameter            | Description                                              |
| -------------------- | -------------------------------------------------------- |
| cursor               | the cursor of the last application on the previous page  |
| limit                | the maximum number of applications to show               |
| filter[]             | a filter to apply on fields of the application           |
| sort                 | name of the field on which to apply the sort of the list |
| versionsChannel      | the channel from which we select the latest version      |

#### Request

```http
GET /registry?filter[category]=main&limit=20&sort=slug&latest&latestVersionChannel=beta HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
    "data": [
        {
            "slug": "drive",
            "type": "webapp",
            "editor": "cozy",
            "versions": {
                "stable": ["3.1.1"],
                "beta": ["3.1.1-beta.1"],
                "dev": ["3.1.1-dev.7a8354f74b50d7beead7719252a18ed45f55d070"]
            },
            "latest_version": {
                "slug": "drive",
                "type": "webapp",
                "version": "3.1.1",
                "url": "http://.../3.1.1",
                "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
                "size": "1000",
                "created_at": "2017-07-05T07:54:40.982Z",
                "manifest": {
                    /* ... */
                }
            }
        },
        {
            // ...
        }
    ],
    "meta": {
        "count": 2,
        "next_cursor": "..."
    }
}
```

### GET /registry/:app

Get an application object by slug.

#### Request

```http
GET /registry/drive HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
    "slug": "drive",
    "editor": "cozy",
    "latest_version": {
        "slug": "drive",
        "type": "webapp",
        "version": "3.1.1",
        "url": "http://.../3.1.1",
        "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
        "size": "1000",
        "created_at": "2017-07-05T07:54:40.982Z",
        "manifest": {
            /* ... */
        }
    },
    "versions": {
        "stable": ["3.1.1"],
        "beta": ["3.1.1-beta.1"],
        "dev": ["3.1.1-dev.7a8354f74b50d7beead7719252a18ed45f55d070"]
    }
}
```

### GET /registry/:app/icon

Get the current application icon.

#### Request

```http
GET /registry/drive/icon HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: image/svg+xml

<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64">
  <g fill="none" fill-rule="evenodd"></g>
</svg>
```

### GET /registry/:app/screenshots/:filename

Get the screenshot with the specified filename from the field `screenshots` of
the application.

#### Request

```http
GET /registry/drive/screenshots/screen1.jpg HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: image/jpeg

...
```

### GET /registry/:app/:version

Get an application version.

#### Request

```http
GET /registry/drive/3.1.1 HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
    "slug": "drive",
    "type": "webapp",
    "version": "3.1.1",
    "url": "http://.../3.1.1",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
    "size": "1000",
    "created_at": "2017-07-05T07:54:40.982Z",
    "manifest": {
        /* ... */
    }
}
```

### GET /registry/:app/:channel/latest

Get the latest version available on the specified channel.

#### Request

```http
GET /registry/drive/dev/latest HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
    "slug": "drive",
    "type": "webapp",
    "version": "3.1.1",
    "url": "http://.../3.1.1",
    "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
    "size": "1000",
    "created_at": "2017-07-05T07:54:40.982Z",
    "manifest": {
        /* ... */
    }
}
```

### GET /registry/maintenance

Get the list of applications with maintenance mode activated.

#### Request

```http
GET /registry/maintenance HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
[
    {
        "slug": "drive",
        "type": "webapp",
        "version": "3.1.1",
        "url": "http://.../3.1.1",
        "sha256": "466aa0815926fdbf33fda523af2b9bf34520906ffbb9bf512ddf20df2992a46f",
        "size": "1000",
        "created_at": "2017-07-05T07:54:40.982Z",
        "manifest": {
            /* ... */
        },
        "maintenance_activated": true,
        "maintenance_options": {
            "flag_infra_maintenance": true,
            "flag_short_maintenance": false,
            "flag_disallow_manual_exec": true
        }
    }
]
```

## Attaching a cozy-stack to a registry or a list of registries

In the configuration file of a stack, a `registries` namespace is added. This
namespace can contain a list of URL for the different registries attached to the
stack.

The stack itself implements the querying API of a registry. When querying this
API, to ask for an application, the stack uses this hierarchy of registries to
proxy or redirect the user.

The hierarchy can also be contextualised to specify different registries to
different contexts. The `default` context is applied lastly.

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
#   - https://registry.cozy.io/
#

registries:
    context1:
        - https://context1.registry.cozy.io/

    context2:
        - https://context2.registry.cozy.io/

    default:
        - https://myregistry.home/
        - https://registry.cozy.io/
```

# Authentication

The authentication is based on a token that allow you to publish applications
and versions with for one specific editor name. This token is base64 encoded.

In order to receive this token, please take a look at the page on
[publication on the registry](./registry-publish.md).

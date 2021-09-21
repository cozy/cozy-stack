[Table of contents](README.md#table-of-contents)

# Admin

## Introduction

An admin API is available on the stack. It offers several endpoints to interact
with your cozy-stack installation (E.g. interacting with instances, generating tokens, ...).

:warning: Use the admin API only if you know what you are doing. The admin API
provides a basic authentication, you **must** protect these endpoints as they
are very powerful.

The default port for the admin endpoints is `6060`. If you want to customize the parameters, please see the [config file documentation page](config.md).


## Instance

### GET /instances

Returns the list of all instances. By default, there is no pagination, but it
is possible to add a `page[limit]` parameter in the query-string to paginate (
cf [JSON-API pagination](./jsonapi.md#pagination)). A `page[skip]` parameter in
the query-string is also supported, but CouchDB may be slow on requests with a
skip on large collections.

#### Request

```http
GET /instances HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
    "data": [
        {
            "type": "instances",
            "id": "3af6ed68a6d9146b3529d2584a001d98",
            "attributes": {
                "domain": "alice.cozy.localhost:8080",
                "prefix": "cozy7d3d5947e7f3b0c674d1b8644646348e",
                "locale": "fr",
                "context": "dev",
                "onboarding_finished": true,
                "indexes_version": 30
            },
            "meta": {
                "rev": "1-32c855c989e8f6def0bc0cc417d8b3b4"
            },
            "links": {
                "self": "/instances/3af6ed68a6d9146b3529d2584a001d98"
            }
        },
        {
            "type": "instances",
            "id": "3af6ed68a6d9146b3529d2584a01d557",
            "attributes": {
                "domain": "bob.cozy.localhost:8080",
                "prefix": "cozybf682065ca3c7d64f2dafc6cc12fe702",
                "locale": "fr",
                "context": "dev",
                "onboarding_finished": true,
                "indexes_version": 30
            },
            "meta": {
                "rev": "1-ab6f77dbfdb3aab5b70b022e37fe231f"
            },
            "links": {
                "self": "/instances/3af6ed68a6d9146b3529d2584a01d557"
            }
        }
    ],
    "meta": {
        "count": 2
    }
}
```

### PATCH /instances/:domain

This route can be used to change an instance (email, locale, disk quota, ToS,
etc.)

**Note** a special parameter `FromCloudery=true` in the query string can be
used to tell the stack to not call the cloudery if the email or public name has
changed, since the change is already coming from the cloudery.

#### Request

```http
PATCH /instances/john.mycozy.cloud?Email=john@example.com&FromCloudery=true HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
```

```json
{
  "data": {
    "type": "instances",
    "id": "0dc76ad9b1cf3a979b916b3155001830",
    "attributes": {
      "domain": "john.mycozy.cloud",
      "prefix": "cozy1a4e1aabf424a194d7daf946d7b1337d",
      "locale": "fr",
      "context": "cozy",
      "onboarding_finished": true,
      "indexes_version": 31,
      "passphrase_hash": "c2NyeXB0JDMyNzY4JDgkMSQyYjMzMTU1YTY5OTdjYzM2ZjQyYjk1MWM0MWU4ZWVkYSRlODA4NTY2ODQ5OTdkOWNmNzc1ZGEzMjdlYWMwOTgyNTMwMTM3NTJjYTMxMTdhYTIyYTIxODI0NzBmODhjYjdl",
      "session_secret": "eyG2l+G1xO38WyD1GfqYkgSU/T4rnti+JzOwj6haHpM8PSMvzkGu/CSH0mpXUuuCNVbjEXc+hRwGMJ8lTKqs+w==",
      "oauth_secret": "tnr6V8jDK27CDVpzNiOOAJZs+5wrvGyNyJxIc/BJ6O87i2eJX4LCzblDFyDbVv/B7qV7HA9/Fc+Agon2gHQg8x0E0zzfGizbFeWt+KPk7UrZNd4sZJ81oWNNd9BrJ2+eKXDmZeYBI0AwUSykyr7iOIpB5jXaIvfOQvH7EYwtKLg=",
      "cli_secret": "dcLa1VqcoI4eNE7nBrFzhJ9w6rRLlAMESl3PAEqr+IDE29OeN3uyhzLhxPlk0b9rkc0yvozQc/AFttZxyqH/DDYa6rrJyrf91gddtwSfka1pJVss+/DiFaghyWJzEbffBs78X3swA2gSJNu0eGDKdFVY7q8iLT4JpfXy+GPzdLk="
    },
    "meta": {
      "rev": "1-1c4efe4196191469a65b2a3e0898db61"
    },
    "links": {
      "self": "/instances/0dc76ad9b1cf3a979b916b3155001830"
    }
  }
}
```


### GET /instances/with-app-version/:slug/:version

Returns all the instances using slug/version pair

#### Request

```http
GET /instances/with-app-version/drive/1.0.0 HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
    "instances": [
        "alice.cozy.localhost",
        "bob.cozy.localhost",
        "zoe.cozy.localhost"
    ]
}
```

### POST /instances/:domain/fixers/content-mismatch

Fixes the 64k (or multiple) content mismatch files of an instance

#### Request

```http
POST /instances/alice.cozy.localhost/fixers/content-mismatch HTTP/1.1
Content-Type: application/json
```

```json
{
  "dry_run": true
}
```

The `dry_run` (default to `true`) body parameter tells if the request is a
dry-run or not.

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "dry_run": true,
  "updated": [
    {
      "filepath": "/file64.txt",
      "id": "3c79846513e81aee78ab30849d006550",
      "created_at": "2019-07-30 15:05:27.268876334 +0200 CEST",
      "updated_at": "2019-07-30 15:05:27.268876334 +0200 CEST"
    }
  ],
  "removed": [
    {
      "filepath": "/.cozy_trash/file64.txt-corrupted",
      "id": "3c79846513e81aee78ab30849d001f98",
      "created_at": "2019-07-30 10:18:28.826400117 +0200 CEST",
      "updated_at": "2019-07-30 14:32:29.862882247 +0200 CEST"
    }
  ],
  "domain": "alice.cozy.localhost"
}
```

### POST /instances/:domain/fixers/orphan-account

Delete the accounts which are not linked to a konnector

#### Request

```http
POST /instances/alice.cozy.localhost/fixers/orphan-account HTTP/1.1
```


## Contexts

### GET /instances/contexts

This endpoint returns the list of the contexts, with their name, config,
registries, office server, and cloudery endpoints.

#### Request

```
GET /instances/contexts HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
```

```json
[
  {
    "config": {
      "claudy_actions": [
        "desktop",
        "mobile",
        "support"
      ],
      "debug": true,
      "features": [
        {
          "foo": "bar"
        },
        {
          "baz": [
            "qux",
            "quux"
          ]
        }
      ],
      "help_link": "https://forum.cozy.io/",
      "noreply_address": "noreply@cozy.beta",
      "noreply_name": "My Cozy Beta",
      "sharing_domain": "cozy.localhost"
    },
    "context": "dev",
    "registries": [
      "https://apps-registry.cozycloud.cc/"
    ],
    "office": {
      "OnlyOfficeURL": "https://documentserver.cozycloud.cc/",
      "InboxSecret": "inbox_secret",
      "OutboxSecret": "outbox_secret"
    },
    "cloudery_endpoint": ""
  }
]
```

### GET /instances/contexts/:name

This endpoint returns the config of a given context.

#### Request

```
GET /instances/contexts/dev HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
```

```json
{
  "config": {
    "claudy_actions": [
      "desktop",
      "mobile",
      "support"
    ],
    "debug": true,
    "features": [
      {
        "foo": "bar"
      },
      {
        "baz": [
          "qux",
          "quux"
        ]
      }
    ],
    "help_link": "https://forum.cozy.io/",
    "noreply_address": "noreply@cozy.beta",
    "noreply_name": "My Cozy Beta",
    "sharing_domain": "cozy.localhost"
  },
  "context": "dev",
  "registries": [
    "https://apps-registry.cozycloud.cc/"
  ],
  "office": {
    "OnlyOfficeURL": "https://documentserver.cozycloud.cc/",
    "InboxSecret": "inbox_secret",
    "OutboxSecret": "outbox_secret"
  },
  "cloudery_endpoint": ""
}
```


## Checkers

### GET /instances/:domain/fsck

This endpoint can be use to check the VFS of a given instance. It accepts three
possible parameters in the query-string:

- `IndexIntegrity=true` to check only the integrity of the data in CouchDB
- `FilesConsistency` to check the consistency between CouchDB and Swift
- `FailFast` to abort on the first error.

It will returns a `200 OK`, except if the instance is not found where the code
will be `404 Not Found` (a `5xx` can also happen in case of server errors like
CouchDB not available). The format of the response will be one JSON per line,
and each JSON represents an error.

#### Request

```http
GET /instances/alice.cozy.localhost/fsck HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
```

```json
{"type":"index_orphan_tree","dir_doc":{"type":"directory","_id":"34a61c6ceb38075fe971cc6a3263659f","_rev":"2-94ca3acfebf927cb231d125c57f85bd7","name":"Photos","dir_id":"45496c5c442dabecae87de3d73008ec4","created_at":"2020-12-15T18:23:21.498323965+01:00","updated_at":"2020-12-15T18:23:21.498323965+01:00","tags":[],"path":"/Photos","cozyMetadata":{"doctypeVersion":"1","metadataVersion":1,"createdAt":"2020-12-15T18:23:21.498327603+01:00","updatedAt":"2020-12-15T18:23:21.498327603+01:00","createdOn":"http://alice.cozy.localhost:8080/"},"size":"0","is_dir":true,"is_orphan":true,"has_cycle":false},"is_file":false,"is_version":false}
{"type":"index_missing","file_doc":{"type":"file","name":"Photos","dir_id":"","created_at":"2020-12-15T18:23:21.527308795+01:00","updated_at":"2020-12-15T18:23:21.527308795+01:00","tags":null,"path":"/Photos","size":"4096","mime":"application/octet-stream","class":"files","executable":true,"is_dir":false,"is_orphan":false,"has_cycle":false},"is_file":true,"is_version":false}
```

### POST /instances/:domain/checks/triggers

This endpoint will check if no trigger has been installed twice (or more).

#### Request

```http
POST /instances/alice.cozy.localhost/checks/triggers HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
[
  {
    "_id": "45496c5c442dabecae87de3d7300666f",
    "arguments": "io.cozy.files:CREATED,UPDATED,DELETED:image:class",
    "debounce": "",
    "other_id": "34a61c6ceb38075fe971cc6a3263895f",
    "trigger": "@event",
    "type": "duplicate",
    "worker": "thumbnail"
  }
]
```

### POST /instances/:domain/checks/shared

This endpoint will check that the io.cozy.shared documents have a correct
revision tree (no generation smaller for a children than its parent).

#### Request

```http
POST /instances/alice.cozy.localhost/checks/shared HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
[
  {"_id":"io.cozy.files/fd1706de234d17d1ac2fe560051a2aae","child_rev":"1-e19947b4f9bfb273bc8958ae932ae4c7","parent_rev":"2-4f82af35577dbc9b686dd447719e4835","type":"invalid_revs_suite"},
  {"_id":"io.cozy.files/fd9bef5df406f5b150f302b8c5b3f5f0","child_rev":"7-05bb459e0ac5450c17df79ed1f13afa1","parent_rev":"8-07b4cafef3c2e74e698ee4a04d1874c2","type":"invalid_revs_suite"}
]
```

### POST /instances/:domain/checks/sharings

#### Request

```http
POST /instances/alice.cozy.localhost/checks/sharings HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
[
  {"id":"314d69d7ebaed0a1870cca67f4433390","trigger":"track","trigger_id":"314d69d7ebaed0a1870cca67f4d75e41","type":"trigger_on_inactive_sharing"},
  {"id":"314d69d7ebaed0a1870cca67f4433390","trigger":"replicate","trigger_id":"314d69d7ebaed0a1870cca67f4d75e41","type":"trigger_on_inactive_sharing"},
  {"id":"314d69d7ebaed0a1870cca67f4433390","trigger":"upload","trigger_id":"314d69d7ebaed0a1870cca67f4d75e41","type":"trigger_on_inactive_sharing"},
  {"id":"314d69d7ebaed0a1870cca67f4433390","member":0,"status":"revoked","type":"invalid_member_status"},
  {"id":"314d69d7ebaed0a1870cca67f4433390","nb_members":0,"owner":false,"type":"invalid_number_of_credentials"}
]
```


## Konnectors

### GET /konnectors/maintenance

#### Request

```http
GET /konnectors/maintenance HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "meta": {
    "count": 1
  },
  "data": [
    {
      "type": "io.cozy.konnectors.maintenance",
      "attributes": {
        "maintenance_activated": true,
        "maintenance_options": {
          "flag_disallow_manual_exec": false,
          "flag_infra_maintenance": true,
          "flag_short_maintenance": true,
          "messages": {
            "fr": {
              "long_message": "Bla bla bla",
              "short_message": "Bla"
            }
          }
        },
        "slug": "ameli",
        "type": "konnector"
      }
    }
  ]
}
```

### PUT /konnectors/maintenance/:slug

#### Request

```http
PUT /konnectors/maintenance/ameli HTTP/1.1
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "attributes": {
      "flag_short_maintenance": true,
      "flag_disallow_manual_exec": false,
      "messages": {
        "fr": {
          "long_message": "Bla bla bla",
          "short_message": "Bla"
        },
        "en": {
          "long_message": "Yadi yadi yada",
          "short_message": "Yada"
        }
      }
    }
  }
}
```

**Note:** the `flag_infra_maintenance` will always be set to true with this
endpoint.

#### Response

```http
HTTP/1.1 204 No Content
```

### DELETE /konnectors/maintenance/:slug

#### Request

```http
DELETE /konnectors/maintenance/ameli HTTP/1.1
```

#### Response

```http
HTTP/1.1 204 No Content
```

## Swift

### GET /swift/layouts

Count swift layouts by type

#### Request

```http
GET /swift/layouts HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "total": 3,
  "unknown": {
    "counter": 0
  },
  "v1": {
    "counter": 1
  },
  "v2a": {
    "counter": 0
  },
  "v2b": {
    "counter": 0
  },
  "v3a": {
    "counter": 2
  },
  "v3b": {
    "counter": 4
  }
}
```

The `show_domains=true` query parameter provides the domain names if needed


#### Request

```http
GET /swift/layouts?show_domains=true HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "total": 3,
  "unknown": {
    "counter": 0
  },
  "v1": {
    "counter": 1,
    "domains": [
      "bob.cozy.localhost:8081"
    ]
  },
  "v2a": {
    "counter": 0
  },
  "v2b": {
    "counter": 0
  },
  "v3a": {
    "counter": 2,
    "domains": [
      "alice.cozy.localhost:8081",
      "ru.cozy.localhost:8081"
    ]
  },
  "v3b": {
    "counter": 4,
    "domains": [
      "foo.cozy.localhost:8081",
      "bar.cozy.localhost:8081",
      "baz.cozy.localhost:8081",
      "foobar.cozy.localhost:8081"
    ]
  }
}
```

### GET /swift/vfs/:object

Retrieves a Swift object

#### Request

```http
GET /swift/vfs/67a88b22520680b1fae840%2F9a8a0%2F18d02%2FiYbkfuCDEMaVoIXg HTTP/1.1
Host: alice.cozy.localhost
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: text/plain
```

```text
"foobar"
```

### PUT /swift/vfs/:object

Put an object in Swift

#### Request

```http
PUT /swift/vfs/67a88b22520680b1fae840%2F9a8a0%2F18d02%2FiYbkfuCDEMaVoIXg HTTP/1.1
Host: alice.cozy.localhost
Content-Type: text/plain
```

```text
"this is my content"
```

### DELETE /swift/vfs/:object

Removes an object from Swift

#### Request

```http
DELETE /swift/vfs/67a88b22520680b1fae840%2F9a8a0%2F18d02%2FiYbkfuCDEMaVoIXg HTTP/1.1
Host: alice.cozy.localhost
```

### GET /swift/vfs

List Swift objects of an instance

#### Request

```http
GET /swift/vfs HTTP/1.1
Host: alice.cozy.localhost
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "objects_names": [
    "67a88b22520680b1fae840/9a8a0/17264/AxfGhAiWVRhPufKK",
    "67a88b22520680b1fae840/9a8a0/18d02/iYbkfuCDEMaVoIXg",
    "thumbs/67a88b22520680b1fae840/9a8a0/17264-large",
    "thumbs/67a88b22520680b1fae840/9a8a0/17264-medium",
    "thumbs/67a88b22520680b1fae840/9a8a0/17264-small"
  ]
}
```

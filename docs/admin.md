# Admin

## Introduction

An admin API is available on the stack. It offers several endpoints to interact
with your cozy-stack installation (E.g. interacting with instances, generating tokens, ...).

:warning: Use the admin API only if you know what you are doing. The admin API
provides a basic authentication, you **must** protect these endpoints as they
are very powerful.

The default port for the admin endpoints is `6060`. If you want to customize the parameters, please see the [config file documentation page](config.md).


## Instance

### GET /instances/with-app-version/:slug/:version

Returns all the instances using slug/version pair

#### Request

```http
GET /instances/with-app-version/drive/1.0.0 HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
    "instances": [
        "alice.cozy.tools",
        "bob.cozy.tools",
        "zoe.cozy.tools"
    ]
}
```

### POST /instances/:domain/fixers/content-mismatch

Fixes the 64k (or multiple) content mismatch files of an instance

#### Request

```http
POST /instances/:domain/fixers/content-mismatch HTTP/1.1
Accept: application/vnd.api+json
```

```json
{
  "dry_run": true
}
```

The `dry_run` (default to `true`) body parameter tells if the request is a
dry-run or not.

#### Response

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
  "domain": "alice.cozy.tools"
}
```

## Swift

### GET /swift/layouts

Count swift layouts by type

#### Request

```http
GET /swift/layouts HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

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
  "v3": {
    "counter": 2
  }
}
```

The `show_domains=true` query parameter provides the domain names if needed


#### Request

```http
GET /swift/layouts?show_domains=true HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "total": 3,
  "unknown": {
    "counter": 0
  },
  "v1": {
    "counter": 1,
    "domains": [
      "bob.cozy.tools:8081"
    ]
  },
  "v2a": {
    "counter": 0
  },
  "v2b": {
    "counter": 0
  },
  "v3": {
    "counter": 2,
    "domains": [
      "alice.cozy.tools:8081",
      "ru.cozy.tools:8081"
    ]
  }
}
```

### GET /swift/vfs/:object

Retrieves a Swift object

#### Request

```http
GET /swift/vfs/67a88b22520680b1fae840%2F9a8a0%2F18d02%2FiYbkfuCDEMaVoIXg HTTP/1.1
Host: alice.cozy.tools
```

#### Response

```text
"foobar"
```

### POST /swift/put

Put an object in Swift

Parameters:
- `instance`
- `object_name`
- `content`
- `content_type`

#### Request

```http
POST /swift/put HTTP/1.1
Accept: application/vnd.api+json
```

```json
{
  "instance": "alice.cozy.tools",
  "object_name": "67a88b22520680b1fae840/9a8a0/18d02/iYbkfuCDEMaVoIXg",
  "content": "this is my content",
  "content-type": "text/plain"
}
```

### DELETE /swift/:domain/:object

Removes an object from Swift

#### Request

```http
POST /swift/alice.cozy.tools/67a88b22520680b1fae840%2F9a8a0%2F18d02%2FiYbkfuCDEMaVoIXg HTTP/1.1
Accept: application/vnd.api+json
```

### GET /swift/ls/:domain

List Swift objects of an instance

#### Request

```http
POST /swift/ls/alice.cozy.tools HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

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

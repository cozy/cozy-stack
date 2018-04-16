[Table of contents](README.md#table-of-contents)

{% raw %}

# Sharing

The owner of a cozy instance can share access to her documents to other users.

## Sharing by links

A client-side application can propose sharing by links:

1. The application must have a public route in its manifest. See
   [Apps documentation](apps.md#routes) for how to do that.
2. The application can create a set of permissions for the shared documents,
   with codes. See [permissions documentation](permissions.md) for the details.
3. The application can then create a shareable link (e.g.
   `https://calendar.cozy.example.net/public?sharecode=eiJ3iepoaihohz1Y`) by
   putting together the app sub-domain, the public route path, and a code for
   the permissions set.
4. The app can then send this link by mail, via the [jobs system](jobs.md), or
   just give it to the user, so he can transmit it to her friends via chat or
   other ways.

When someone opens the shared link, the stack will load the public route, find
the corresponding `index.html` file, and replace `{{.Token}}` inside it by a
token with the same set of permissions that `sharecode` offers. This token can
then be used as a `Bearer` token in the `Authorization` header for requests to
the stack (or via cozy-client-js).

If necessary, the application can list the permissions for the token by calling
`/permissions/self` with this token.

## Cozy to cozy sharing

The owner of a cozy instance can send and synchronize documents to others cozy
users.

### Routes

#### POST /sharings/

Create a new sharing. The sharing rules and recipients must be specified. The
`description`, `preview_path`, and `open_sharing` fields are optional. The
`app_slug` field is optional and is the slug of the web app by default.

##### Request

```http
POST /sharings/ HTTP/1.1
Host: alice.example.net
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.sharings",
    "attributes": {
      "description": "sharing test",
      "preview_path": "/preview-sharing",
      "rules": [
        {
          "title": "Hawaii",
          "doctype": "io.cozy.files",
          "values": ["612acf1c-1d72-11e8-b043-ef239d3074dd"],
          "add": "sync",
          "update": "sync",
          "remove": "sync"
        }
      ]
    },
    "relationships": {
      "recipients": {
        "data": [
          {
            "id": "2a31ce0128b5f89e40fd90da3f014087",
            "type": "io.cozy.contacts"
          }
        ]
      }
    }
  }
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.sharings",
    "id": "ce8835a061d0ef68947afe69a0046722",
    "meta": {
      "rev": "1-4859c6c755143adf0838d225c5e97882"
    },
    "attributes": {
      "description": "sharing test",
      "preview_path": "/preview-sharing",
      "app_slug": "drive",
      "owner": true,
      "created_at": "2018-01-04T12:35:08Z",
      "updated_at": "2018-01-04T13:45:43Z",
      "members": [
        {
          "status": "owner",
          "name": "Alice",
          "email": "alice@example.net",
          "instance": "alice.example.net"
        },
        {
          "status": "mail-not-sent",
          "name": "Bob",
          "email": "bob@example.net"
        }
      ],
      "rules": [
        {
          "title": "Hawaii",
          "doctype": "io.cozy.files",
          "values": ["612acf1c-1d72-11e8-b043-ef239d3074dd"],
          "add": "sync",
          "update": "sync",
          "remove": "sync"
        }
      ]
    },
    "links": {
      "self": "/sharings/ce8835a061d0ef68947afe69a0046722"
    }
  }
}
```

### GET /sharings/:sharing-id/discovery

If no preview_path is set, it's an URL to this route that will be sent to the
users to notify them that someone wants to share something with them. On this
page, they can fill the URL of their Cozy (if the user has already filled its
Cozy URL in a previous sharing, the form will be pre-filled and the user will
just have to click OK).

#### Query-String

| Parameter | Description                        |
| --------- | ---------------------------------- |
| state     | a code that identify the recipient |

#### Example

```http
GET /sharings/ce8835a061d0ef68947afe69a0046722/discovery?state=eiJ3iepoaihohz1Y HTTP/1.1
Host: alice.example.net
```

### POST /sharings/:sharing-id/discovery

Give to the cozy of the sharer the URL of the Cozy of one recipient. The sharer
will register its-self as an OAuth client on the recipient cozy, and then will
ask the recipient to accept the permissions on its instance.

This route exists in two versions, the version is selected by the HTTP header
`Accept`

#### Classical (`x-www-url-encoded`)

| Parameter | Description                           |
| --------- | ------------------------------------- |
| state     | a code that identify the recipient    |
| url       | the URL of the Cozy for the recipient |

##### Example

```http
POST /sharings/ce8835a061d0ef68947afe69a0046722/discovery HTTP/1.1
Host: alice.example.org
Content-Type: application/x-www-form-urlencoded
Accept: text/html

state=eiJ3iepoaihohz1Y&url=https://bob.example.net/
```

```http
HTTP/1.1 302 Moved Temporarily
Location: https://bob.example.net/auth/sharing?...
```

#### JSON

This version can be more convenient for applications that implement the
preview page. To do that, an application must give a `preview_path` when
creating the sharing. This path must be a public route of this application.
The recipients will receive a link to the application subdomain, on this page,
and with a `sharecode` in the query string (like for a share by link).

To know the `sharing-id`, it's possible to ask `GET /permissions/self`, with
the `sharecode` in the `Authorization` header (it's a JWT token). In the
response, the `source_id` field will be `io.cozy.sharings/<sharing-id>`.

##### Parameters

| Parameter | Description                           |
| --------- | ------------------------------------- |
| sharecode | a code that identify the recipient    |
| url       | the URL of the Cozy for the recipient |

##### Example

```http
POST /sharings/ce8835a061d0ef68947afe69a0046722/discovery HTTP/1.1
Host: alice.example.org
Content-Type: application/x-www-form-urlencoded
Accept: application/json

sharecode=eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJhcHAiLCJpYXQiOjE1MjAzNDM4NTc&url=https://bob.example.net/
```

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "redirect": "https://bob.example.net/auth/sharing?..."
}
```

### GET /sharings/:sharing-id

Get the information about a sharing. This includes the content of the rules, the members, as well as the already shared documents for this sharing.

#### Request

```http
GET /sharings/ce8835a061d0ef68947afe69a0046722 HTTP/1.1
Host: alice.example.net
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
    "type": "io.cozy.sharings",
    "id": "ce8835a061d0ef68947afe69a0046722",
    "meta": {
      "rev": "1-4859c6c755143adf0838d225c5e97882"
    },
    "attributes": {
      "description": "sharing test",
      "preview_path": "/preview-sharing",
      "app_slug": "drive",
      "owner": true,
      "created_at": "2018-01-04T12:35:08Z",
      "updated_at": "2018-01-04T13:45:43Z",
      "members": [
        {
          "status": "owner",
          "name": "Alice",
          "email": "alice@example.net",
          "instance": "alice.example.net"
        },
        {
          "status": "ready",
          "name": "Bob",
          "email": "bob@example.net"
        }
      ],
      "rules": [
        {
          "title": "Hawaii",
          "doctype": "io.cozy.files",
          "values": ["612acf1c-1d72-11e8-b043-ef239d3074dd"],
          "add": "sync",
          "update": "sync",
          "remove": "sync"
        }
      ],
    },
    "relationships": {
      "shared_docs": {
        "data": [
          {
            "id": "612acf1c-1d72-11e8-b043-ef239d3074dd",
            "type": "io.cozy.files"
          },
          {
            "id": "a34528d2-13fb-9482-8d20-bf1972531225",
            "type": "io.cozy.files"
          }
        ]
      }
    },
    "links": {
      "self": "/sharings/ce8835a061d0ef68947afe69a0046722"
    }
  }
}
```


### PUT /sharings/:sharing-id

The sharer's cozy sends a request to this route on the recipient's cozy to
create a sharing request, with most of the informations about the sharing.
These informations will be displayed to the recipient just before its final
acceptation of the sharing, to be sure he/she knows what will be shared.

#### Request

```http
PUT /sharings/ce8835a061d0ef68947afe69a0046722 HTTP/1.1
Host: bob.example.net
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.sharings",
    "id": "ce8835a061d0ef68947afe69a0046722",
    "attributes": {
      "description": "sharing test",
      "preview_path": "/preview-sharing",
      "app_slug": "drive",
      "owner": true,
      "created_at": "2018-01-04T12:35:08Z",
      "updated_at": "2018-01-04T13:45:43Z",
      "members": [
        {
          "status": "owner",
          "name": "Alice",
          "email": "alice@example.net",
          "instance": "alice.example.net"
        },
        {
          "status": "mail-not-sent",
          "name": "Bob",
          "email": "bob@example.net",
          "instance": "bob.example.net"
        }
      ],
      "rules": [
        {
          "title": "Hawaii",
          "doctype": "io.cozy.files",
          "values": ["612acf1c-1d72-11e8-b043-ef239d3074dd"],
          "add": "sync",
          "update": "sync",
          "remove": "sync"
        }
      ]
    }
  }
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.sharings",
    "id": "ce8835a061d0ef68947afe69a0046722",
    "meta": {
      "rev": "1-f579a69a9fa5dd720010a1dbb82320be"
    },
    "attributes": {
      "description": "sharing test",
      "preview_path": "/preview-sharing",
      "app_slug": "drive",
      "owner": true,
      "created_at": "2018-01-04T12:35:08Z",
      "updated_at": "2018-01-04T13:45:43Z",
      "members": [
        {
          "status": "owner",
          "name": "Alice",
          "email": "alice@example.net",
          "instance": "alice.example.net"
        },
        {
          "status": "mail-not-sent",
          "name": "Bob",
          "email": "bob@example.net",
          "instance": "bob.example.net"
        }
      ],
      "rules": [
        {
          "title": "Hawaii",
          "doctype": "io.cozy.files",
          "values": ["612acf1c-1d72-11e8-b043-ef239d3074dd"],
          "add": "sync",
          "update": "sync",
          "remove": "sync"
        }
      ]
    },
    "links": {
      "self": "/sharings/ce8835a061d0ef68947afe69a0046722"
    }
  }
}
```

### POST /sharings/:sharing-id/answer

This route is used by the Cozy of a recipient to exchange credentials with the
Cozy of the sharer, after the recipient has accepted a sharing.

#### Request

```http
POST /sharings/ce8835a061d0ef68947afe69a0046722/answer HTTP/1.1
Host: alice.example.net
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.sharings.answer",
    "id": "ce8835a061d0ef68947afe69a0046722",
    "attributes": {
      "state": "eiJ3iepoaihohz1Y",
      "client": {...},
      "access_token": "uia7b85928e5cf"
    }
  }
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.sharings.answer",
    "id": "ce8835a061d0ef68947afe69a0046722",
    "attributes": {
      "client": {...},
      "access_token": "ui77bd4670fbd3"
    }
  }
}
```

### POST /sharings/:sharing-id/\_revs_diff

This endpoint is used by the sharing replicator of the stack to know which
documents must be sent to the other cozy. It is inspired by
http://docs.couchdb.org/en/2.1.1/api/database/misc.html#db-revs-diff.

#### Request

```http
POST /sharings/ce8835a061d0ef68947afe69a0046722/_revs_diff HTTP/1.1
Host: bob.example.net
Accept: application/json
Content-Type: application/json
Authorization: Bearer ...
```

```json
{
  "io.cozy.files/29631902-2cec-11e8-860d-435b24c2cc58": [
    "2-4a7e4ae49c4366eaed8edeaea8f784ad"
  ],
  "io.cozy.files/44f5752a-2cec-11e8-b227-abfc3cfd4b6e": [
    "4-2ee767305024673cfb3f5af037cd2729",
    "4-efc54218773c6acd910e2e97fea2a608"
  ]
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "io.cozy.files/44f5752a-2cec-11e8-b227-abfc3cfd4b6e": {
    "missing": [
      "4-2ee767305024673cfb3f5af037cd2729"
    ],
    "possible_ancestors": [
      "3-753875d51501a6b1883a9d62b4d33f91"
    ]
  }
}
```

### POST /sharings/:sharing-id/\_bulk_docs

This endpoint is used by the sharing replicator of the stack to send
documents in a bulk to the other cozy. It is inspired by
http://docs.couchdb.org/en/2.1.1/api/database/bulk-api.html#db-bulk-docs.

**Note**: we force `new_edits` to `false`.

#### Request

```http
POST /sharings/ce8835a061d0ef68947afe69a0046722/_bulk_docs HTTP/1.1
Host: bob.example.net
Accept: application/json
Content-Type: application/json
Authorization: Bearer ...
```

```json
{
  "io.cozy.files": [{
    "_id": "44f5752a-2cec-11e8-b227-abfc3cfd4b6e",
    "_rev": "4-2ee767305024673cfb3f5af037cd2729",
    "_revisions": {
      "start": 4,
      "ids": [
        "2ee767305024673cfb3f5af037cd2729",
        "753875d51501a6b1883a9d62b4d33f91",
      ]
    }
  }]
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
[]
```

{% endraw %}

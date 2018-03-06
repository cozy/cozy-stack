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
Host: cozy.example.net
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
          "title": "folder",
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
          "email": "bob@example.net",
        },
      ],
      "rules": [
        {
          "title": "folder",
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

{% endraw %}

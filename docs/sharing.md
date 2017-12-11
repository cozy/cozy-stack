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

### Sharing document

A sharing document has the following structure in CouchDB. Note some fields are
purposely left empty for space convenience. And the response from the API
hides some fields from the document (OAuth credentials in particular).

```json
{
  "_id": "xxx",
  "_rev": "yyy",
  "sharing_type": "one-shot",
  "description": "Give it to me baby!",
  "owner": true,
  "app_slug": "cal",
  "preview_path": "/sharings/preview",
  "permissions": {
    "type": "io.cozy.permissions",
    "id": "80a466d6-d034-11e7-bf9e-e3c1c4a9f82a"
  },
  "recipients": [
    {
      "contact": { "id": "mycontactid1", "type": "io.cozy.contacts" },
      "status": "accepted",
      "url": "https://example.mycozy.cloud/",
      "access_token": {},
      "client": {},
      "inbound_client_id": "myhostclientid"
    },
    {
      "contact": { "id": "mycontactid2", "type": "io.cozy.contacts" },
      "status": "pending"
    }
  ]
}
```

#### owner

To tell if the owner of the Cozy is also the owner of the sharing. This field is
set automatically by the stack when creating (`true`) or receiving (`false`)
one.

#### revoked

An additional field, `revoked`, will be added to the document, with the value
`true` when the sharing has been revoked.

#### permissions

Which documents will be shared. See the
[permissions](https://cozy.github.io/cozy-stack/permissions.html) for a detailed
explanation of the permissions format. In the sharing document, there is only a
reference to the permissions document.

The supported permissions are the following:

* **Static**: one specify the documents ids to share through the `values` field.

Example of a single file sharing permission:

```json
"permissions": {
  "files": {
    "description": "My secret document",
    "type": "io.cozy.files",
    "values": ["fileid"],
    "verbs": ["ALL"]
  }
},
```

* **Reference**: uses the
  [referenced_by](https://cozy.github.io/cozy-stack/references-docs-in-vfs.html)
  field to express a sharing based on relations. Each time a new relation is
  added to the Cozy and match the permission, e.g. a new photo related to an
  album, it is automatically shared with the recipients.

This requires to specify 2 permissions. The first one is a static permission on
the referenced object, e.g. an album. The second one specifies the targets of
the referenced object, e.g. the files containing a reference to the album. This
permission includes the 'referenced_by' keyword in the `selector` field and the
referenced object in the `values` field, formatted as `doctype/id`.

Example of a photo album sharing:

```json
"permissions": {
  "photos": {
    "description": "Holidays album",
    "type": "io.cozy.albums.photos",
    "values": ["albumdocid"],
    "verbs": ["ALL"]
  },
  "files": {
    "description": "Holidays photos",
    "type": "io.cozy.files",
    "values": ["io.cozy.albums.photos/albumdocid"],
    "selector": "referenced_by",
    "verbs": ["ALL"]
  }
}
```

It is worth mentionning that the permissions are defined on the sharer side, but
are enforced on the recipients side (and also on the sharer side if the sharing
is a two-way type), as the documents are pushed to their databases.

#### recipients

List all the recipients of the sharing:

```json
"recipients": [
  {
    "contact": {"id": "mycontactid1", "type": "io.cozy.contacts"},
    "status": "accepted",
    "url": "https://example.mycozy.cloud/"
  },
  {
    "contact": {"id": "mycontactid2", "type": "io.cozy.contacts"},
    "status": "pending"
  }
]
```

##### contact

Specify the contact document containing the `url` and `email` informations.

We differentiate a recipient from a contact. Semantically, The former has a
meaning only in a sharing context while the later is a
[Cozy contact](https://cozy.github.io/cozy-doctypes/io.cozy.contacts.html),
usable in other contexts.

A contact has the following minimal structure:

```json
{
  "id": "mycontactid1",
  "type": "io.cozy.contacts",
  "email": [{ "address": "bob@mail.cozy" }],
  "cozy": [{ "url": "https://bob.url.cozy" }]
}
```

Note that the `email` is mandatory to contact the recipient. If the cozy `URL`
is missing, a discovery mail will be sent in order to ask the recipient to give
it.

##### url

The URL of the recipient cozy that will be used to synchronize the documents.

##### status

The recipient' sharing status possible values are:

* `pending`: the recipient didn't reply yet.
* `mail-not-sent`: no invitation mail was sent for this recipient (no known
  email or an error on sending the mail).
* `accepted`: the recipient accepted the sharing.
* `revoked`: the recipient has been revoked.

##### access_token

The OAuth credentials used to authenticate to the recipient's Cozy.

See
[here](https://github.com/cozy/cozy-stack/blob/master/docs/auth.md#post-authaccess_token)
for structure details.

Here, the `scope` corresponds to the accepted sharing permissions by the
recipient.

##### client

From a OAuth perspective, Bob being Alice's recipient means Alice is registered
as a OAuth client to Bob's Cozy. Thus, we store for each recipient the
informations sent by the recipient after the sharer registration.

See
[here](https://github.com/cozy/cozy-stack/blob/master/docs/auth.md#post-authaccess_token)
for structure details.

##### inbound_client_id

This field is only used for `two-way` sharing. It corresponds to the id of the
OAuth document stored in the host database, containing the recipient's OAuth
information after registration.

#### sharer

It's the same structure as a recipient, but it's only here for the sharing
document that is not the owner of the sharing.

#### sharing_type

The type of sharing. It should be one of the followings: `two-way`, `one-way`,
`one-shot`. They represent the access rights the recipient and sender have:

* `one-shot`: the documents are sent and no modification is propagated.
* `one-way`: only the sharer can propagate modifications to the recipient. The
  recipient can only modify the documents localy.
* `two-way`: both recipient and sharer can modify the documents and have their
  modifications propagated to the other.

#### description

The answer to the question: "What are you sharing?". It is an optional field
but, still, it is recommended to provide a human-readable description.

#### preview_path

The path part of the URL on the application that will be used by recipients to
preview the sharing before accepting it. It is optional.

The application has to define a public route for this path in its manifest, else
the recipients will be denied the access to this page.

### Routes

#### POST /sharings/

Create a new sharing. The sharing type, permissions and recipients must be
specified. The `description` and `preview_path` fields are optional.

##### Request

```http
POST /sharings/ HTTP/1.1
Host: cozy.example.net
Content-Type: application/json
```

```json
{
  "sharing_type": "one-shot",
  "permissions": {
    "tests": {
      "description": "test",
      "type": "io.cozy.tests",
      "values": ["test-id"]
    }
  },
  "recipients": ["2a31ce0128b5f89e40fd90da3f014087"],
  "description": "sharing test",
  "preview_path": "/sharings/preview"
}
```

**Note:** for the permissions, the HTTP `verbs` will be overwritten by the
cozy-stack with the values needed to operate the sharing. The recipients field
is an array with ids of contacts (that must have been already created on the
cozy).

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
      "sharing_type": "one-shot",
      "description": "sharing test",
      "preview_path": "/sharings/preview",
      "app_slug": "cal",
      "owner": true
    },
    "links": {
      "self": "/sharings/ce8835a061d0ef68947afe69a0046722"
    },
    "relationships": {
      "permissions": {
        "data": {
          "id": "46b25ad6-d044-11e7-af96-579d7e6c689e",
          "type": "io.cozy.permissions"
        }
      },
      "recipients": {
        "data": [
          {
            "id": "2a31ce0128b5f89e40fd90da3f014087",
            "type": "io.cozy.contacts",
            "status": "pending"
          }
        ]
      }
    }
  },
  "included": [
    {
      "type": "io.cozy.permissions",
      "id": "46b25ad6-d044-11e7-af96-579d7e6c689e",
      "meta": {
        "rev": "1-fbed00fed407"
      },
      "attributes": {
        "type": "shared-by-me",
        "source_id": "io.cozy.sharings/ce8835a061d0ef68947afe69a0046722",
        "codes": {
          "yuot7NaiaeGugh8T": "2a31ce0128b5f89e40fd90da3f014087"
        },
        "expires_at": 1483951978,
        "permissions": {
          "tests": {
            "description": "test",
            "type": "io.cozy.tests",
            "values": ["test-id"],
            "verbs": ["GET"]
          }
        }
      },
      "links": {
        "self": "/permissions/46b25ad6-d044-11e7-af96-579d7e6c689e"
      }
    },
    {
      "type": "io.cozy.contacts",
      "id": "2a31ce0128b5f89e40fd90da3f014087",
      "meta": {
        "rev": "1-461114b45855dc6acdb9bdc5d67e1092"
      },
      "attributes": {
        "email": {
          "address": "toto@fr"
        },
        "cozy": {
          "url": "url.fr"
        }
      },
      "links": {
        "self": "/contacts/2a31ce0128b5f89e40fd90da3f014087"
      }
    }
  ]
}
```

#### GET /sharings/:sharing-id

Get the informations about a sharing.

#### POST /sharings/answer

This route is used by the cozy of the recipient to inform the cozy of the owner
that the sharing has been accepted, and to setup the OAuth clients and start the
replication of the shared documents.

#### POST /sharings/:sharing-id/recipients

Add a recipient to a sharing.

##### Query-String

| Parameter | Description                  |
| --------- | ---------------------------- |
| ContactID | the id of the contact to add |

#### GET /sharings/:sharing-id/discovery

If no preview_path is set, it's an URL to this route that will be sent to the
users to notify them that someone wants to share something with them. On this
page, they can fill the URL of their Cozy (if the user has already filled its
Cozy URL in a previous sharing, the form will be pre-filled and the user will
just have to click OK).

##### Query-String

| Parameter | Description                        |
| --------- | ---------------------------------- |
| sharecode | a code that identify the recipient |

The sharecode is the same as the one used on the preview page to give the user
the right to display the to-be-shared documents.

#### POST /sharings/:sharing-id/discovery

Give to the cozy of the sharer the URL of the Cozy of one recipient. The sharer
will register its-self as an OAuth client on the recipient cozy, and then will
ask the recipient to accept the permissions on its instance.

##### Parameters (`x-www-url-encoded`)

| Parameter | Description                           |
| --------- | ------------------------------------- |
| sharecode | a code that identify the recipient    |
| url       | the URL of the Cozy for the recipient |

#### DELETE /sharings/:sharing-id

Revoke a sharing. Depending on the role of the logged-in user and the type of
sharing, the implications are different:

| ROLE / SHARING-TYPE | ONE-WAY SHARING                                         | TWO-WAY SHARING                                                  |
| ------------------- | ------------------------------------------------------- | ---------------------------------------------------------------- |
| Sharer              | Delete all triggers linked to the sharing.              | Delete all triggers linked to the sharing.                       |
|                     | Ask all recipients to revoke the sharing.               | Ask all recipients to revoke the sharing.                        |
|                     |                                                         | Revoke the OAuth clients of all the recipients for that sharing. |
| Recipient           | Revoke the OAuth client of the sharer for that sharing. | Revoke the OAuth client of the sharer for that sharing.          |
|                     | Ask the sharer to revoke the logged-in user.            | Ask the sharer to revoke the logged-in user.                     |
|                     |                                                         | Delete all triggers linked to the sharing.                       |

Permissions for that route are checked as following:

* The application at the origin of the sharing can revoke it.
* The sharer can ask the recipients to revoke the sharing.

##### Request

```http
DELETE /sharings/CfFNWhvEDzHDYOxQvzqPAfHcqQolmjEY HTTP/1.1
Authorization: Bearer zE3OTMsImlzcyI6ImNvenkyLmxvY2FsOjgwODAiLCJzdWIiOiI5ZTZlN …
Host: cozy.example.net
Content-Type: application/json
```

##### Response

```http
HTTP/1.1 204 No Content
Content-Type: application/json
```

Note: this route is also used internally by the sharer to inform the cozy of the
recipient that the sharing has been revoked on its side.

#### DELETE /sharings/:sharing-id/:recipient-client-id

This internal route is used by the cozy instance of the recipient to inform the
sharer that its owner has revoked the sharing.

#### DELETE /sharings/:sharing-id/recipient/:contact-id

Revoke a recipient from a sharing. Only the sharer can make that action and
depending on the type of sharing the implications differ:

* for both _Two-way_ and _One-way_ sharings the sharer asks the recipient to
  revoke the sharing;
* for _Two-way_ sharing the sharer also deletes the OAuth client of the
  recipient for that sharing.

##### Request

```http
DELETE /sharings/xkWMVOrVitZVSqXAAvErcmUAdEKMCLlx/recipient/f319a796-bfed-11e7-9903-d3d8f0929aa5 HTTP/1.1
Authorization: Bearer WQiOiJhY2Nlc3MiLCJpYXQiOjE1MDAzNzM0NDIsIml …
Host: cozy.example.net
Content-Type: application/json
```

##### Response

```http
HTTP/1.1 204 No Content
Content-Type: application/json
```

#### POST /sharings/destination/:doctype

Sets the destination directory of the given application. The "destination
directory" is where the shared files received by this application will go. Only
files shared using "Cozy to Cozy sharings" are concerned.

For example if a user sets the destination directory of the application "Photos"
to `/Shared with Me/Photos` (by providing its **id**) then all shared photos
will go there.

##### Request

Required parameters:

* `doctype`: the doctype concerned. For now only `io.cozy.files` can be used.
* `Dir_id`: the id of the destination directory. The directory should already
  exist.

```http
POST /sharings/destination/io.cozy.sharings?Dir_id=9e6e595ee50575a3faa064987d0e30eb HTTP/1.1
Host: cozy.example.net
Content-Type: application/json
```

##### Response

```http
HTTP/1.1 204 No Content
Content-Type: application/json
```

##### Note

The slug of the application that makes this request is extracted from its token
and stored in the config document. The application that creates the sharing on
the other cozy instance must have the same slug to trigger this behaviour.

### Frequently Asked Questions

#### How can I know if something is shared with me?

First, call the route
[GET /permissions/doctype/:doctype/sharedWithMe](permissions.md#get-permissionsdoctypedoctypesharedwithme)
to get a list of permissions. This route will only look for permissions that
apply to sharings where the logged-in user is a recipient.

Now check if your resource is subject to one of those permissions. If that's the
case then the resource was shared with the logged-in user.

#### How can I know if something was shared by me?

Same as above except you need to call the route
[GET /permissions/doctype/:doctype/sharedWithOthers](permissions.md#get-permissionsdoctypedoctypesharedwithothers).

#### Great! I know that my resource is shared. Can I have more information regarding the sharing?

Yes, in the permissions you obtained before there is a field called `source_id`.
The value of that field is the id of the sharing document the permission was
extracted from.

Having its id, you can fetch it and get all the information you need.

#### Could you remind me the different types of sharings?

* _One-shot_: the documents are sent to the recipients and that's it. No
  updates, no nothing. It's as if you gave them a copy of the data on a usb key.
* _One-way_: updates you make on the documents are propagated to the recipients.
  The recipients can only consult as everything they do will not be propagated
  back.
* _Two-way_: what you and the recipients do is propagated to everybody. Updates,
  deletions, additions are shared to all parties no matter if they are the
  sharer or the recipients.

#### Do you have use-cases for the different types of sharings?

Yes!

* For _one-shot_: an official paper (such as a bill or an ID) you want to give
  to someone.
* For _one-way_: a password file that the sysadmins want to share to the rest of
  the company. Only the sysadmins can modify the password file, the others can
  only consult it.
* For _two-way_: a folder containing shared resources for a project. You want
  all parties to be able to modify the content as well as adding new ones.

#### What are the information required for a recipient?

Only an email address is mandatory. The URL of their Cozy can also be used, but
it is not here, it's not a big deal: we have a discovery feature so we can ask
the recipients what is the URL of their Cozy.

#### Which documents are created and when?

When the user asks to share a resource, a sharing document is created. That
happens before the emails are sent to the recipients. That also means that if
all recipients refuse the sharing, the sharing document will still be there.

When the recipients accept, a sharing document is created on their own Cozy. The
sharing document the recipients have is slighty different from the sharer's one.

#### What are the differences between the sharing document located at the sharer and the one located at the recipients?

This table sums up the differences:

| Field      | Sharer                                          | Recipient                                   |
| ---------- | ----------------------------------------------- | ------------------------------------------- |
| Owner      | true                                            | false                                       |
| Recipients | Contains all the recipients related information | (empty)                                     |
| Sharer     | (empty)                                         | Contains all the sharer related information |

{% endraw %}

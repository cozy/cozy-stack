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

A sharing document has the following structure. Note some fields are purposely
left empty for space convenience.

```json
{
  "_id": "xxx",
  "_rev": "yyy",
  "type": "io.cozy.sharings",
  "sharing_type": "one-shot",
  "description": "Give it to me baby!",
  "sharing_id": "zzz",
  "owner": true,
  "app_slug": "cal",

  "permissions": {
    "doctype1": {
      "description": "doctype1 description",
      "type": "io.cozy.doctype1",
      "values": ["docid1", "docid2"],
      "selector": "calendar-id", //not supported yet
      "verbs": ["GET", "POST", "PUT"]
    }
  },
  "recipients": [
    {
      "recipient": { "id": "mycontactid1", "type": "io.cozy.contacts" },
      "status": "accepted",
      "AccessToken": {},
      "Client": {},
      "HostClientID": "myhostclientid"
    },
    {
      "recipient": { "id": "mycontactid2", "type": "io.cozy.contacts" },
      "status": "pending",
    }
  ]
}
```

#### owner

To tell if the owner of the Cozy is also the owner of the sharing. This field is
set automatically by the stack when creating (`true`) or receiving (`false`)
one.

#### permissions

Which documents will be shared. See the
[permissions](https://cozy.github.io/cozy-stack/permissions.html) for a
detailed explanation of the permissions format.

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
are enforced on the recipients side (and also on the sharer side if the
sharing is a master-master type), as the documents are pushed to their
databases.

#### recipients

List all the recipients of the sharing:

```json
"recipients": [
    {
        "recipient": {"id": "mycontactid1", "type": "io.cozy.contacts"},
        "status": "accepted",
    },
    {
        "recipient": {"id": "mycontactid2", "type": "io.cozy.contacts"},
        "status": "pending",
    }
]
```

##### recipient

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

Note that the `email` is mandatory to contact the recipient. If the `URL` is
missing, a discovery mail will be sent in order to ask the recipient to give it.

##### Status

The recipient' sharing status possible values are:

* `pending`: the recipient didn't reply yet.
* `accepted`: the recipient accepted.
* `refused`: the recipient refused.
* `error`: an error occured for this recipient
* `unregistered`: the registration failed
* `mail-not-sent`: the mail has not been sent
* `revoked`: the recipient has been revoked

##### AccessToken

The OAuth credentials used to authenticate to the recipient's Cozy.

See
[here](https://github.com/cozy/cozy-stack/blob/master/docs/auth.md#post-authaccess_token)
for structure details.

Here, the `scope` corresponds to the accepted sharing permissions by the
recipient.

##### Client

From a OAuth perspective, Bob being Alice's recipient means Alice is registered
as a OAuth client to Bob's Cozy. Thus, we store for each recipient the
informations sent by the recipient after the sharer registration.

See
[here](https://github.com/cozy/cozy-stack/blob/master/docs/auth.md#post-authaccess_token)
for structure details.

##### HostClientID

This field is only used for `master-master` sharing. It corresponds to the id of
the OAuth document stored in the host database, containing the recipient's OAuth
information after registration.

#### sharing_type

The type of sharing. It should be one of the followings: `master-master`,
`master-slave`, `one-shot`. They represent the access rights the recipient and sender have:

* `one-shot`: the documents are sent and no modification is propagated.
* `master-slave`: only the sharer can propagate modifications to the recipient.
  The recipient can only modify the documents localy.
* `master-master`: both recipient and sharer can modify the documents and have
  their modifications propagated to the other.

#### description

The answer to the question: "What are you sharing?". It is an optional field
but, still, it is recommended to provide a human-readable description.

#### sharing_id

This uniquely identify a sharing. It is automatically generated at the sharing
creation.

### Routes

#### POST /sharings/

Create a new sharing. The sharing type, permissions and recipients must be
specified. The description field is optionnal.

Note the recipient id must correspond to an actual contact previously inserted
in the database.

##### Request

```http
POST /sharings/ HTTP/1.1
Host: cozy.example.net
Content-Type: application/json
```

```json
{
  "sharing_type": "one-shot",
  "description": "sharing test",
  "permissions": {
    "tests": {
      "description": "test",
      "type": "io.cozy.tests",
      "verbs": ["GET", "POST"],
      "values": ["test-id"]
    }
  },
  "recipients": [
    {
      "recipient": {
        "type": "io.cozy.contacts",
        "id": "2a31ce0128b5f89e40fd90da3f014087"
      }
    }
  ]
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
    "attributes": {
      "owner": true,
      "sharing_id": "wccKeeGnAppnHgXWqBxKqSpKNpZiMeFR",
      "sharing_type": "one-shot",
      "permissions": {
        "tests": {
          "type": "io.cozy.tests",
          "description": "test",
          "values": ["test-id"]
        }
      }
    },
    "meta": {
      "rev": "1-4859c6c755143adf0838d225c5e97882"
    },
    "links": {
      "self": "/sharings/ce8835a061d0ef68947afe69a0046722"
    },
    "relationships": {
      "recipients": {
        "data": [
          {
            "id": "2a31ce0128b5f89e40fd90da3f014087",
            "type": "io.cozy.contacts"
            "status": "pending",
          }
        ]
      }
    }
  },
  "included": [
    {
      "type": "io.cozy.contacts",
      "id": "2a31ce0128b5f89e40fd90da3f014087",
      "attributes": {
        "email": {
          "address": "toto@fr"
        },
        "cozy": {
          "url": "url.fr"
        }
      },
      "meta": {
        "rev": "1-461114b45855dc6acdb9bdc5d67e1092"
      },
      "links": {
        "self": "/contacts/2a31ce0128b5f89e40fd90da3f014087"
      }
    }
  ]
}
```

### POST /sharings/:id/sendMail

Send a sharing request by mail.

### PUT /sharings/:id

Receive a sharing request.

### POST /sharings/:id/answer

Answer a sharing request.

### DELETE /sharings/:sharing-id

Revoke a sharing. Depending on the role of the logged-in user and the type of
sharing, the implications are different:

| ROLE / SHARING-TYPE | MASTER-SLAVE SHARING                                    | MASTER-MASTER SHARING                                            |
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

#### Request

```http
DELETE /sharings/CfFNWhvEDzHDYOxQvzqPAfHcqQolmjEY HTTP/1.1
Authorization: Bearer zE3OTMsImlzcyI6ImNvenkyLmxvY2FsOjgwODAiLCJzdWIiOiI5ZTZlN …
Host: cozy.example.net
Content-Type: application/json
```

#### Response

```http
HTTP/1.1 204 No Content
Content-Type: application/json
```

### DELETE /sharings/:sharing-id/:recipient-client-id

This internal route is used by the cozy instance of the recipient to inform the
sharer that its owner has revoked the sharing.

### DELETE /sharings/:sharing-id/recipient/:contact-id

Revoke a recipient from a sharing. Only the sharer can make that action and
depending on the type of sharing the implications differ:

* for both _Master-Master_ and _Master-Slave_ sharings the sharer asks the
  recipient to revoke the sharing;
* for _Master-Master_ sharing the sharer also deletes the OAuth client of the
  recipient for that sharing.

#### Request

```http
DELETE /sharings/xkWMVOrVitZVSqXAAvErcmUAdEKMCLlx/recipient/f319a796-bfed-11e7-9903-d3d8f0929aa5 HTTP/1.1
Authorization: Bearer WQiOiJhY2Nlc3MiLCJpYXQiOjE1MDAzNzM0NDIsIml …
Host: cozy.example.net
Content-Type: application/json
```

#### Response

```http
HTTP/1.1 204 No Content
Content-Type: application/json
```

### POST /sharings/app/destinationDirectory

Sets the destination directory of the given application.\
The "destination directory" is where the shared files received by this application
will go. Only files shared using "Cozy to Cozy sharings" are concerned.

For example if a user sets the destination directory of the application "Photos"
to `/Shared with Me/Photos` (by providing its **id**) then all shared photos
will go there.

#### Request

Query parameters required:

* `App_slug`: the slug of the application.
* `Doctype`: the doctype concerned. For now only `io.cozy.files` can be used.
* `Dir_id`: the id of the destination directory. The directory should already
  exist.

```http
POST /sharings/app/destinationDirectory?App_slug=sharotronic&Doctype=io.cozy.sharings&Dir_id=9e6e595ee50575a3faa064987d0e30eb HTTP/1.1
Host: cozy.example.net
Content-Type: application/json
```

#### Response

```http
HTTP/1.1 204 No Content
Content-Type: application/json
```

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
* _Master-slave_: updates you make on the documents are propagated to the
  recipients. The recipients can only consult as everything they do will not be
  propagated back.
* _Master-master_: what you and the recipients do is propagated to everybody.
  Updates, deletions, additions are shared to all parties no matter if they are
  the sharer or the recipients.

#### Do you have use-cases for the different types of sharings?

Yes!

* For _one-shot_: an official paper (such as a bill or an ID) you want to give
  to someone.
* For _master-slave_: a password file that the sysadmins want to share to the
  rest of the company. Only the sysadmins can modify the password file, the
  others can only consult them.
* For _master-master_: a folder containing shared resources for a project. You
  want all parties to be able to modify the content as well as adding new ones.

#### What are the information required for a recipient?

Two things: an e-mail and the URL of the Cozy. We have a discovery feature so
the URL is not a necessity but it will be convenient if you don't want the
recipients to enter their URL everytime you share something with them.

#### Which documents are created and when?

When the user asks to share a resource, a sharing document is created. That
happens before the emails are sent to the recipients. That also means that if
all recipients refuse the sharing, the sharing document will still be there.

The permissions associated are described in that document but **no actual permission
documents are created, at any point in the protocol** — permissions are still enforced,
there is just no need to create permission documents.

When the recipients accept, a sharing document is created on their own Cozy. The
sharing document the recipients have is slighty different from the sharer's one.

#### What are the differences between the sharing document located at the sharer and the one located at the recipients?

This table sums up the differences:

| Field      | Sharer                                          | Recipient                                   |
| ---------- | ----------------------------------------------- | ------------------------------------------- |
| Owner      | True                                            | False                                       |
| Recipients | Contains all the recipients related information | (empty)                                     |
| Sharer     | (empty)                                         | Contains all the sharer related information |

{% endraw %}

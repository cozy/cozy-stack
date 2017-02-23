[Table of contents](README.md#table-of-contents)

{% raw %}

# Sharing

The owner of a cozy instance can share access to her documents to other users.

## Sharing by links

A client-side application can propose sharing by links:

1. The application must have a public route in its manifest. See
   [Apps documentation](apps.md#routes) for how to do that.
2. The application can create a set of permissions for the shared documents,
   with codes. See [permissions documentation](permissions.md) for the
   details.
3. The application can then create a shareable link (e.g.
   `https://calendar.cozy.example.net/public?code=eiJ3iepoaihohz1Y`) by
   putting together the app sub-domain, the public route path, and a code for
   the permissions set.
4. The app can then send this link by mail, via the [jobs system](jobs.md), or
   just give it to the user, so he can transmit it to her friends via chat or
   other ways.

When someone opens the shared link, the stack will load the public route, find
the corresponding `index.html` file, and replace `{{.Token}}` inside it by a
token with the same set of permissions that `code` offers. This token can then
be used as a `Bearer` token in the `Authorization` header for requests to the
stack (or via cozy-client-js).

If necessary, the application can list the permissions for the token by
calling `/permissions/self` with this token.


## Cozy to cozy sharing

**Warning**: this is a work in progress, and might change in the future.

The owner of a cozy instance can send and synchronize documents to others cozy users.

### Sharing declaration

A sharing document has this structure:

```json
    {
        "_id": "xxx",
        "_rev": "yyy",
        "type": "io.cozy.sharings",
        "type": "one-shot",
        "description": "Give it to me baby!",
        "shareID": "zzz",
        "owner": true,

        "permissions": {
            "doctype1": {
                "description": "doctype1 description",
                "type": "io.cozy.doctype1",
                "values": ["id1", "id2"],
                "selector": "calendar-id", //not supported yet
                "verbs": ["GET","POST", "PUT"]
            }          
        },
        "recipients": [
            {
                "recipientID": "recipientID1",
                "status": "accepted",
                "access_token": "myaccesstoken1",
                "refresh_token": "myrefreshtoken1"
            },
            {
                "recipientID": "recipientID2",
                "status": "pending"
            }
        ]
    }
```

#### Owner

To tell if the owner of the Cozy is also the owner of the sharing. This field is set automatically by the stack when creating (`true`) or receiving (`false`) one.

#### Permissions

Which documents will be shared. We provide their ids, and eventually a selector for a more dynamic solution (this will come later, though). See [here](https://github.com/cozy/cozy-stack/blob/master/docs/permissions.md) for a detailed explanation of the permissions format.

It is worth mentionning that the permissions are defined on the sharer side, but are be enforced on the recipients side (and also on the sharer side if the sharing is a master-master type), as the documents are pushed to their databases.


#### Recipients

An array of the recipients and, for each of them, their recipientID, the status of the sharing as well as their token of authentification and the refresh token, if they have accepted the sharing.

The recipientID is the id the document storing the informations relatives to a recipient. The structure is the following:
```json
{
    "type": "io.cozy.recipients",

    "url": "bob.url",
    "mail": "bob@mail",

    "oauth": {
        "client_id": "myclientid",
        "client_name": "myclientname",
        "client_secret": "myclientsecret",
        "registration_access_token": "myregistration",
        "redirect_uri": ["alice.cozy/oauth/callback"]
    }
}


```
From a OAuth perspective, Bob being Alice's recipient means Alice is registered as a OAuth client to Bob's Cozy. Thus, we store in this document the informations sent by Bob after Alice's registration.


For the sharing status, the possible values are:
* `pending`: the recipient didn't reply yet.
* `accepted`: the recipient accepted.
* `refused`: the recipient refused.

#### Type

The type of sharing. It should be one of the followings: `master-master`, `master-slave`, `one-shot`.  
They represent the access rights the recipient and sender have:
* `master-master`: both recipient and sender can modify the documents and have their modifications pushed to the other.
* `master-slave`: only the sender can push modifications to the recipient. The recipient can modify localy the documents.
* `one-shot`: the documents are duplicated and no modifications are pushed.

#### Description

The answer to the question: "What are you sharing?". It is an optional field but, still, it is recommended to provide a small human-readable description.

#### ShareID

This uniquely identify a sharing. This corresponds to the id of the sharing document, on the sharer point of view and is automatically generated at the sharing creation.


### Where is the corresponding code?

#### cozy-stack/pkg/sharings

The implementation of the logic: creating a new sharing, handling an answer, starting a replication, etc.

#### cozy-stack/web/sharings

The declaration of the routes and their chaining.

### Routes

#### POST /sharings

Create a new sharing.

### POST /sharings/:id/sendMail

Send a sharing request by mail.

### PUT /sharings/:id

Receive a sharing request.

### POST /sharings/:id/answer

Answer a sharing request.

### DELETE /sharings/:id

Delete the specified sharing (both the sharing document and the associated permission).

{% endraw %}
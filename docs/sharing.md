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

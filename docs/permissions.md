[Table of contents](./README.md#table-of-contents)

# Permissions

## What is a permission?

A permission gives the right for a request having it to do something on the
stack. It is defined by four components.

### `type`

`type` is the attribute used in JSON-API or the `docType` for the Data System.

It is the only mandatory component. If just the `type` is specified, it gives
access to all the operations on this `type`. For example, a permission on type
`io.cozy.contacts` gives the right to create, read, update and delete any
contact, and to fetch all the contacts. A permission on type `io.cozy.files`
allow to access and modify any file or directory.

Some known types:

- `io.cozy.files`, for files and folder in the [VFS](files.md)
- `io.cozy.manifests` and `io.cozy.applications`, for [apps](apps.md)
- `io.cozy.settings`, for the [settings](settings.md)
- `io.cozy.jobs` and `io.cozy.triggers`, for [jobs](jobs.md)
- `io.cozy.oauth.clients`, to list and revoke [OAuth 2 clients](auth.md)

### `verbs`

It says which HTTP verbs can be used for requests to the cozy-stack. `GET`
will gives read-only access, `DELETE` can be used for deletions, etc. You can
put several verbs separed by commas, like `GET,POST,DELETE`, and use `ALL` as
a shortcut for `GET,POST,PUT,PATCH,DELETE` (it is the default).

**Note**: `HEAD` is implicitely implied when `GET` is allowed. `OPTIONS` for
Cross-Origin Resources Sharing is always allowed, the stack does not have the
informations about the permission when it answers the request.

### `values`

It's possible to restrict the permissions to only some documents of a docType,
or to just some files and folders. You can give a list of ids in `values`.

**Note**: a permission for a folder also gives permissions with same verbs for
files and folders inside it.

### `selector`

By default, the `values` are checked with the `id`. But it's possible to use a
`selector` to filter on another `field`. In particular, it can be used for
sharing. A user may share a calendar and all the events inside it. It will be
done with two permissions. The first one is for the calendar:

```json
{
  "type": "io.cozy.calendars",
  "verbs": "GET",
  "values": ["1355812c-d41e-11e6-8467-53be4648e3ad"]
}
```

And the other is for the events inside the calendar:

```json
{
  "type": "io.cozy.events",
  "verbs": "GET",
  "selector": "calendar_id",
  "values": ["1355812c-d41e-11e6-8467-53be4648e3ad"]
}
```

## What format for a permission?

### JSON

Example:

```json
{
  "permissions": {
    "contacts": {
      "description": "Required for autocompletion on @name",
      "type": "io.cozy.contacts",
      "verbs": "GET"
    },
    "images": {
      "description": "Required for the background",
      "type": "io.cozy.files",
      "access": "GET",
      "values": ["io.cozy.files.music-dir"]
    },
    "mail": {
      "description": "Required to send a congratulations email to your friends"
      "type": "io.cozy.jobs",
      "selector": "worker",
      "values": ["sendmail"]
    }
  }
}
```

### Inline

OAuth2 as a `scope` parameter for defining the permissions given to the
application. But it's only a string, not a JSON. In that case, we use a space
delimited list of permissions, each permission is written compactly with `:`
between the components.

Example:

```
io.cozy.contacts io.cozy.files:GET:io.cozy.files.music-dir io.cozy.jobs:POST:sendmail:worker
```

**Note**: the `verbs` component can't be omitted when the `values` and
`selector` are used.

### Inspiration

- [Access control on other similar platforms](https://news.ycombinator.com/item?id=12784999)

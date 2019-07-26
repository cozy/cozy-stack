[Table of contents](README.md#table-of-contents)

# Instances

A single cozy-stack can manage several instances. Requests to different
instances are identified through the `Host` HTTP Header, any reverse proxy
placed in front of the cozy-stack should forward this header.

## Creation

An instance is created on the command line:

```sh
$ cozy-stack instances add <domain> [flags]
```

The flags are documented in the
[`instances add manpage`](cli/cozy-stack_instances_add.md).

It registers the instance in a global couchdb database `global/instances`
and creates the proper databases ($PREFIX/$DOCTYPE) for these doctypes:

-   `io.cozy.apps`
-   `io.cozy.contacts`
-   `io.cozy.files`
-   `io.cozy.jobs`
-   `io.cozy.konnectors`
-   `io.cozy.notifications`
-   `io.cozy.oauth.clients`
-   `io.cozy.permissions`
-   `io.cozy.sessions.logins`
-   `io.cozy.settings`
-   `io.cozy.shared`
-   `io.cozy.sharings`
-   `io.cozy.triggers`

Then, it creates some indexes for these doctypes, and some directories:

-   `/`, with the id `io.cozy.files.root-dir`
-   the trash, with the id `io.cozy.files.trash-dir`
-   `/Administrative`
-   `/Photos`
-   `/Photos/Uploaded from Cozy Photos`
-   `/Photos/Backed up from my mobile`

**The names are localized:** If a locale is provided through the CLI, the
directories will be created with names in this locale.

Then it creates the basic settings with the options from the CLI.

Finally, applications from the `--apps` CLI option are installed.

## Onboarding

At the end of the instance creation, the CLI returns a token, called
`registerToken`. The `registerToken` can be used to send a link to the user,
where they will be able to choose their password. After that, the instance
is said to be onboarded, and the user can use it normally.

## Destroying

An instance is destroyed through the command line. A confirmation is asked from
the CLI user unless the --force flag is passed:

```sh
$ cozy-stack instances destroy <domain>
```

## Blocking

If you manage several instances on your stack, you can block some instances.
When an instance is blocked (for whatever reason), the user cannot use his Cozy
anymore and has to wait to be unblocked.

```sh
$ cozy-stack instances modify <domain> --blocked
```

A `blocking_reason` can be added for specific cases and to inform a user the
reason of the blocking.

Several pre-included reasons are included, but you can define your owns if needed. These reasons are:
- `"LOGIN_FAILED"`: The instance was blocked because of too many login attempts
- `"PAYMENT_FAILED"`: The instance waits for a payment to be done
- `"UNKNOWN"`: The instance was blocked for an unknown reason

## Modifying

If you want to edit an instance, see [`instances modify manpage`](cli/cozy-stack_instances_modify.md).

## Admin endpoints

Several admin endpoints are available. This list may not be exhaustive.

### GET /instances/:app-slug/:version

Returns all the instances using slug/version pair

#### Request

```http
GET /instances/drive/1.0.0 HTTP/1.1
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

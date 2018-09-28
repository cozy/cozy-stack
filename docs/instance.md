[Table of contents](README.md#table-of-contents)

# Instances

**TODO** it's still a work in progress that needs to be completed.

A single cozy-stack can manage several instances. Requests to different
instances are identified through the `Host` HTTP Header, any reverse proxy
placed in front of the cozy-stack should forward this header.

To simplify development, a `dev` instance name is used when no Host Header is
provided. This behaviour will be kept when the stack is started in dev mode but
will be blocked in production environment.

**Exemple:**

-   `curl -H "Host: bob.cozycloud.cc" localhost:8080` →
    `localhost:5984/bob-cozycloud.cc`
-   `curl -H "Host: alice.cozycloud.cc" localhost:8080` →
    `localhost:5984/alice-cozycloud.cc`
-   `curl localhost:8080` → `localhost:5984/dev` (in dev mode only)

## Creation

An instance is created on the command line:

```sh
$ cozy-stack instances add <domain>
```

With some possible additional options

-   `--locale <lang>`
-   `--tz <timezone>`
-   `--email <email>`
-   `--apps <app1,app2,app3>`

It registers the instance in a global couchdb database `global/instances`

```json
{
    "hostname": "example.cozycloud.cc",
    "dbprefix": "example-clozycloud-cc/",
    "fsroot": "/var/lib/cozy/example.cozycloud.cc/fs/"
}
```

and creates the proper databases ($PREFIX/$DOCTYPE) for these doctypes:

-   `io.cozy.apps`
-   `io.cozy.files`
-   `io.cozy.notifications`
-   `io.cozy.settings`

Then, it creates the following indexes for these doctypes :

-   **TODO :** complete this list of indexes

Then, it creates some directories:

-   `/`, with the id `io.cozy.files.root-dir`
-   `/Apps`, with the id `io.cozy.files.apps-dir`
-   `/Documents`, with the id `io.cozy.files.documents-dir`
-   `/Documents/Downloads`, with the id `io.cozy.files.downloads-dir`
-   `/Documents/Pictures`, with the id `io.cozy.files.pictures-dir`
-   `/Documents/Music`, with the id `io.cozy.files.music-dir`
-   `/Documents/Videos`, with the id `io.cozy.files.videos-dir`

**The ids are forced to known values:** even if these directories are moved or
renamed, they can still be found for the permissions.

**The names are localized:** If a locale is provided through the CLI, the
directories will be created with names in this locale. Otherwise, theses
directories will be created in english and renamed to localized name the first
time the locale is set (during onboarding).

Then it creates the basic settings

-   `email` if an email was provided through the CLI
-   `locale` if a locale was provided through the CLI
-   `tz` if a timezone was provided through the CLI

Settings are created as named id in the `$PREFIX/io.cozy.settings` database.
During onboarding, the fields will be prefilled with these value if they were
provided.

Finally, applications from the `--apps` CLI option are installed.

---

## Renaming

An instance is renamed through the command line.

```sh
$ cozy-stack instances rename <olddomain> <newdomain>
```

Renaming an instance only change the HostName in global/instances base.

---

## Destroying

An instance is destroyed through the command line. A confirmation is asked from
the CLI user unless the --yes flag is passed

```sh
$ cozy-stack instances destroy <domain>
```

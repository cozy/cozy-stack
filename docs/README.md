---
title: Cozy-Stack documentation - Table of contents
---

## How-to guides

### Usage

-   [Install the cozy-stack](INSTALL.md)
-   [Configuration file](config.md)
-   [Managing Instances](instance.md)
-   [Security](security.md)
-   [Manpages of the command-line tool](cli/cozy-stack.md)

### For developpers

-   [Develop a client-side app](client-app-dev.md)
-   [Running and building Docker images](docker.md)
-   [Adding a new doctype](doctype.md)
-   [Working the stack assets](assets.md)
-   [Build a release](release.md)
-   [The contributing guide](CONTRIBUTING.md)

## Explanation

### Up-to-date

-   [Sharing design](sharing-design.md)
-   [Workflow of the konnectors](konnectors-workflow.md)

### Archives

These pages are the results of studies we made. They may be outdated and are
kept as an archive to help understanding what were out original intentions when
designing new services.

-   [General overview of the initial architecture](archives/architecture.md)
-   [Onboarding with an application](archives/onboarding.md)
-   [Moving](archives/moving.md)
-   [Golang Couchdb Plugins](archives/couchdb-plugins.md)
-   [Konnectors design](archives/konnectors-design.md)
-   [Replication](archives/replication.md)

## Reference

### List of services

-   `/auth` - [Authentication & OAuth](auth.md)
-   `/apps` - [Applications Management](apps.md)
    -   [Apps registry](registry.md)
    -   [Konnectors](konnectors.md)
-   `/data` - [Data System](data-system.md)
    -   [Mango](mango.md)
    -   [CouchDB Quirks](couchdb-quirks.md) &
        [PouchDB Quirks](pouchdb-quirks.md)
-   `/files` - [Virtual File System](files.md)
    -   [References of documents in VFS](references-docs-in-vfs.md)
-   `/intents` - [Intents](intents.md)
-   `/jobs` - [Jobs](jobs.md)
    -   [Workers](workers.md)
-   `/move` - [Move, export and import an instance](move.md)
-   `/notifications` - [Notifications](notifications.md)
-   `/permissions` - [Permissions](permissions.md)
-   `/public` - [Public](public.md)
-   `/realtime` - [Realtime](realtime.md)
-   `/remote` - [Proxy for remote data/API](remote.md)
-   `/settings` - [Settings](settings.md)
    -   [Terms of Services](user-action-required.md)
-   `/sharings` - [Sharing](sharing.md)

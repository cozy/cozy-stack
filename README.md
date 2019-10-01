Cozy Cloud
==========

[![GoDoc](https://godoc.org/github.com/cozy/cozy-stack?status.svg)](https://godoc.org/github.com/cozy/cozy-stack)
[![Build Status](https://github.com/cozy/cozy-stack/workflows/CI/badge.svg)](https://github.com/cozy/cozy-stack/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/cozy/cozy-stack)](https://goreportcard.com/report/github.com/cozy/cozy-stack)


## What is Cozy?

![Cozy Logo](https://cdn.rawgit.com/cozy/cozy-guidelines/master/templates/cozy_logo_small.svg)

[Cozy](https://cozy.io) is a platform that brings all your web services in the
same private space. With it, your web apps and your devices can share data
easily, providing you with a new experience. You can install Cozy on your own
hardware where no one profiles you.


## What is the Cozy-Stack

It is the core server of the Cozy platform. It consists of a single process, *the Cozy stack*. 

[Full Cozy-Stack documentation here](https://docs.cozy.io/en/cozy-stack/).

The Cozy-Stack is in charge of serving the Web applications users have installed from the application store.

It provides its services through a REST API that allows to:

 - create, update, delete documents inside the database;
 - authenticate users and client applications;
 - send emails;
 - launch jobs on the server. Connectors that import data from remote websites are some sort of jobs. Jobs can be one time tasks (sending a message) or periodic tasks. Some jobs, like the connectors, that require executing third party code on the server side, are sandboxed (we use `nsjail` for now).
 - …

The Cozy-Stack also allows to access the database replication API, allowing to sync documents between the server and local databases, for example in mobile clients.

Two authentication methods are available:

 - Web applications running on the server get a session token when the user log in;
 - OAuth2 for other applications.

Feel free to [open an issue](https://github.com/cozy/cozy-stack/issues/new)
for questions and suggestions.


## Installing a `cozy-stack`

You can follow the [Install guide](docs/INSTALL.md) and the [configuration
documentation](docs/config.md).


## How to contribute?

We are eager for contributions and very happy when we receive them! It can
code, of course, but it can also take other forms. The workflow is explained
in [the contributing guide](docs/CONTRIBUTING.md).


## Community

You can reach the Cozy Community by:

* Chatting with us on IRC #cozycloud on irc.freenode.net
* Posting on our [Forum](https://forum.cozy.io)
* Posting issues on the [Github repos](https://github.com/cozy/)
* Mentioning us on [Twitter](https://twitter.com/cozycloud)


## License

Cozy is developed by Cozy Cloud and distributed under the AGPL v3 license.

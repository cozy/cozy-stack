Cozy Cloud
==========

[![Build Status](https://travis-ci.org/nono/cozy-stack.svg?branch=master)](https://travis-ci.org/nono/cozy-stack)


## What is Cozy?

![Cozy Logo](assets/images/happycloud.png)

[Cozy](https://cozy.io) is a platform that brings all your web services in the
same private space.  With it, your web apps and your devices can share data
easily, providing you with a new experience. You can install Cozy on your own
hardware where no one profiles you.


## And what about this repository?

This repository contains thoughts for a new version of Cozy Cloud which aims
to be simpler for hosting thousands of instances. It should also bring
multi-users for self-hosted and improve many things, starting with security
and reliability. You can find:

- [some notes about the architecture](doc/architecture.md)
- some code in Go to help me immerse in the new architecture.

Feel free to [open an issue](https://github.com/nono/cozy-stack/issues/new)
for questions and suggestions.

There are some useful commands to know in order to play with the go code:

```bash
go get -u ./...         # To install the go dependencies
go test -v ./...        # To launch the tests
go run main.go serve    # To start the API server
```


## Community

You can reach the Cozy Community by:

* Chatting with us on IRC #cozycloud on irc.freenode.net
* Posting on our [Forum](https://forum.cozy.io)
* Posting issues on the [Github repos](https://github.com/cozy/)
* Mentioning us on [Twitter](https://twitter.com/mycozycloud)


## License

Cozy is developed by Cozy Cloud and distributed under the AGPL v3 license.

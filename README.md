Cozy Cloud
==========

[![Build Status](https://travis-ci.org/cozy/cozy-stack.svg?branch=master)](https://travis-ci.org/cozy/cozy-stack)


## What is Cozy?

![Cozy Logo](assets/images/happycloud.png)

[Cozy](https://cozy.io) is a platform that brings all your web services in the
same private space. With it, your web apps and your devices can share data
easily, providing you with a new experience. You can install Cozy on your own
hardware where no one profiles you.


## And what about this repository?

This repository contains thoughts for a new version of Cozy Cloud which aims
to be simpler for hosting thousands of instances. It should also bring
multi-users for self-hosted and improve many things, starting with security
and reliability. You can find:

- [some notes about the architecture](doc/architecture.md)
- some code in Go to help me immerse in the new architecture.

Feel free to [open an issue](https://github.com/cozy/cozy-stack/issues/new)
for questions and suggestions.

There are some useful commands to know in order to play with the go code:

```bash
go get -u github.com/cozy/cozy-stack
cd $GOPATH/src/github.com/cozy/cozy-stack

go get -t -u ./...      # To install or update the go dependencies
go test -v ./...        # To launch the tests
go run main.go serve    # To start the API server
godoc -http=:6060       # To start the documentation server
                        # Open http://127.0.0.1:6060/pkg/github.com/cozy/cozy-stack/

```

The swagger description of the REST API can be displayed with those commands:

```bash
go get -u github.com/go-swagger/go-swagger/cmd/swagger
git clone git@github.com:swagger-api/swagger-ui.git
mkdir -p swagger-ui/dist/specs
swagger generate spec -o swagger-ui/dist/specs/swagger.json
go get github.com/mholt/caddy/caddy
cd swagger-ui/dist && caddy
xdg-open http://localhost:2015/index.html?url=http://localhost:2015/specs/swagger.json
```


## Community

You can reach the Cozy Community by:

* Chatting with us on IRC #cozycloud on irc.freenode.net
* Posting on our [Forum](https://forum.cozy.io)
* Posting issues on the [Github repos](https://github.com/cozy/)
* Mentioning us on [Twitter](https://twitter.com/mycozycloud)


## License

Cozy is developed by Cozy Cloud and distributed under the AGPL v3 license.

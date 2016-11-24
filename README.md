Cozy Cloud
==========

[![GoDoc](https://godoc.org/github.com/cozy/cozy-stack?status.svg)](https://godoc.org/github.com/cozy/cozy-stack)
[![Build Status](https://travis-ci.org/cozy/cozy-stack.svg?branch=master)](https://travis-ci.org/cozy/cozy-stack)
[![codecov](https://codecov.io/gh/cozy/cozy-stack/branch/master/graph/badge.svg)](https://codecov.io/gh/cozy/cozy-stack)


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

- [some notes about the architecture](docs/architecture.md)
- some code in Go to help me immerse in the new architecture.

Feel free to [open an issue](https://github.com/cozy/cozy-stack/issues/new)
for questions and suggestions.

## Dependencies

* CouchDB 2.0.0

To install CouchDB 2.0.0 through Docker, take a look at our [Docker specific documentation](docs/docker.md).

## Installing a `cozy-stack`

We do not yet provide releases binaries, but we will soon and you won't have to install go in order to run `cozy-stack`

### Using `go`

[Install go](https://golang.org/doc/install), version >= 1.7. With `go` installed and configured, you can run the following command:

```
go get github.com/cozy/cozy-stack
```

This will fetch the sources in `$GOPATH/src/github.com/cozy/cozy-stack` and build a binary in `$GOPATH/bin/cozy-stack`.

Don't forget to add your `$GOPATH` to your `$PATH` in your `*rc` file so that you can execute the binary without entering its full path.

```
export PATH="$GOPATH:$PATH"
```

### Add an instance and run

You can configure your `cozy-stack` using a configuration file or different comand line arguments. You can have more informations on our [Configuration page](docs/config.md).

Assuming CouchDB is installed and running on default port `5984`, you can a *dev* instance:

```bash
cozy-stack instances add dev  # assuming couchdb is running
```

Then run the server with:

```bash
cozy-stack serve
```

The cozy-stack server listens on http://localhost:8080/ by default. See `cozy-stack --help` for more informations.

Make sure the full stack is up with:

```bash
curl -H 'Accept: application/json' 'http://localhost:8080/status/'
```

## Configuration

See [configuration documentation](/docs/config.md).

## How to contribute?

We are eager for contributions and very happy when we receive them! It can
code, of course, but it can also take other forms. The workflow is explained
in [the contributing guide](CONTRIBUTING.md).

There are some useful commands to know in order to develop with the go code of cozy-stack:

```bash
go get -u github.com/cozy/cozy-stack
cd $GOPATH/src/github.com/cozy/cozy-stack

go get -t -u ./...      # To install or update the go dependencies
go test -v ./...        # To launch the tests
go run main.go serve    # To start the API server
godoc -http=:6060       # To start the documentation server
                        # Open http://127.0.0.1:6060/pkg/github.com/cozy/cozy-stack/
```


## Community

You can reach the Cozy Community by:

* Chatting with us on IRC #cozycloud on irc.freenode.net
* Posting on our [Forum](https://forum.cozy.io)
* Posting issues on the [Github repos](https://github.com/cozy/)
* Mentioning us on [Twitter](https://twitter.com/mycozycloud)


## License

Cozy is developed by Cozy Cloud and distributed under the AGPL v3 license.

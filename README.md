Cozy Cloud
==========

[![GoDoc](https://godoc.org/github.com/cozy/cozy-stack?status.svg)](https://godoc.org/github.com/cozy/cozy-stack)
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

- [some notes about the architecture](docs/architecture.md)
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


## Requirements

* Go 1.7.1
* CouchDB 2.0.0


## With Docker

### Running a CouchDB instance

This will run a new instance of CouchDB in `single` mode (no cluster) and in `admin-party-mode` (no user).

```bash
$ docker run -d \
    --name cozy-stack-couch \
    -p 5984:5984 \
    -v $HOME/.cozy-stack-couch:/opt/couchdb/data \
    klaemo/couchdb:2.0.0
$ curl -X PUT http://127.0.0.1:5984/{_users,_replicator,_global_changes}
```

Verify your installation at: http://127.0.0.1:5984/_utils/#verifyinstall


### Building a `cozy-stack` binary

To build a `cozy-stack` binary with Docker using the **local** source code, run:

```bash
# From your cozy-stack developement folder
docker run -it --rm --name cozy-stack \
    -v $(pwd):/go/src/github.com/cozy/cozy-stack \
    -v $(pwd):/go/bin \
    golang:1.7.1 \
    go get -v github.com/cozy/cozy-stack
```

The `cozy-stack` binary is now present in the current folder.

Create a *dev* instance:

```bash
./cozy-stack instances add dev  # assuming couchdb is running
```

Then run the server with:

```bash
./cozy-stack serve
```

The cozy-stack server listens on http://localhost:8080/ by default.

Make sure the full stack is up with:

```bash
curl -H 'Accept: application/json' 'http://localhost:8080/status/'
```

## Building a release

To build a release of cozy-stack, a `build.sh` script can automate the work. The `release` option of this script will generate a binary with a name containing the version of the file, along with a SHA-256 sum of the binary.

See `./build.sh --help` for more informations.

```sh
COZY_ENV=development GOOS=linux GOARCH=arm64 ./build.sh release
```

The version string is deterministic and reflects entirely the state of the working-directory from which the release is built from. It is generated using the following format:

        <TAG>[-<NUMBER OF COMMITS AFTER TAG>][-dirty][-dev]

Where:

 - `<TAG>`: closest annotated tag of the current working directory. If no tag is present, is uses the string "v0". This is not allowed in a production release.
 - `<NUMBER OF COMMITS AFTER TAG>`: number of commits after the closest tag if the current working directory does not point exactly to a tag
 - `dirty`: added if the working if the working-directory is not clean (contains un-commited modifications). This is not allowed in production release.
 - `dev`: added for a development mode relase

## How to contribute?

We are eager for contributions and very happy when we receive them! It can
code, of course, but it can also take other forms. The workflow is explained
in [the contributing guide](CONTRIBUTING.md).


## Community

You can reach the Cozy Community by:

* Chatting with us on IRC #cozycloud on irc.freenode.net
* Posting on our [Forum](https://forum.cozy.io)
* Posting issues on the [Github repos](https://github.com/cozy/)
* Mentioning us on [Twitter](https://twitter.com/mycozycloud)


## License

Cozy is developed by Cozy Cloud and distributed under the AGPL v3 license.

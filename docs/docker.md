[Table of contents](README.md#table-of-contents)

# Docker

This page list various operations that can be automated _via_ Docker.

## Running a CouchDB instance

This will run a new instance of CouchDB in `single` mode (no cluster) and in
`admin-party-mode` (no user). This command exposes couchdb on the port `5984`.

```bash
$ docker run -d \
    --name cozy-stack-couch \
    -p 5984:5984 \
    -v $HOME/.cozy-stack-couch:/opt/couchdb/data \
    apache/couchdb:2.2
$ curl -X PUT http://127.0.0.1:5984/{_users,_replicator,_global_changes}
```

Verify your installation at: http://127.0.0.1:5984/_utils/#verifyinstall.

## Building a cozy-stack _via_ Docker

Warning, this command will build a linux binary. Use
[`GOOS` and `GOARCH`](https://golang.org/doc/install/source#environment) to
adapt to your own system.

```bash
# From your cozy-stack developement folder
docker run -it --rm --name cozy-stack \
    -v $(pwd):/go/src/github.com/cozy/cozy-stack \
    -v $(pwd):/go/bin \
    golang:1.10 \
    go get -v github.com/cozy/cozy-stack
```

## Publishing a new cozy-app-dev image

```bash
./scripts/build.sh docker-dev
docker push cozy/cozy-app-dev
```

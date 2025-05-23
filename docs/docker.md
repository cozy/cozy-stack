[Table of contents](README.md#table-of-contents)

# Docker

This page list various operations that can be automated _via_ Docker when
developing cozy-stack.

For docker usage in production to self-host your cozy instance, please refer
to our [Self Hosting Documentation](https://docs.cozy.io/en/tutorials/selfhosting/).

## Running a CouchDB instance

This will run a new instance of CouchDB in `single` mode (no cluster). This
command exposes couchdb on the port `5984`.

```bash
$ docker run -d \
    --name cozy-stack-couch \
    -p 5984:5984 \
    -e COUCHDB_USER=admin -e COUCHDB_PASSWORD=password \
    -v $HOME/.cozy-stack-couch:/opt/couchdb/data \
    couchdb:3.3
$ curl -X PUT http://admin:password@127.0.0.1:5984/{_users,_replicator}
```

Verify your installation at: http://127.0.0.1:5984/_utils/#verifyinstall.

Note: for running some unit tests, you will need to use `--net=host` instead of
`-p 5984:5984` as we are using CouchDB replications and CouchDB will need to be
able to open a connexion to the stack.

## Building a cozy-stack _via_ Docker

Warning, this command will build a linux binary. Use
[`GOOS` and `GOARCH`](https://golang.org/doc/install/source#environment) to
adapt to your own system.

```bash
# From your cozy-stack developement folder
docker run -it --rm --name cozy-stack \
    --workdir /app \
    -v $(pwd):/app \
    -v $(pwd):/go/bin \
    golang:1.24 \
    go get -v github.com/cozy/cozy-stack
```

## Publishing a new cozy-app-dev image

We publish the cozy-app-dev image when we release a new version of the stack.
See `scripts/docker/cozy-app-dev/release.sh` for details.

## Docker run and url name for cozy-app-dev

A precision for the app name:

```bash
docker run --rm -it -p 8080:8080 -v "$(pwd)/build":/data/cozy-app/***my-app*** cozy/cozy-app-dev
```

***my-app*** will be the first part of: ***my-app***.cozy.localhost:8080

## Only-Office document server

The `cozy/onlyoffice-dev` docker image can be used for local development on
Linux (the `--net=host` option doesn't work on macOS). Just start it with:

```bash
$ docker run -it --rm --name=oodev --net=host cozy/onlyoffice-dev
```

and run the stack with:

```bash
$ cozy-stack serve --disable-csp --onlyoffice-url=http://localhost:8000 --onlyoffice-inbox-secret=inbox_secret --onlyoffice-outbox-secret=outbox_secret
$ cozy-stack features defaults '{"drive.office": {"enabled": true, "write": true}}'
```

If you need to rebuild it, you can do that with:

```bash
$ cd scripts/onlyoffice-dev
$ docker build -t "cozy/onlyoffice-dev" .
```

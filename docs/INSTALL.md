[Table of contents](README.md#table-of-contents)

# How to install Cozy-stack?

## Dependencies

-   A reverse-proxy (nginx, caddy, haproxy, etc.)
-   A SMTP server
-   CouchDB 3
-   Git
-   Image Magick (and the Lato font)

To install CouchDB 3 through Docker, take a look at our
[Docker specific documentation](docker.md).

**Note:** to generate thumbnails for heic/heif images, the version 6.9+ of
Image Magick is required.

## Install for self-hosting

We have started to write documentation on how to install cozy on your own
server. We have [guides for
self hosting](https://docs.cozy.io/en/tutorials/selfhosting/), either on
Debian with precompiled binary packages of from sources on Ubuntu.
Don't hesitate to [report issues](https://github.com/cozy/cozy.github.io/issues/new) with them.
It will help us improve documentation.

## Install for development / local tests

### Install the binary

You can either download the binary or compile it.

#### Download an official release

You can download a `cozy-stack` binary from our official releases:
https://github.com/cozy/cozy-stack/releases. It is a just a single executable
file (choose the one for your platform). Rename it to cozy-stack, give it the
executable bit (`chmod +x cozy-stack`) and put it in your `$PATH`.
`cozy-stack version` should show you the version if every thing is right.

#### Compile the binary using `go`

You can compile a `cozy-stack` from the source.
First, you need to [install go](https://golang.org/doc/install), version >= 1.15. With `go`
installed and configured, you can run the following commands:

```
git clone git@github.com:cozy/cozy-stack.git
cd cozy-stack
make
```

This will fetch the sources and build a binary in `$GOPATH/bin/cozy-stack`.

Don't forget to add your `$GOPATH/bin` to your `$PATH` at the end of your `*rc` file so
that you can execute the binary without entering its full path.

```
export PATH="$(go env GOPATH)/bin:$PATH"
```

##### Troubleshooting

Check if you don't have an alias "go" configurated in your `*rc` file.

### Add an instance for testing

You can configure your `cozy-stack` using a configuration file or different
comand line arguments. Assuming CouchDB is installed and running on default port
`5984`, you can start the server:

```bash
cozy-stack serve
```

And then create an instance for development:

```bash
make instance
```

The cozy-stack server listens on http://cozy.localhost:8080/ by default. See
`cozy-stack --help` for more informations.

The above command will create an instance on http://cozy.localhost:8080/ with the
passphrase `cozy`. By default this will create a `storage/` entry in your current directory, containing all your instances by their URL. An instance "cozy.localhost:8080" will have its stored files in `storage/cozy.localhost:8080/`. Installed apps will be found in the `.cozy_apps/` directory of each instance.

Make sure the full stack is up with:

```bash
curl -H 'Accept: application/json' 'http://cozy.localhost:8080/status/'
```

You can then remove your test instance:

```bash
cozy-stack instances rm cozy.localhost:8080
```

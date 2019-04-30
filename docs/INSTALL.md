# How to install Cozy-Stack?

## Dependencies

-   A reverse-proxy (Nginx, Caddy, HAProxy, etc.)
-   A SMTP server
-   CouchDB 2
-   Image Magick

**Note:** to generate thumbnails for heic/heif images, the version 7.0.7-22+ of
Image Magick is required.

## Install using Docker

See the [Docker dedicated documentation](https://github.com/cozy/cozy-stack/blob/master/docs/docker.md)

## Install using Debian package

See the [Debian self-hosting guide](https://docs.cozy.io/en/tutorials/selfhost-debian/)

## Install the binary

The cozy-stack code is written in Go and released as a single binary. You can compile it yourself or download it

### Download an official release

Here are the official releases: https://github.com/cozy/cozy-stack/releases.

- Download the right one for your platform
- Rename it to cozy-stack
- Make it executable (`chmod +x cozy-stack`)
- Add it to your PATH (`mv cozy-stack /usr/local/bin`)
- The command `cozy-stack version` should show you the downloaded version number

### Compile from sources

Install git. [Install go](https://golang.org/doc/install), version >= 1.11. Then you can run the following command:

```
go get -u github.com/cozy/cozy-stack
```

This will fetch the sources in `$GOPATH/src/github.com/cozy/cozy-stack` and
build a binary in `$GOPATH/bin/cozy-stack`.

Don't forget to add your `$GOPATH` to your `$PATH` in your `*rc` file so that
you can execute the binary without entering its full path.

```
export PATH="$(go env GOPATH)/bin:$PATH"
```

### After installing the binary

You can configure your `cozy-stack` using [a configuration file](https://github.com/cozy/cozy-stack/blob/master/cozy.example.yaml) or different
command line arguments. Assuming CouchDB is installed and running on default port
`5984`, you can start the server:

```bash
cozy-stack serve
```

And then create an instance for development:

```bash
cozy-stack instances add --apps home,drive,settings,store --passphrase cozy "cozy.tools:8080"
```

The cozy-stack server listens on http://cozy.tools:8080/ by default. See
`cozy-stack --help` for more informations.

The above command will create an instance on http://cozy.tools:8080/ with the
passphrase `cozy`.

Make sure the full stack is up with:

```bash
curl -H 'Accept: application/json' 'http://cozy.tools:8080/status/'
```

You can then remove your test instance:

```bash
cozy-stack instances rm cozy.tools:8080
```

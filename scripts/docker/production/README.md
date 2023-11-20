# cozy-stack docker production image

The files in this directory let you build a cozy-stack production image.

## Variables

The following variables can be used to customize the image:

- `COUCHDB_PROTOCOL`: protocol used to access couchdb (http or https).
  defaults to `http`
- `COUCHDB_HOST`: CouchDB Host (defaults to `couchdb`)
- `COUCHDB_PORT`: CouchDB Port to use (defaults to `5984`)
- `COUCHDB_USER`: CouchDB User (defaults to `cozy`)
- `COUCHDB_PASSWORD`: CouchDB password (defaults to `cozy`)
- `COZY_ADMIN_PASSPHRASE`: The admin passphrase. If unset, a random password will be used and echoed in container logs
- `START_EMBEDDED_POSTFIX`: Set it to "true" to start a out-only smtp postfix relay. Not started by default.
- `LOCAL_USER_ID`: UID of the user running cozy-stack (defaults to 3552)
- `LOCAL_GROUP_ID`: GID of the user running cozy-stack (defaults to 3552)
- All variables that can be used to configure cozy-stack. Usually they are the command line options in upper case prefixed by COZY_ (and dashes replaced with underscores). Refer to our [documentation](https://docs.cozy.io/en/cozy-stack/config/#stack-endpoints)

## Running the cozy-stack production image

```bash
docker run --rm \
    -e COUCHDB_HOST=couchdb \
    -e COUCHDB_USER=cozy \
    -e COUCHDB_PASSWORD=P@ssw0rd \
    -e COZY_COUCHDB_URL=http://cozy:p@ssw0rd@couchdb:5984 \
    -e COZY_FS_URL=/var/lib/cozy \
    -v ./volumes/cozy-stack/data:/var/lib/cozy \
    -v ./volumes/cozy-stack/config:/etc/cozy/ \
    cozy/cozy-stack:latest
```

you can configure the stack by adding a `cozy.yml` file in your cozy-stack/config volume.

## Building the image

simply execute the following command from the cozy-stack repo root

```bash
docker build -t "cozy/cozy-stack:latest"  -f scripts/docker/production/Dockerfile .
```

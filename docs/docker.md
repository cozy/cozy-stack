[Table of contents](README.md#table-of-contents)

# Docker

Production and development images for [Cozy Stack](https://cozy.io), Dockerfiles, docker-compose.yml & co lives [there](https://github.com/cozy/cozy-stack/tree/master/scripts)

## Requirements

You need docker and using docker-compose is a good idea. Here are the versions used for develoment

```
➜  docker git:(master) docker -v
Docker version 18.09.5, build e8ff056
➜  docker git:(master) docker-compose version
docker-compose version 1.24.0, build 0aa5906
docker-py version: 3.7.2
CPython version: 3.6.7
OpenSSL version: OpenSSL 1.1.1  11 Sep 2018
```

Cozy Stack needs at least a CouchDB 2.3 to run.

A reverse proxy is strongly recommended for HTTPS, Caddy being a good pick since it's able to provide on-demand TLS. Though any other can do.

Redis for caching is optional.

## Supported tags

- `development`. Running `cozy-stack` compiled against `master` branch in `development` mode, a.k.a without enforcing HTTPS and other security features, so it can be run localy without certificates. Plus `couchdb` and `mailhog`, so it's a all in one environment for developers.

- `latest`. Running `cozy-stack` only compiled against `master` branch in `production` mode. *Use this one*.

- `<version>` tag. Each one match a source code tag and are compiled in production mode, enforcing HTTPS and so on. Use this one.
- `latest` tag. It matches the `master` branch of the source code, also compiled in production mode. Can be unstable, but you'll have the latest features here.
- `development`. It also matches the `master` branch of the source code, but it's compiled in development mode, hence not enforcing HTTPS so it can be used localy without settings up

## The secure and easy way

The production image enforce security, but can be difficult to use since it requires setting up a CouchDB, a reverse proxy, getting a certificate. So we provide an easy setup using docker-compose that will get a Cozy Stack + CouchDB database + Redis cache + Caddy reverse proxy started.

Here's the [docker-compose.yml](https://raw.githubusercontent.com/cozy/cozy-stack/master/docker/docker-compose.yml) which rely on an `.env` file for parameters, a [sample is here](https://raw.githubusercontent.com/cozy/cozy-stack/master/docker/env.sample). On your server you need docker & docker-compose installed, plus a `*.your.domain` DNS record.

```bash
# Get the files
git clone https://github.com/cozy/cozy-stack
cd cozy-stack/scripts

cp env.sample .env
# Edit the .env file to specify your own values for passwords, mail settings, ...
# Avoid special characters in the passwords, it break the scripts. But make it long, something like the output of "uuidgen" will do.

# Start the environment
docker-compose up -d

# Create a first instance
source .env
docker-compose exec -T stack bash -c "yes $COZY_ADMIN_PASSPHRASE | cozy-stack instances add --email you@$DOMAIN --passphrase AVerySecretPasswordHere YourFirstInstanceName.$DOMAIN --apps home,drive,settings,store,photos"
```

Then heads to https://YourFirstInstanceName.stack.your.domain.

Whenever done, here's the cleanup

```bash
docker-compose down
sudo rm -rf volumes/
```

## The secure do it yourself way

Provided you already got a server running with Docker installed, a reverse proxy running, a domain set up, and a wildcard certificate for `*.cozy.your.domain`, and set up your reverse proxy to redirect all incoming `*.cozy.your.domain` traffic to `localhost:8080`, you have to

    docker run -d -e COUCHDB_USER=cozy -e COUCHDB_PASSWORD=cozy --name couch --volume $(pwd)/volumes/couchdb:/opt/couchdb/data couchdb
    docker run -d -p 127.0.0.1:8080:8080 -p 6060:6060 --link couch --name stack -e LOCAL_USER_ID=$(id -u) -e LOCAL_GROUP_ID=$(id -g) --volume $(pwd)/volumes/stack:/var/lib/cozy/data cozy/cozy-stack

An alternative to an expensive wildcard certificate can be Caddy Server as a reverse proxy. It's able to generate on demand certificates via the ACME protocol and LetsEncrypt. Meaning the first time you open an url, something like https://your-drive.cozy.your.domain, there'll be a few second lags for Caddy to grab the certificate, and you'll be good to go.

## The unsecure all-in-one usage for tests

This image is intended for development & local tests. It's compiled in development, hence some security features are disabled. Here's an HOWTO get started

```bash
# Start a CouchDB
docker run -d -e COUCHDB_USER=cozy -e COUCHDB_PASSWORD=cozy --name couchdb couchdb

# Start your cozy stack
docker run -d -p 80:8080 -p 6060:6060 --link couchdb --name stack cozy/cozy-stack:development

# Create your first cozy test instance
docker exec -ti stack cozy-stack instances add --passphrase test test.localhost --apps home,drive,settings,store,photos
```

Then heads to http://test.localhost. Whenever done, remove the containers

```bash
docker rm -f couchdb stack
```

To persist data you've to use volumes, meaning mounting relevant containers folders to local folders, something like

    docker run -d -e COUCHDB_USER=cozy -e COUCHDB_PASSWORD=cozy --name couchdb --volume $(pwd)/volumes/couchdb:/opt/couchdb/data couchdb
    docker run -d -p 80:8080 -p 6060:6060 --link couchdb --name stack -e LOCAL_USER_ID=$(id -u) -e LOCAL_GROUP_ID=$(id -g) --volume $(pwd)/volumes/stack:/var/lib/cozy/data cozy/cozy-stack

Removing `volumes/couchdb` afterward might requires root privileges, since CouchDB container creates files with UID:GID 5984:5984

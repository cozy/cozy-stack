[Table of contents](README.md#table-of-contents)

# Docker

Production and development images for [Cozy](https://cozy.io). Dockerfiles, docker-compose.yml & co lives in the `scripts/` directory of the [Cozy Stack source code](https://github.com/cozy/cozy-stack)

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

## Supported tags

- `<version>` tags. Each one match a source code tag and are compiled in production mode, enforcing HTTPS and so on. *We recommend using this one*.

- `latest` tag. Same as the `<version>` tags but compiled against `master` branch. You'll have the latest features but it can be unstable.

- `development`. Running `cozy-stack` compiled against `master` branch in `development` mode, a.k.a without enforcing HTTPS and other security features, so it can be run localy without certificates. Plus `couchdb` and `mailhog`, so it's a all-in-one environment for developers.

## Running Cozy on your server the secure and easy way

The production image enforce security, but can be difficult to use since it requires setting up a CouchDB, a reverse proxy, getting a certificate. So we provide an easy setup using docker-compose that will get a Cozy Stack + CouchDB database + Caddy reverse proxy started.

Here's the [docker-compose.yml](https://raw.githubusercontent.com/cozy/cozy-stack/master/docker/docker-compose.yml) which rely on an `.env` file for parameters, a [sample is here](https://raw.githubusercontent.com/cozy/cozy-stack/master/docker/env.sample). On a server with Git, Docker & docker-compose installed, reachable from Internet including a `*.stack.your.domain` DNS record:

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
docker-compose exec -T stack bash -c "yes $COZY_ADMIN_PASSPHRASE | cozy-stack instances add --email you@$DOMAIN --passphrase YourPassword YourFirstInstanceName.$DOMAIN --apps home,drive,settings,store,photos"
```

Then head to https://YourFirstInstanceName.stack.your.domain.

Whenever done, here's how to clean up

```bash
# Stop and remove containers
docker-compose down

# Remove the folder storing data. Requires root privileges since CouchDB uses UID/GID 5984
sudo rm -rf volumes/
```

## The secure do it yourself way

Here's a more detailed approach. You still need your server accessing Internet, with a domain set up. Plus a wildcard certificate for `*.cozy.your.domain`, and your reverse proxy redirecting all incoming `*.cozy.your.domain` trafic to `localhost:8080`.

Note: an alternative to a wildcard certificate can be [Caddy Server](https://caddyserver.com/) as a reverse proxy. It's able to generate on demand certificates via the ACME protocol and LetsEncrypt. Meaning the first time you open an url, something like https://YourFirstInstanceName-drive.cozy.your.domain, there'll be a few second lags for Caddy to grab the certificate, and you'll be good to go.

### Start a CouchDB

You can run it any way you want, here the [official CouchDB 2.3.1 installation documentation](https://docs.couchdb.org/en/2.3.1/install/unix.html). Since we're running Docker anyway, here's how to start it using the official container.

```bash
docker run -d -e COUCHDB_USER=cozy -e COUCHDB_PASSWORD=cozy --name couch -v $(pwd)/volumes/couchdb:/opt/couchdb/data couchdb:2.3
```

The data will be stored in `./volumes/couchdb`. Removing this folder will requires root privileges due to CouchDB using UID/GID 5984 in the container. Skip the `-v` option for ephemeral environment.

### Start a Cozy Stack

You'll want to publish the port 8080 for the web access. Port 6060 is optional, to access the API. Linking against CouchDB is required for an easy access between containers.

```bash
docker run -d -p 127.0.0.1:8080:8080 -p 6060:6060 --link couch --name stack -e LOCAL_USER_ID=$(id -u) -e LOCAL_GROUP_ID=$(id -g) --volume $(pwd)/volumes/stack:/var/lib/cozy/data  cozy/cozy-stack
```

If you'ld like to overwrite pieces of configuration, write down a `cozy.yaml.local` and mount it with this option `--volume $(pwd)/cozy.yaml.local:/etc/cozy/cozy.yaml.local`

The data, meaning the files living in the stack will be stored in `./volumes/stack`. Specifying `LOCAL_USER_ID` and `LOCAL_GROUP_ID` allow to use a specific UID & GID.

#### Create your first Cozy instance

Any `cozy-stack` command, including creating your first instance, can roughtly be executed prefixing it by `docker exec -ti stack `. For example:

```bash
docker exec -ti stack cozy-stack instances add --passphrase YourPassword YourFirstInstanceName.your.domain --apps home,drive,settings,store,photos
```

Once your first instance is created, if your reverse proxy is already configured, access it via https://YourFirstInstanceName.your.domain.

## The unsecure all-in-one usage for development & tests

The `development` tag is intended for development & tests. It directly includes CouchDB, plus MailHog, a webUI to view mails sent. It's compiled on master branch in development mode, hence some security features like enforcing HTTPS are disabled.

See the [development documentation](docs/client-app-dev/#with-docker) for its usage.

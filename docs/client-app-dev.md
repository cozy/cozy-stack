Develop a client-side application
=================================

This document describe a tool to run an environment in order to develop client-side application on the cozy-stack.

We provide two different ways to run this environment, either manually where you have to install part of the dependencies yourself or *via* a Docker image in which all dependencies are packed.

This environment will provide a running instance a http server serving both a specified directory of your application on `app.cozy.local:8080` and the `cozy-stack` on `cozy.local:8080` (you can change the hostname and port if you want, see below).


Manually
--------

To run the `scripts/cozy-app-dev.sh` directly on you system, you'll need to following dependencies:

  - `go`
  - `curl`
  - `git`
  - `couchdb2` (optionally installed on your computer, you need at least a running instance on couchdb2 though)

Examples:

```sh
$ ./scripts/cozy-app-dev.sh -d ~/code/myapp
```

You'll need to add the following line to your `/etc/hosts` file:

```
127.0.0.1  app.cozy.local,cozy.local
```

You can have more informations about the usage of this script with the following command:

```
$ ./scripts/cozy-app-dev.sh -h
```


With Docker
-----------

If you do not want to install the required dependencies, we provide a Docker image which encapsulates the dev script and all its dependencies.

To run a ephemeral instance, on the `$HOME/myapp` directory, use the following command (warning: all the data stored by your application in couchdb and the VFS won't remain after):

```sh
$ docker run -d \
    -p 8080:8080 \
    -v "$HOME/myapp":/data/cozy-app \
    cozy:cozy-app-dev
```

To keep your data even when stopping the container, run the following command:

```
$ docker run -d \
    -p 8080:8080 \
    -v "$HOME/myapp":/data/cozy-app \
    -v "$(pwd)":/usr/local/couchdb/data \
    cozy:cozy-app-dev
```

You can also expose the couchdb port (listening in the container on 5984) in order to access its admin page. For instance add `-p 1234:5984` to access to the admin interface on `http://localhost:1234/_utils`.

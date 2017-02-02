[Table of contents](README.md#table-of-contents)

{% raw %}

# Develop a client-side application

## Using `cozy-app-dev`

This document describe a tool to run an environment in order to develop client-side application on the cozy-stack.

We provide two different ways to run this environment, either manually where you have to install part of the dependencies yourself or *via* a Docker image in which all dependencies are packed.

This environment will provide a running instance a http server serving both a specified directory of your application on `app.cozy.local:8080` and the `cozy-stack` on `cozy.local:8080` (you can change the hostname and port if you want, see below).


### Manually

To run the `scripts/cozy-app-dev.sh` directly on you system, you'll need to following dependencies:

  - `go`
  - `curl`
  - `git`
  - `couchdb2`: you need at least a running instance of CouchDB 2

Examples:

```sh
$ ./scripts/cozy-app-dev.sh -d ~/code/myapp
```

You'll need to add the following line to your `/etc/hosts` file:

```
127.0.0.1  app.cozy.local cozy.local
```

If your CouchDB 2 instance is not running on `localhost:5984`, you can specify another host and port with the variables `COUCHDB_HOST` and `COUCHDB_PORT` like so:

```sh
$ COUCHDB_HOST=couchdb.local COUCHDB_PORT=1234 ./scripts/cozy-app-dev.sh -d ~/code/myapp
```

You can have more informations about the usage of this script with the following command:

```
$ ./scripts/cozy-app-dev.sh -h
```


### With Docker

If you do not want to install the required dependencies, we provide a Docker image which encapsulates the dev script and all its dependencies.

To run a ephemeral instance, on the `$HOME/myapp` directory, use the following command (warning: all the data stored by your application in couchdb and the VFS won't remain after):

```sh
$ docker run --rm -it \
    -p 8080:8080 \
    -v "$HOME/myapp":/data/cozy-app \
    cozy/cozy-app-dev
```

To keep your data even when stopping the container, run the following command:

```sh
$ docker run --rm -it \
    -p 8080:8080 \
    -v "$HOME/myapp":/data/cozy-app \
    -v "$(pwd)/db":/usr/local/couchdb/data \
    -v "$(pwd)/storage":/data/cozy-storage \
    cozy/cozy-app-dev
```

You can also expose the couchdb port (listening in the container on 5984) in order to access its admin page. For instance add `-p 1234:5984` to access to the admin interface on `http://localhost:1234/_utils`.

Make sure you application is built into `$HOME/myapp` (it should have an `index.html` file), otherwise it will not work. As an example, for the [Files application](https://github.com/cozy/cozy-files-v3/), it should be `$HOME/files/build`.


## Good practices for your application

When an application makes a request to the stack, like loading a list of
contacts, it sends two informations that will be used by the stack to allow or
deny the access:

- the user session cookie
- a token that identifies the application (only when the user is connected).

So, the application needs such a token. It also needs to know where to send
the requests for the stack (it can be guessed, but with the nested vs flat
subdomains structures, it's better to get the information from the stack). To
do that, when the application loads its HTML index file, the stack will parse
it as a template and will insert the relevant values.

- `{{.Token}}` will be replaced by the token for the application.
- `{{.Domain}}` will be replaced by the stack hostname.
- `{{.Locale}}` will be replaced by the locale for the instance.
- `{{.CozyBar}}` will be replaced by the JavaScript to inject the cozy-bar.

So, the `index.html` should probably looks like:

```html
<!DOCTYPE html>
<html lang="{{.Locale}}">
  <head>
    <meta charset="utf-8">
    <title>My Awesome App for Cozy</title>
    <link rel="stylesheet" src="//{{.Domain}}/settings/theme.css">
    <link rel="stylesheet" src="my-app.css">
    {{.CozyBar}}
    <script defer src="my-app.js"></script>
    <meta name="viewport" content="width=device-width, initial-scale=1">
  </head>
  <body>
    <div role="application" data-token="{{.Token}}" data-cozy-stack="{{.Domain}}">
    </div>
  </body>
</html>
```

And `my-app.js`:

```js
'use strict'

document.addEventListener('DOMContentLoaded', () => {
  const app = document.querySelector('[role=application]')
  cozy.init({
    cozyURL: app.dataset.cozyStack,
    token: app.dataset.token
  })
})

// ...
```

For the `theme.css` stylesheet, you can read the [settings documentation](settings.md).

{% endraw %}

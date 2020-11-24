[Table of contents](README.md#table-of-contents)

# Configuration

## Main Configuration file

You can configure your `cozy-stack` using a configuration file. This file should
be named `cozy.yaml`, `cozy.yml`, or `cozy.json` depending on the format of your chosing, and
should be present in one of these directories (ordered by priority):

-   `./.cozy`
-   `$HOME/.cozy`
-   `/etc/cozy`

The path of the configuration file can also be define from an absolute path
given by the `--config` (or `-c`) flag of the [cozy-stack command](./cli/cozy-stack_serve.md).

Note that is is possible to have an additional configuration file, with the
`.local` suffix. For example, it can be used to have `/etc/cozy/cozy.yml`
managed by a package manager, and `/etc/cozy/cozy.yml.local` for things that a
user can customize.

### Templating and Environment Variables

It is possible to pass environnment variable to this configuration using the
[template language of golang](https://golang.org/pkg/text/template/), delimited
by `{{` and `}}`.

The environment variables are available in the `.Env` variable. For instance the
text `{{.Env.COUCHDB_PASSPHRASE }}` will be replaced by the value of the
`COUCHDB_PASSPHRASE` environment variable. The template is evaluated at startup
of the stack.

### Values and Example

To see the detail of the available parameters available, you can see an example
of configuration in the [cozy.example.yaml](https://github.com/cozy/cozy-stack/blob/master/cozy.example.yaml)
file at the root of this repository.

This file contains all the parameters and fields that can be used to configure
the stack with some example values.

Some fields can be overriden by the flags of the
[cozy-stack serve command](cli/cozy-stack_serve.md).

## Stack endpoints

By default, `cozy-stack` use plain-text & local socket for client
(`localhost:8080`) and admin (`localhost:6060`) communications.

If you want to control a remote stack or using TLS to secure communications, you
can configure your `cozy-stack` client with the following CLI arguments or
environment variables.

| Argument                  | Env variable                                       | Default value | Usage |
|---------------------------|----------------------------------------------------|---------------|-------|
| `--host` / `--admin-host` | `COZY_HOST` / `COZY_ADMIN_HOST`                    | localhost     | `[http[s]://]<fqdn>[:<port>]` |
| `--port` / `--admin-port` | `COZY_PORT` / `COZY_ADMIN_PORT`                    | 8080 / 6060   |  |
|                           | `COZY_HOST_TIMEOUT` / `COZY_ADMIN_TIMOUT`          | 15s           | HTTP timeout to use<br />Must be [a valid golang duration](https://golang.org/pkg/time/#ParseDuration) like `10s` or `1m` |
|                           | `COZY_HOST_VALIDATE` / `COZY_ADMIN_VALIDATE`       | true          | Enable HTTPS certificate validation<br />Can also be set via host URL query part, like `https://localhost:6060?validate=false` |
|                           | `COZY_HOST_CA` / `COZY_ADMIN_CA`                   | none          | CA file to use for HTTPS certificate validation<br />Can also be set via host URL query part, like `https://localhost:6060?ca=<ca>` |
|                           | `COZY_HOST_CERT` / `COZY_ADMIN_CERT`               | none          | Client certificate to use<br />Can also be set via host URL query part, like `https://localhost:6060?cert=<cert>` |
|                           | `COZY_HOST_KEY` / `COZY_ADMIN_KEY`                 | none          | Client certificate to use<br />Can also be set via host URL query part, like `https://localhost:6060?key=<key>` |
|                           | `COZY_HOST_FINGERPRINT` / `COZY_ADMIN_FINGERPRINT` | none          | Hex-encoded SHA-256 key pinning to use<br />Can also be set via host URL query part, like `https://localhost:6060?fp=<fp>`<br /><br />You can get the fingerprint of a given certificate with<br />`openssl x509 -in <certificat.crt> -pubkey \| openssl pkey -pubin -outform der \| openssl dgst -sha256 -hex`<br />or directly from a private key with `openssl pkey -in <key.pem> -pubout -outform der \| openssl dgst -sha256 -hex` |

## Administration secret

To access to the administration API (the `/admin/*` routes), a secret passphrase
should be stored in a `cozy-admin-passphrase`. This file should be in one of the
configuration directories, along with the main config file.

The passphrase is stored in a salted-hashed representation using scrypt. To
generate this file, you can use the `cozy-stack config passwd [filepath]`
command. This command will ask you for a passphrase and will create the
`cozy-admin-passphrase` at the specified path.

You can use the `COZY_ADMIN_PASSWORD` env variable if you do not want to type
the passphrase each time you call `cozy-stack`.

### Example

```sh
$ mkdir ~/.cozy && cozy-stack config passwd ~/.cozy/cozy-admin-passphrase
Hashed passphrase will be writtent in ~/.cozy/cozy-admin-passphrase
Passphrase:
Confirmation:
$ cat ~/.cozy/cozy-admin-passphrase
scrypt$16384$8$1$936bd62faf633b5f946f653c21161a9b$4e0d11dfa5fc1676ed329938b11a6584d30e603e0d06b8a63a99e8cec392d682
```

## Hooks

Cozy-stack can run scripts on some events to customize it. The scripts must be
in the hooks directory defined in the config, have a predefined name, and be
executable. Then, they should be fired automatically. Let's the available hooks.

The `pre-add-instance` hook is run just before creating an instance. It can
prevent the command from running by exiting with non-zero status. It can be used
to check the domain for example. It is called with the following parameter:

1. the domain of the instance that will be created.

The `post-add-instance` hook is run just after an instance has been created.
installed. It can be used to setup DNS for this instance for example. It is
called with the following parameter:

1. the domain of the instance that has been created.

The `pre-remove-instance` hook is run just before destroying an instance. It can
prevent the command from running by exiting with non-zero status. It can be used
to make a backup of the instance before destroying it. It is called with the
following parameter:

1. the domain of the instance that will be destroyed.

The `post-remove-instance` hook is run just after an instance has been
destroyed. It can be used to do some cleanup. It is called with the following
parameter:

1. the domain of the instance that has been destroyed.

The `pre-install-app` hook is run just before installing an application, and can
prevent the command from running by exiting with non-zero status. It is called
with the following parameters:

1. the instance on which the application will be installed
2. the application name that will be installed.

The `post-install-app` hook is run just after an application has been installed.
It can be used for logging, notification, statistics, etc. It's also a good
place to add a vhost for an application in the reverse-proxy configuration, with
a TLS certificate. It is called with the following parameters:

1. the instance on which the application has been installed
2. the application name that has been installed.

The `pre-uninstall-app` hook is run just before uninstalling an application, and
can prevent the command from running by exiting with non-zero status. It is
called with the following parameters:

1. the instance on which the application will be uninstalled
2. the application name that will be uninstalled.

The `post-uninstall-app` hook is run just after an application has been
uninstalled. It can be used for cleaning the configuration of the reverse- proxy
for example. It is called with the following parameters:

1. the instance on which the application has been uninstalled
2. the application name that has been uninstalled.

## Customizing a context

### Intro

In the config file of cozy-stack, it's possible to declare some contexts, that
are a way to regroup some cozy instances to give similar configuration. For
example, it is possible to give a `default_redirection` that will be used
when the user logs into their cozy. You can find more example in the example
config file.

### Assets

The visual appearance of a cozy instance can be customized via some assets
(CSS, JS, images). These assets can be inserted from the command-line with the
[`cozy-stack assets add`](./cli/cozy-stack_assets_add.md)
command.

Here are a small list of assets that you may want to customize:

- `/styles/theme.css`: a CSS file where you can override the colors and put
  other CSS rules
- `/favicon-16x16.png` and `/favicon-32x32.png`: the two variants of the
  favicon
- `/apple-touch-icon.png`: the same but for Apple
- `/images/default-avatar.png`: the image to use as the default avatar.
- `/images/default-wallpaper.png`: the image to use as the default wallpapper
  on the home.
- `/images/icon-cozy-home.svg`: the home icon used and displayed by the cozy-bar.

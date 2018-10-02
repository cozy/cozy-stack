[Table of contents](README.md#table-of-contents)

# Configuration

## Main Configuration file

You can configure your `cozy-stack` using a configuration file. This file should
be named `cozy.yaml` or `cozy.json` depending on the format of your chosing, and
should be present in one of these directories (ordered by priority):

-   `./.cozy`
-   `$HOME/.cozy`
-   `/etc/cozy`

The path of the configuration file can also be define from an absolute path
given by the `--config` (or `-c`) flag of the [cozy-stack command](./cli/cozy-stack_serve.md).

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
of configuration in the [cozy.example.yaml](../cozy.example.yaml) file at the
root of this repository.

This file contains all the parameters and fields that can be used to configure
the stack with some example values.

Some fields can be overriden by the flags of the
[cozy-stack serve command](docs/cli/cozy-stack_serve.md).

## Stack endpoints

By default, `cozy-stack` use plain-text & local socket for client
(`localhost:8080`) and admin (`localhost:6060`) communications.

If you want to control a remote stack or using TLS to secure communications, you
can configure your `cozy-stack` client with the following CLI arguments or
environment variables.

<table>
    <thead>
        <tr>
            <th>Argument</th>
            <th>Env variable</th>
            <td>Default value</td>
            <td>Usage</td>
        </tr>
    </thead>
    <tbody>
        <tr>
            <th>--host / --admin-host</th>
            <th>COZY_HOST / COZY_ADMIN_HOST</th>
            <td>localhost</td>
<td>

`[http[s]://]<fqdn>[:<port>]`

</td>
        </tr>
        <tr>
            <th>--port / --admin-port</th>
            <th>COZY_PORT / COZY_ADMIN_PORT</th>
            <td>8080 / 6060</td>
            <td></td>
        </tr>
        <tr>
            <th></th>
            <th>COZY_HOST_TIMEOUT / COZY_ADMIN_TIMOUT</th>
            <td>15s</td>
<td>
            
HTTP timeout to use  
Must be [a valid golang duration](https://golang.org/pkg/time/#ParseDuration) like `10s` or `1m`
</td>
        </tr>
        <tr>
                    <th></th>
                    <th>COZY_HOST_VALIDATE / COZY_ADMIN_VALIDATE</th>
                    <td>true</td>
<td>

Enable HTTPS certificate validation  
Can also be set via host URL query part, like
`https://localhost:6060?validate=false`

</td>
        </tr>
        <tr>
                    <th></th>
                    <th>COZY_HOST_CA / COZY_ADMIN_CA</th>
                    <td>none</td>
<td>

CA file to use for HTTPS certificate validation  
Can also be set via host URL query part, like `https://localhost:6060?ca=<ca>`

</td>
        </tr>
        <tr>
                    <th></th>
                    <th>COZY_HOST_CERT / COZY_ADMIN_CERT</th>
                    <td>none</td>
<td>

Client certificate to use  
Can also be set via host URL query part, like
`https://localhost:6060?cert=<cert>`

</td>
        </tr>
        <tr>
                    <th></th>
                    <th>COZY_HOST_KEY / COZY_ADMIN_KEY</th>
                    <td>none</td>
<td>

Client certificate to use  
Can also be set via host URL query part, like `https://localhost:6060?key=<key>`

</td>
        </tr>
        <tr>
                    <th></th>
                    <th>COZY_HOST_FINGERPRINT / COZY_ADMIN_FINGERPRINT</th>
                    <td>none</td>
<td>

Hex-encoded SHA-256 key pinning to use  
Can also be set via host URL query part, like `https://localhost:6060?fp=<fp>`

You can get the fingerprint of a given certificate with  
`openssl x509 -in <certificat.crt> -pubkey | openssl pkey -pubin -outform der | openssl dgst -sha256 -hex`  
Or directly from a private key with
`openssl pkey -in <key.pem> -pubout -outform der | openssl dgst -sha256 -hex`

</td>
        </tr>
    </tbody>
</table>

## Administration secret

To access to the administration API (the `/admin/*` routes), a secret passphrase
should be stored in a `cozy-admin-passphrase`. This file should be in one of the
configuration directories, along with the main config file.

The passphrase is stored in a salted-hashed representation using scrypt. To
generate this file, you can use the
`cozy-stack config passwd [config directory]` command. This command will ask you
for a passphrase and will create the `cozy-admin-passphrase` in the specified
directory.

You can use the `COZY_ADMIN_PASSWORD` env variable if you do not want to type
the passphrase each time you call `cozy-stack`.

### Example

```sh
cozy-stack config passwd ~/.cozy
# Hashed passphrase outputed in ~/.cozy/cozy-admin-passphrase
cat ~/.cozy/cozy-admin-passphrase
# scrypt$16384$8$1$936bd62faf633b5f946f653c21161a9b$4e0d11dfa5fc1676ed329938b11a6584d30e603e0d06b8a63a99e8cec392d682
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

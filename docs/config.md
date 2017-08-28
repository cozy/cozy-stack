[Table of contents](README.md#table-of-contents)

# Configuration

## Main Configuration file

You can configure your `cozy-stack` using a configuration file. This file
should be named `cozy.yaml` or `cozy.json` depending on the format of your
chosing, and should be present in one of these directories (ordered by
priority):

-  `./.cozy`
-  `$HOME/.cozy`
-  `/etc/cozy`

The configuration can also be passed or overriden using the flags of the
command line interface. See `cozy-stack --help`. And it's also possible to use
env variables, in upper case, prefixed by `COZY_`.

```sh
COZY_PORT=9090 cozy-stack serve
```

See the example to check what contains the configuration.

### Example

You can see an example of configuration in the
[cozy.example.yaml](../cozy.example.yaml) file at the root of this repository.

This example's values represent the default values of the configuration. The
equivalent cli flag are also filled in.


## Administration secret

To access to the administration API (the `/admin/*` routes), a secret passphrase should be stored in a `cozy-admin-passphrase`. This file should be in one of the configuration directories, along with the main config file.

The passphrase is stored in a salted-hashed representation using scrypt. To generate this file, you can use the `cozy-stack config passwd [config directory]` command. This command will ask you for a passphrase and will create the `cozy-admin-passphrase` in the specified directory.

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
executable. Then, they should be fired automatically. Let's the available
hooks.

The `pre-add-instance` hook is run just before creating an instance. It can
prevent the command from running by exiting with non-zero status. It can be
used to check the domain for example. It is called with the following
parameter:

1. the domain of the instance that will be created.

The `post-add-instance` hook is run just after an instance has been created.
installed. It can be used to setup DNS for this instance for example. It is
called with the following parameter:

1. the domain of the instance that has been created.

The `pre-remove-instance` hook is run just before destroying an instance. It
can prevent the command from running by exiting with non-zero status. It can
be used to make a backup of the instance before destroying it. It is called
with the following parameter:

1. the domain of the instance that will be destroyed.

The `post-remove-instance` hook is run just after an instance has been
destroyed. It can be used to do some cleanup. It is called with the following
parameter:

1. the domain of the instance that has been destroyed.

The `pre-install-app` hook is run just before installing an application, and
can prevent the command from running by exiting with non-zero status. It is
called with the following parameters:

1. the instance on which the application will be installed
2. the application name that will be installed.

The `post-install-app` hook is run just after an application has been
installed. It can be used for logging, notification, statistics, etc. It's
also a good place to add a vhost for an application in the reverse-proxy
configuration, with a TLS certificate. It is called with the following
parameters:

1. the instance on which the application has been installed
2. the application name that has been installed.

The `pre-uninstall-app` hook is run just before uninstalling an application,
and can prevent the command from running by exiting with non-zero status. It
is called with the following parameters:

1. the instance on which the application will be uninstalled
2. the application name that will be uninstalled.

The `post-uninstall-app` hook is run just after an application has been
uninstalled. It can be used for cleaning the configuration of the
reverse-proxy for example. It is called with the following parameters:

1. the instance on which the application has been uninstalled
2. the application name that has been uninstalled.

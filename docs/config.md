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
command line interface. See `cozy-stack --help`.

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

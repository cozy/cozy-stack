[Table of contents](./README.md#table-of-contents)

Configuration
=============

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

Example
-------

You can see an example of configuration in the
[cozy.dist.yaml](./cozy.dist.yaml) file at the root of this repository.

This example's values represent the default values of the configuration. The
equivalent cli flag are also filled in.

Configuration
=============

You can configure your `cozy-stack` using a configuration file. This file should be named `cozy.yaml` or `cozy.json` depending on the format of your chosing, and should be present in one of these directories (ordered by priority):

-  `.cozy`
-  `$HOME/.cozy`
-  `/etc/cozy`

The configuration can also be passed or overriden using the flags of the command line interface. See `cozy-stack --help`.

See the example to check what contains the configuration.

Example
-------

This example values represent the default values of the configuration. The equivalent cli flag is in parenthesis.

```yaml
mode: development   # server mode: development or production (--mode -m)
host: localhost     # server host (--host)
port: 8080          # server port (--port -p)
couchdb:
    host: localhost # couchdb host (--couchdb-host)
    port: 5984      # couchdb port (--couchdb-port)
log:
    level: info     # logger level (debug, info, warning, panic, fatal) (--log-level)
```

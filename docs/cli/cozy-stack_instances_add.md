## cozy-stack instances add

Manage instances of a stack

### Synopsis


cozy-stack instances add allows to create an instance on the cozy for a
given domain.

If the COZY_DISABLE_INSTANCES_ADD_RM env variable is set, creating and
destroying instances will be desactivated and the content of this variable will
be used as the error message.


```
cozy-stack instances add <domain> [flags]
```

### Examples

```
$ cozy-stack instances add --passphrase cozy --apps drive,photos,settings,home,store cozy.localhost:8080
```

### Options

```
      --apps strings             Apps to be preinstalled
      --context-name string      Context name of the instance
      --couch-cluster int        Specify the CouchDB cluster where the instance will be created (-1 means the default) (default -1)
      --dev                      To create a development instance (deprecated)
      --disk-quota string        The quota allowed to the instance's VFS
      --domain-aliases strings   Specify one or more aliases domain for the instance (separated by ',')
      --email string             The email of the owner
  -h, --help                     help for add
      --locale string            Locale of the new cozy instance (default "en")
      --oidc_id string           The identifier for checking authentication from OIDC
      --passphrase string        Register the instance with this passphrase (useful for tests)
      --public-name string       The public name of the owner
      --settings string          A list of settings (eg context:foo,offer:premium)
      --swift-layout int         Specify the layout to use for Swift (from 0 for layout V1 to 2 for layout V3, -1 means the default) (default -1)
      --tos string               The TOS version signed
      --trace                    Show where time is spent
      --tz string                The timezone for the user
      --uuid string              The UUID of the instance
```

### Options inherited from parent commands

```
      --admin-host string   administration server host (default "localhost")
      --admin-port int      administration server port (default 6060)
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --host string         server host (default "localhost")
  -p, --port int            server port (default 8080)
```

### SEE ALSO

* [cozy-stack instances](cozy-stack_instances.md)	 - Manage instances of a stack


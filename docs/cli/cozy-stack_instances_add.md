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
$ cozy-stack instances add --dev --passphrase cozy --apps drive,photos,settings cozy.tools:8080
```

### Options

```
      --apps strings             Apps to be preinstalled
      --context-name string      Context name of the instance
      --dev                      To create a development instance
      --disk-quota string        The quota allowed to the instance's VFS
      --domain-aliases strings   Specify one or more aliases domain for the instance (separated by ',')
      --email string             The email of the owner
  -h, --help                     help for add
      --locale string            Locale of the new cozy instance (default "en")
      --passphrase string        Register the instance with this passphrase (useful for tests)
      --public-name string       The public name of the owner
      --settings string          A list of settings (eg context:foo,offer:premium)
      --swift-cluster int        Specify a cluster number for swift
      --tos string               The TOS version signed
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


## cozy-stack instances modify

Modify the instance properties

### Synopsis


cozy-stack instances modify allows to change the instance properties and
settings for a specified domain.


```
cozy-stack instances modify <domain> [flags]
```

### Options

```
      --blocked                  Block the instance
      --context-name string      New context name
      --disk-quota string        Specify a new disk quota
      --domain-aliases strings   Specify one or more aliases domain for the instance (separated by ',')
      --email string             New email
  -h, --help                     help for modify
      --locale string            New locale
      --onboarding-finished      Force the finishing of the onboarding
      --public-name string       New public name
      --settings string          New list of settings (eg offer:premium)
      --swift-cluster int        New swift cluster
      --tos string               Update the TOS version signed
      --tos-latest string        Update the latest TOS version
      --tz string                New timezone
      --uuid string              New UUID
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


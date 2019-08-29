## cozy-stack instances debug

Activate or deactivate debugging of the instance

### Synopsis


cozy-stack instances debug allows to activate or deactivate the debugging of a
specific domain.


```
cozy-stack instances debug <true/false> [flags]
```

### Examples

```
$ cozy-stack instances debug --domain cozy.tools:8080 true
```

### Options

```
      --domain string   Specify the domain name of the instance (default "cozy.tools:8080")
  -h, --help            help for debug
      --ttl duration    Specify how long the debug mode will last (default 24h0m0s)
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


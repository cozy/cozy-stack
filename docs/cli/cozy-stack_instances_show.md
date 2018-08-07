## cozy-stack instances show

Show the instance of the specified domain

### Synopsis


cozy-stack instances show allows to show the instance on the cozy for a
given domain.


```
cozy-stack instances show <domain> [flags]
```

### Examples

```
$ cozy-stack instances show cozy.tools:8080
```

### Options

```
  -h, --help   help for show
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


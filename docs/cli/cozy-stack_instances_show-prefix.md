## cozy-stack instances show-prefix

Show the instance prefix of the specified domain

### Synopsis


cozy-stack instances show allows to show the instance prefix on the cozy for a
given domain. The prefix is used for databases and VFS prefixing.


```
cozy-stack instances show-prefix <domain> [flags]
```

### Examples

```
$ cozy-stack instances show-prefix cozy.tools:8080
```

### Options

```
  -h, --help   help for show-prefix
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


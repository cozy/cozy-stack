## cozy-stack instances show-db-prefix

Show the instance DB prefix of the specified domain

### Synopsis


cozy-stack instances show allows to show the instance prefix on the cozy for a
given domain. The prefix is used for databases and VFS prefixing.

It will also show the couch_cluster if it is not the default one.


```
cozy-stack instances show-db-prefix <domain> [flags]
```

### Examples

```
$ cozy-stack instances show-db-prefix cozy.localhost:8080
```

### Options

```
  -h, --help   help for show-db-prefix
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


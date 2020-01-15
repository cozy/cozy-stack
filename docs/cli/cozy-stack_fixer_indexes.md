## cozy-stack fixer indexes

Rebuild the CouchDB views and indexes

### Synopsis


This fixer ensures that the CouchDB views and indexes used by the stack for
this instance are correctly set.


```
cozy-stack fixer indexes <domain> [flags]
```

### Options

```
  -h, --help   help for indexes
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

* [cozy-stack fixer](cozy-stack_fixer.md)	 - A set of tools to fix issues or migrate content for retro-compatibility.


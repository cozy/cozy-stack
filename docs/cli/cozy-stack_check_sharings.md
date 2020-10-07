## cozy-stack check sharings

Check the io.cozy.sharings documents

### Synopsis


This command checks that the io.cozy.sharings have no inconsistencies. It can
be triggers that are missing on an active sharing, or missing credentials for
an active member.


```
cozy-stack check sharings <domain> [flags]
```

### Options

```
  -h, --help   help for sharings
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

* [cozy-stack check](cozy-stack_check.md)	 - A set of tools to check that instances are in the expected state.


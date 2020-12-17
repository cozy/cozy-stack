## cozy-stack check triggers

Check the triggers

### Synopsis


This command checks that the instance doesn't have duplicate triggers: several
triggers of the same type, for the same worker, and with the same arguments.


```
cozy-stack check triggers <domain> [flags]
```

### Options

```
  -h, --help   help for triggers
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


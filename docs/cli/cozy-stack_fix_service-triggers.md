## cozy-stack fix service-triggers

Clean the triggers for webapp services

### Synopsis


This fixer cleans duplicate triggers for webapp services, and recreates missing
triggers.


```
cozy-stack fix service-triggers <domain> [flags]
```

### Options

```
  -h, --help   help for service-triggers
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

* [cozy-stack fix](cozy-stack_fix.md)	 - A set of tools to fix issues or migrate content.


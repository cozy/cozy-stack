## cozy-stack features config

Display the feature flags from configuration for a context

### Synopsis


cozy-stack feature config displays the feature flags from configuration for a context.

These flags are read only and can only be updated by changing configuration and restarting the stack.


```
cozy-stack features config <context-name> [flags]
```

### Examples

```
$ cozy-stack feature config --context beta
```

### Options

```
      --context string   The context for the feature flags
  -h, --help             help for config
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

* [cozy-stack features](cozy-stack_features.md)	 - Manage the feature flags


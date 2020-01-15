## cozy-stack features ratio

Display and update the feature flags for a context

### Synopsis


cozy-stack feature ratio displays the feature flags for a context.

It can also create, update, or remove flags (with a ratio and value).

To remove a flag, set it to an empty array (or null).


```
cozy-stack features ratio <context-name> [flags]
```

### Examples

```
$ cozy-stack feature ratio --context beta '{"set_this_flag": [{"ratio": 0.1, "value": 1}, {"ratio": 0.9, "value": 2}] }'
```

### Options

```
      --context string   The context for the feature flags
  -h, --help             help for ratio
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


## cozy-stack features flags

Display and update the feature flags for an instance

### Synopsis


cozy-stack feature flags displays the feature flags that are specific to an instance.

It can also take a list of flags to update.

If you give a null value, the flag will be removed.


```
cozy-stack features flags [flags]
```

### Examples

```
$ cozy-stack feature flags --domain cozy.tools:8080 '{"add_this_flag": true, "remove_this_flag": null}'
```

### Options

```
      --domain string   Specify the domain name of the instance
  -h, --help            help for flags
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


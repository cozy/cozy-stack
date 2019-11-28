## cozy-stack features defaults

Display and update the default values for feature flags

### Synopsis


cozy-stack feature defaults displays the default values for feature flags.

It can also take a list of flags to update.

If you give a null value, the flag will be removed.


```
cozy-stack features defaults [flags]
```

### Examples

```
$ cozy-stack feature defaults '{"add_this_flag": true, "remove_this_flag": null}'
```

### Options

```
  -h, --help   help for defaults
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


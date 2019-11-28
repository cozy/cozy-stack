## cozy-stack features show

Display the computed feature flags for an instance

### Synopsis


cozy-stack feature show displays the feature flags that are shown by apps.


```
cozy-stack features show [flags]
```

### Examples

```
$ cozy-stack feature show --domain cozy.tools:8080
```

### Options

```
      --domain string   Specify the domain name of the instance
  -h, --help            help for show
      --source          Show the sources of the feature flags
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


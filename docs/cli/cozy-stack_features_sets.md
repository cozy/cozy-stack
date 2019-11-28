## cozy-stack features sets

Display and update the feature sets for an instance

### Synopsis


cozy-stack feature sets displays the feature sets coming from the manager.

It can also take a space-separated list of sets that will replace the previous
list (no merge).

All the sets can be removed by setting an empty list ('').


```
cozy-stack features sets [flags]
```

### Examples

```
$ cozy-stack feature sets --domain cozy.tools:8080 'set1 set2'
```

### Options

```
      --domain string   Specify the domain name of the instance
  -h, --help            help for sets
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


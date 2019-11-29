## cozy-stack config insert-asset

Inserts an asset

### Synopsis

Inserts a custom asset in a specific context

Deprecated: please use the command cozy-stack assets add.


```
cozy-stack config insert-asset --url <url> --name <name> --shasum <shasum> --context <context> [flags]
```

### Options

```
      --context string   The context of the asset
  -h, --help             help for insert-asset
      --name string      The name of the asset
      --shasum string    The shasum of the asset
      --url string       The URL of the asset
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

* [cozy-stack config](cozy-stack_config.md)	 - Show and manage configuration elements


## cozy-stack instances insert-asset

Inserts an asset

### Synopsis

Inserts a custom asset in a specific context

```
cozy-stack instances insert-asset --url <url> --name <name> --shasum <shasum> --context <context> [flags]
```

### Examples

```
$ cozy-stack config insert-asset --url file:///foo/bar/baz.js --name /foo/bar/baz.js --shasum 0763d6c2cebee0880eb3a9cc25d38cd23db39b5c3802f2dc379e408c877a2788 --context foocontext
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

* [cozy-stack instances](cozy-stack_instances.md)	 - Manage instances of a stack


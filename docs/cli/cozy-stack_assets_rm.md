## cozy-stack assets rm

Removes an asset

### Synopsis

Removes a custom asset in a specific context

```
cozy-stack assets rm [context] [name] [flags]
```

### Examples

```
$ cozy-stack config rm-asset foobar /foo/bar/baz.js
```

### Options

```
  -h, --help   help for rm
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

* [cozy-stack assets](cozy-stack_assets.md)	 - Show and manage dynamic assets


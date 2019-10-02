## cozy-stack assets add

Insert a dynamic asset

### Synopsis

Insert an asset that will be available on https://<instance>/assets/<name>

For example, if a dynamic asset with the name '/foo/bar/baz' is added for a
context foocontext, and an instance example.mycozy.cloud is in the foocontext
context, then this asset can be requested on
https://example.mycozy.cloud/assets/foo/bar/baz.js (and not on
'example-app.mycozy.cloud').

```
cozy-stack assets add --url <url> --name <name> --shasum <shasum> --context <context> [flags]
```

### Examples

```
$ cozy-stack assets add --url file:///foo/bar/baz.js --name /foo/bar/baz.js --shasum 0763d6c2cebee0880eb3a9cc25d38cd23db39b5c3802f2dc379e408c877a2788 --context foocontext
```

### Options

```
      --context string   The context of the asset
  -h, --help             help for add
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

* [cozy-stack assets](cozy-stack_assets.md)	 - Show and manage dynamic assets


## cozy-stack swift put



### Synopsis

cozy-stack swift put can be used to create or update an object in
the swift container associated to the given domain. The content of the file is
expected on the standard input.

```
cozy-stack swift put <domain> <object-name> [flags]
```

### Options

```
      --content-type string   Specify a Content-Type for the created object
  -h, --help                  help for put
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

* [cozy-stack swift](cozy-stack_swift.md)	 - Interact directly with OpenStack Swift object storage


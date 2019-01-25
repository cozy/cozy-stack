## cozy-stack instances set-passphrase

Change the passphrase of the instance

### Synopsis

Change the passphrase of the instance

```
cozy-stack instances set-passphrase <domain> <new-passphrase> [flags]
```

### Examples

```
$ cozy-stack instances set-passphrase cozy.tools:8080 myN3wP4ssowrd!
```

### Options

```
  -h, --help   help for set-passphrase
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


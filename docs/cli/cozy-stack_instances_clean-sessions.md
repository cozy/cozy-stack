## cozy-stack instances clean-sessions

Remove the io.cozy.sessions and io.cozy.sessions.logins bases

```
cozy-stack instances clean-sessions <domain> [flags]
```

### Examples

```
$ cozy-stack instance clean-sessions cozy.localhost:8080
```

### Options

```
  -h, --help   help for clean-sessions
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


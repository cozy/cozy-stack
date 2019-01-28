## cozy-stack instances auth-mode

Set instance auth-mode

### Synopsis

Change the authentication mode for an instance. Two options are allowed:
- two_factor_mail
- basic


```
cozy-stack instances auth-mode [domain] [auth-mode] [flags]
```

### Examples

```
$ cozy-stack instances auth-mode cozy.tools:8080 two_factor_mail
```

### Options

```
  -h, --help   help for auth-mode
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


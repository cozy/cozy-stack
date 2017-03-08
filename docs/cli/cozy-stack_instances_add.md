## cozy-stack instances add

Manage instances of a stack

### Synopsis



cozy-stack instances add allows to create an instance on the cozy for a
given domain.


```
cozy-stack instances add [domain]
```

### Options

```
      --apps stringSlice    Apps to be preinstalled
      --dev                 To create a development instance
      --email string        The email of the owner
      --locale string       Locale of the new cozy instance (default "en")
      --passphrase string   Register the instance with this passphrase (useful for tests)
      --tz string           The timezone for the user
```

### Options inherited from parent commands

```
      --admin-host string   administration server host (default "localhost")
      --admin-port int      administration server port (default 6060)
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --host string         server host (default "localhost")
      --log-level string    define the log level (default "info")
  -p, --port int            server port (default 8080)
```

### SEE ALSO
* [cozy-stack instances](cozy-stack_instances.md)	 - Manage instances of a stack


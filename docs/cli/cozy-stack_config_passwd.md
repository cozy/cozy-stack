## cozy-stack config passwd

Generate an admin passphrase

### Synopsis



cozy-stack instances passphrase generate a passphrase hash and save it to a file in
the specified directory. This passphrase is the one used to authenticate accesses
to the administration API.

example: cozy-stack config passwd ~/.cozy


```
cozy-stack config passwd [directory]
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
* [cozy-stack config](cozy-stack_config.md)	 - Show and manage configuration elements


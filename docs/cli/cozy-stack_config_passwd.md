## cozy-stack config passwd

Generate an admin passphrase

### Synopsis


cozy-stack instances passphrase generate a passphrase hash and save it to the
specified file. If no file is specified, it is directly printed in standard output.
This passphrase is the one used to authenticate accesses to the administration API.

The environment variable 'COZY_ADMIN_PASSPHRASE' can be used to pass the passphrase
if needed.

example: cozy-stack config passwd ~/.cozy/


```
cozy-stack config passwd <filepath> [flags]
```

### Options

```
  -h, --help   help for passwd
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


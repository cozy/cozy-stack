## cozy-stack fix password-defined

Set the password_defined setting

### Synopsis


A password_defined field has been added to the io.cozy.settings.instance
(available on GET /settings/instance). This fixer can fill it for existing Cozy
instances if it was missing.


```
cozy-stack fix password-defined <domain> [flags]
```

### Options

```
  -h, --help   help for password-defined
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

* [cozy-stack fix](cozy-stack_fix.md)	 - A set of tools to fix issues or migrate content.


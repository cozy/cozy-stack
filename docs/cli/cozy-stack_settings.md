## cozy-stack settings

Display and update settings

### Synopsis


cozy-stack settings displays the settings.

It can also take a list of settings to update.

If you give a blank value, the setting will be removed.


```
cozy-stack settings [settings] [flags]
```

### Examples

```
$ cozy-stack settings --domain cozy.tools:8080 context:beta,public_name:John,to_remove:
```

### Options

```
      --domain string   specify the domain name of the instance (default "cozy.tools:8080")
  -h, --help            help for settings
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

* [cozy-stack](cozy-stack.md)	 - cozy-stack is the main command


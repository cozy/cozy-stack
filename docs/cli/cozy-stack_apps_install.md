## cozy-stack apps install

Install an application with the specified slug name
from the given source URL.

### Synopsis

[Some schemes](../../docs/apps.md#sources) are allowed as `[sourceurl]`.

```
cozy-stack apps install <slug> [sourceurl] [flags]
```

### Examples

```
$ cozy-stack apps install --domain cozy.tools:8080 drive registry://drive/stable
```

### Options

```
      --ask-permissions   specify that the application should not be activated after installation
  -h, --help              help for install
```

### Options inherited from parent commands

```
      --admin-host string   administration server host (default "localhost")
      --admin-port int      administration server port (default 6060)
      --all-domains         work on all domains iterativelly
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --domain string       specify the domain name of the instance (default "cozy.tools:8080")
      --host string         server host (default "localhost")
  -p, --port int            server port (default 8080)
```

### SEE ALSO

* [cozy-stack apps](cozy-stack_apps.md)	 - Interact with the applications


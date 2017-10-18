## cozy-stack apps install

Install an application with the specified slug name
from the given source URL.

### Synopsis


Install an application with the specified slug name
from the given source URL.

```
cozy-stack apps install [slug] [sourceurl] [flags]
```

[Some schemes](../../docs/apps.md#sources) are allowed as `[sourceurl]`.

### Examples

```
$ cozy-stack apps install --domain cozy.tools:8080 drive 'git://github.com/cozy/cozy-drive.git#build-drive'
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
      --client-use-https    if set the client will use https to communicate with the server
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --domain string       specify the domain name of the instance
      --host string         server host (default "localhost")
  -p, --port int            server port (default 8080)
```

### SEE ALSO
* [cozy-stack apps](cozy-stack_apps.md)	 - Interact with the cozy applications


## cozy-stack konnectors install

Install an konnector with the specified slug name
from the given source URL.

### Synopsis


Install an konnector with the specified slug name
from the given source URL.

```
cozy-stack konnectors install [slug] [sourceurl]
```

### Examples

```
$ cozy-stack konnectors install --domain cozy.tools:8080 trainline 'git://github.com/cozy/cozy-konnector-trainline.git#build'
```

### Options inherited from parent commands

```
      --admin-host string   administration server host (default "localhost")
      --admin-port int      administration server port (default 6060)
      --all-domains         work on all domains iterativelly
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --domain string       specify the domain name of the instance
      --host string         server host (default "localhost")
      --log-level string    define the log level (default "info")
  -p, --port int            server port (default 8080)
```

### SEE ALSO
* [cozy-stack konnectors](cozy-stack_konnectors.md)	 - Interact with the cozy applications


## cozy-stack konnectors install

Install a konnector with the specified slug name
from the given source URL.

### Synopsis


Install a konnector with the specified slug name. You can also provide the
version number to install a specific release if you use the registry:// scheme.
Following formats are accepted:
	registry://<konnector>/<channel>/<version>
	registry://<konnector>/<channel>
	registry://<konnector>/<version>
	registry://<konnector>

If you provide a channel and a version, the channel is ignored.
Default channel is stable.
Default version is latest.


```
cozy-stack konnectors install <slug> [sourceurl] [flags]
```

### Examples

```

$ cozy-stack konnectors install --domain cozy.tools:8080 trainline registry://trainline/stable/1.0.1
$ cozy-stack konnectors install --domain cozy.tools:8080 trainline registry://trainline/stable
$ cozy-stack konnectors install --domain cozy.tools:8080 trainline registry://trainline/1.2.0
$ cozy-stack konnectors install --domain cozy.tools:8080 trainline registry://trainline

```

### Options

```
  -h, --help   help for install
```

### Options inherited from parent commands

```
      --admin-host string   administration server host (default "localhost")
      --admin-port int      administration server port (default 6060)
      --all-domains         work on all domains iterativelly
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --domain string       specify the domain name of the instance (default "cozy.tools:8080")
      --host string         server host (default "localhost")
      --parameters string   override the parameters of the installed konnector
  -p, --port int            server port (default 8080)
```

### SEE ALSO

* [cozy-stack konnectors](cozy-stack_konnectors.md)	 - Interact with the konnectors


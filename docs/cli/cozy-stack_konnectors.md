## cozy-stack konnectors

Interact with the konnectors

### Synopsis


cozy-stack konnectors allows to interact with the cozy konnectors.

It provides commands to install or update applications on
a cozy.


```
cozy-stack konnectors <command> [flags]
```

### Options

```
      --all-domains         work on all domains iterativelly
      --domain string       specify the domain name of the instance (default "cozy.tools:8080")
  -h, --help                help for konnectors
      --parameters string   override the parameters of the installed konnector
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
* [cozy-stack konnectors install](cozy-stack_konnectors_install.md)	 - Install a konnector with the specified slug name
from the given source URL.
* [cozy-stack konnectors ls](cozy-stack_konnectors_ls.md)	 - List the installed konnectors.
* [cozy-stack konnectors run](cozy-stack_konnectors_run.md)	 - Run a konnector.
* [cozy-stack konnectors show](cozy-stack_konnectors_show.md)	 - Show the application attributes
* [cozy-stack konnectors uninstall](cozy-stack_konnectors_uninstall.md)	 - Uninstall the konnector with the specified slug name.
* [cozy-stack konnectors update](cozy-stack_konnectors_update.md)	 - Update the konnector with the specified slug name.


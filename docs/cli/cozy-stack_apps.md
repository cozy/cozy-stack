## cozy-stack apps

Interact with the applications

### Synopsis


cozy-stack apps allows to interact with the cozy applications.

It provides commands to install or update applications on
a cozy.


```
cozy-stack apps <command> [flags]
```

### Options

```
      --all-domains     work on all domains iterativelly
      --domain string   specify the domain name of the instance (default "cozy.tools:8080")
  -h, --help            help for apps
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
* [cozy-stack apps install](cozy-stack_apps_install.md)	 - Install an application with the specified slug name
from the given source URL.
* [cozy-stack apps ls](cozy-stack_apps_ls.md)	 - List the installed applications.
* [cozy-stack apps show](cozy-stack_apps_show.md)	 - Show the application attributes
* [cozy-stack apps uninstall](cozy-stack_apps_uninstall.md)	 - Uninstall the application with the specified slug name.
* [cozy-stack apps update](cozy-stack_apps_update.md)	 - Update the application with the specified slug name.
* [cozy-stack apps versions](cozy-stack_apps_versions.md)	 - Show apps versions of all instances


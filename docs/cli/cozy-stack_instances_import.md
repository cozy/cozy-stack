## cozy-stack instances import

Import data from an export link

### Synopsis

This command will reset the Cozy instance and import data from an export link

```
cozy-stack instances import <URL> [flags]
```

### Options

```
      --domain string   Specify the domain name of the instance
      --force           Force the import without asking for confirmation
  -h, --help            help for import
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

* [cozy-stack instances](cozy-stack_instances.md)	 - Manage instances of a stack


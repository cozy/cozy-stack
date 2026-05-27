## cozy-stack fix sharings-moved

Fix sharings after a domain migration

### Synopsis


This fixer repairs sharings after a domain migration. It performs three actions:

1. Updates the local sharing documents to use the new instance URL for "self"
2. Updates all trigger documents to use the new domain (fixes share-track jobs)
3. Notifies other sharing members about the domain change

Run this fixer after migrating an instance to a new domain to restore file
sharing synchronization.


```
cozy-stack fix sharings-moved <domain> [flags]
```

### Options

```
  -h, --help   help for sharings-moved
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


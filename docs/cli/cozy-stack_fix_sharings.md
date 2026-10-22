## cozy-stack fix sharings

Fix the issues found by the check of the io.cozy.sharings documents

### Synopsis


This fixer runs the "cozy-stack check sharings --fast" checks and fixes the
issues that can be fixed automatically. The other issues are left as is.

The fixed issues are:

- drive_revoked_by_owner: the shared drive is revoked on the member instance,
  as if the missed revocation notification had been received.

The fixed issues are printed, one per line.


```
cozy-stack fix sharings <domain> [flags]
```

### Options

```
  -h, --help   help for sharings
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


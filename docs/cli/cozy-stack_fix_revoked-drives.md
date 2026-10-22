## cozy-stack fix revoked-drives

Revoke the shared drives already revoked by their owner

### Synopsis


This fixer revokes, on a member instance, the shared drives that are still
active while their owner has already revoked this member, as if the missed
revocation notification had been received. They are reported as
drive_revoked_by_owner by the "cozy-stack check sharings" command. Only the
owners on the same stack can be checked.

The IDs of the revoked sharings are printed, one per line.


```
cozy-stack fix revoked-drives <domain> [flags]
```

### Options

```
  -h, --help   help for revoked-drives
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


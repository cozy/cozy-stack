## cozy-stack fix orphan-account

Remove the orphan accounts

### Synopsis


This fixer detects the accounts that are linked to a konnector that has been
uninstalled, and then removed them.

For banking accounts, the konnector must run to also clean the account
remotely. To do so, the konnector is installed, the account is deleted,
the stack runs the konnector with the AccountDeleted flag, and when it's
done, the konnector is uninstalled again.


```
cozy-stack fix orphan-account <domain> [flags]
```

### Options

```
  -h, --help   help for orphan-account
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


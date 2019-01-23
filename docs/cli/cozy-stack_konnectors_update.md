## cozy-stack konnectors update

Update the konnector with the specified slug name.

### Synopsis

Update the konnector with the specified slug name.

```
cozy-stack konnectors update <slug> [sourceurl] [flags]
```

### Options

```
  -h, --help   help for update
      --safe   do not upgrade if there are blocking changes
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


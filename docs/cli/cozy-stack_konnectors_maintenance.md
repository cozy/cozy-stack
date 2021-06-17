## cozy-stack konnectors maintenance

Activate the maintenance for the given konnector

```
cozy-stack konnectors maintenance [slug] [flags]
```

### Options

```
  -h, --help             help for maintenance
      --no-manual-exec   specify a maintenance disallowing manual execution
      --short            specify a short maintenance
```

### Options inherited from parent commands

```
      --admin-host string   administration server host (default "localhost")
      --admin-port int      administration server port (default 6060)
      --all-domains         work on all domains iteratively
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --domain string       specify the domain name of the instance (default "cozy.localhost:8080")
      --host string         server host (default "localhost")
      --parameters string   override the parameters of the installed konnector
  -p, --port int            server port (default 8080)
```

### SEE ALSO

* [cozy-stack konnectors](cozy-stack_konnectors.md)	 - Interact with the konnectors


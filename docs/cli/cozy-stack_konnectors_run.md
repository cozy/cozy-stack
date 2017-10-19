## cozy-stack konnectors run

Run a konnector.

### Synopsis


Run a konnector named with specified slug using the specified options.

```
cozy-stack konnectors run [slug] [flags]
```

### Options

```
      --account-id string   specify the account ID to use for running the konnector
      --folder string       specify the folder path associated with the konnector
  -h, --help                help for run
```

### Options inherited from parent commands

```
      --admin-host string   administration server host (default "localhost")
      --admin-port int      administration server port (default 6060)
      --all-domains         work on all domains iterativelly
      --client-use-https    if set the client will use https to communicate with the server
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --domain string       specify the domain name of the instance (default "cozy.tools:8080")
      --host string         server host (default "localhost")
      --parameters string   override the parameters of the installed konnector
  -p, --port int            server port (default 8080)
```

### SEE ALSO
* [cozy-stack konnectors](cozy-stack_konnectors.md)	 - Interact with the cozy applications


## cozy-stack instances update

Start the updates for the specified domain instance.

### Synopsis


Start the updates for the specified domain instance. Use whether the --domain
flag to specify the instance or the --all-domains flags to updates all domains.
The slugs arguments can be used to select which applications should be
updated.

```
cozy-stack instances update [domain] [slugs...] [flags]
```

### Options

```
      --all-domains     Work on all domains iterativelly
      --domain string   Specify the domain name of the instance
  -h, --help            help for update
```

### Options inherited from parent commands

```
      --admin-host string   administration server host (default "localhost")
      --admin-port int      administration server port (default 6060)
      --client-use-https    if set the client will use https to communicate with the server
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --host string         server host (default "localhost")
  -p, --port int            server port (default 8080)
```

### SEE ALSO
* [cozy-stack instances](cozy-stack_instances.md)	 - Manage instances of a stack


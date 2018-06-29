## cozy-stack instances update

Start the updates for the specified domain instance.

### Synopsis

Start the updates for the specified domain instance. Use whether the --domain
flag to specify the instance or the --all-domains flags to updates all domains.
The slugs arguments can be used to select which applications should be
updated.

```
cozy-stack instances update [slugs...] [flags]
```

### Options

```
      --all-domains           Work on all domains iterativelly
      --context-name string   Work only on the instances with the given context name
      --domain string         Specify the domain name of the instance
      --force-registry        Force to update all applications sources from git to the registry
  -h, --help                  help for update
      --only-registry         Only update applications installed from the registry
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


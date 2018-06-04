## cozy-stack triggers launch

Creates a job from a specific trigger

### Synopsis

Creates a job from a specific trigger

```
cozy-stack triggers launch [triggerId] [flags]
```

### Examples

```
$ cozy-stack triggers launch --domain cozy.tools:8080 748f42b65aca8c99ec2492eb660d1891
```

### Options

```
  -h, --help   help for launch
```

### Options inherited from parent commands

```
      --admin-host string   administration server host (default "localhost")
      --admin-port int      administration server port (default 6060)
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --domain string       specify the domain name of the instance (default "cozy.tools:8080")
      --host string         server host (default "localhost")
  -p, --port int            server port (default 8080)
```

### SEE ALSO

* [cozy-stack triggers](cozy-stack_triggers.md)	 - Interact with the triggers


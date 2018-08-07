## cozy-stack triggers

Interact with the triggers

### Synopsis


cozy-stack apps allows to interact with the cozy triggers.

It provides command to run a specific trigger.


```
cozy-stack triggers <command> [flags]
```

### Options

```
      --domain string   specify the domain name of the instance (default "cozy.tools:8080")
  -h, --help            help for triggers
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

* [cozy-stack](cozy-stack.md)	 - cozy-stack is the main command
* [cozy-stack triggers launch](cozy-stack_triggers_launch.md)	 - Creates a job from a specific trigger
* [cozy-stack triggers ls](cozy-stack_triggers_ls.md)	 - List triggers
* [cozy-stack triggers show-from-app](cozy-stack_triggers_show-from-app.md)	 - Show the application triggers


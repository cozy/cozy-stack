## cozy-stack instances set-disk-quota

Change the disk-quota of the instance

### Synopsis



cozy-stack instances set-disk-quota allows to change the disk-quota of the
instance of the given domain. Set the quota to 0 to remove the quota.


```
cozy-stack instances set-disk-quota [domain] [disk-quota]
```

### Examples

```
$ cozy-stack instances set-disk-quota cozy.tools:8080 3GB
```

### Options inherited from parent commands

```
      --admin-host string   administration server host (default "localhost")
      --admin-port int      administration server port (default 6060)
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --host string         server host (default "localhost")
      --log-level string    define the log level (default "info")
  -p, --port int            server port (default 8080)
```

### SEE ALSO
* [cozy-stack instances](cozy-stack_instances.md)	 - Manage instances of a stack


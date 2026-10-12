## cozy-stack tools goroutine

Dump the stack traces of all current goroutines

### Synopsis


This command can be used to look for goroutine leaks. It dumps the stack traces
of all current goroutines on stdout.

See https://go.dev/doc/diagnostics#profiling.


```
cozy-stack tools goroutine [flags]
```

### Examples

```
$ cozy-stack tools goroutine > goroutine.pprof && go tool pprof goroutine.pprof
```

### Options

```
  -h, --help   help for goroutine
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

* [cozy-stack tools](cozy-stack_tools.md)	 - Regroup some tools for debugging and tests


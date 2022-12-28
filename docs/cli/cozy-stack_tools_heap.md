## cozy-stack tools heap

Dump a sampling of memory allocations of live objects

### Synopsis


This command can be used for memory profiling. It dumps a sampling of memory
allocations of live objects on stdout.

See https://go.dev/doc/diagnostics#profiling.


```
cozy-stack tools heap [flags]
```

### Examples

```
$ cozy-stack tools heap > heap.pprof && go tool pprof heap.pprof
```

### Options

```
  -h, --help   help for heap
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


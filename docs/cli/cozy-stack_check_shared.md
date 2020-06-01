## cozy-stack check shared

Check the io.cozy.shared documents

### Synopsis


The io.cozy.shared documents have a tree of revisions. This command will check
that all revisions in this tree are either the root or their parent have a
generation smaller than their generation.


```
cozy-stack check shared <domain> [flags]
```

### Options

```
  -h, --help   help for shared
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

* [cozy-stack check](cozy-stack_check.md)	 - A set of tools to check that instances are in the expected state.


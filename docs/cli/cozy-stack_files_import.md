## cozy-stack files import

Import the specified file or directory into cozy

### Synopsis

Import the specified file or directory into cozy

```
cozy-stack files import [--domain domain] [--match pattern] --from <name> --to <name> [flags]
```

### Options

```
      --dry-run        do not actually import the files
      --from string    directory to import from in cozy
  -h, --help           help for import
      --match string   pattern that the imported files must match
      --to string      directory to import to in cozy (default "/")
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

* [cozy-stack files](cozy-stack_files.md)	 - Interact with the cozy filesystem


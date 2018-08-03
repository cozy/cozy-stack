## cozy-stack doc man

Print the manpages of cozy-stack

### Synopsis

Print the manual pages for using cozy-stack in command-line

```
cozy-stack doc man <directory> [flags]
```

### Examples

```
$ mkdir -p ~/share/man
$ export MANPATH=~/share/man:$MANPATH
$ cozy-stack doc man ~/share/man
$ man cozy-stack
```

### Options

```
  -h, --help   help for man
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

* [cozy-stack doc](cozy-stack_doc.md)	 - Print the documentation


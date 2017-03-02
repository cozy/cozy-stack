## cozy-stack doc man

Print the manpages of cozy-stack

### Synopsis


Print the manual pages for using cozy-stack in command-line

```
cozy-stack doc man [directory]
```

### Examples

```
$ mkdir -p ~/share/man
$ export MANPATH=~/share/man:$MANPATH
$ cozy-stack doc man ~/share/man
$ man cozy-stack
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
* [cozy-stack doc](cozy-stack_doc.md)	 - Print the documentation


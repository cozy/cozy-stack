## cozy-stack files

Interact with the cozy filesystem

### Synopsis


cozy-stack files allows to interact with the cozy filesystem.

It provides command to create, move copy or delete files and
directories inside your cozy instance, using the command line
interface. It also provide an import command to import from your
current filesystem into cozy.


```
cozy-stack files <command> [flags]
```

### Options

```
      --domain string   specify the domain name of the instance (default "cozy.tools:8080")
  -h, --help            help for files
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
* [cozy-stack files exec](cozy-stack_files_exec.md)	 - Execute the given command on the specified domain and leave
* [cozy-stack files import](cozy-stack_files_import.md)	 - Import the specified file or directory into cozy


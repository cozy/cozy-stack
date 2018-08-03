## cozy-stack files exec

Execute the given command on the specified domain and leave

### Synopsis

Execute a command on the VFS of the specified domain.
Available commands:

    mkdir <name>               Creates a directory with specified name
    ls [-l] [-a] [-h] <name>   Prints the children of the specified directory
    tree <name>                Prints the tree structure of the specified directory
    attrs <name>               Prints the attributes of the specified file or directory
    cat <name>                 Echo the file content in stdout
    mv <from> <to>             Rename a file or directory
    rm [-f] [-r] <name>        Move the file to trash, or delete it permanently with -f flag
    restore <name>             Restore a file or directory from trash

	Don't forget to put quotes around the command!


```
cozy-stack files exec [--domain domain] <command> [flags]
```

### Options

```
  -h, --help   help for exec
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


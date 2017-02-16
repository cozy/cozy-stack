## cozy-stack files exec

Execute the given command on the specified domain and leave

### Synopsis


Execute a command on the VFS of the specified domain.
Available commands:

    mkdir [name]               Creates a directory with specified name
    ls [-l] [-a] [-h] [name]   Prints the children of the specified directory
    tree [name]                Prints the tree structure of the specified directory
    attrs [name]               Prints the attributes of the specified file or directory
    cat [name]                 Echo the file content in stdout
    mv [from] [to]             Rename a file or directory
    rm [-f] [-r] [name]        Move the file to trash, or delete it permanently with -f flag
    restore [name]             Restore a file or directory from trash


```
cozy-stack files exec [domain] [command]
```

### Options inherited from parent commands

```
      --admin-host string      administration server host (default "localhost")
      --admin-port int         administration server port (default 6060)
      --assets string          path to the directory with the assets (use the packed assets by default)
  -c, --config string          configuration file (default "$HOME/.cozy.yaml")
      --couchdb-url string     CouchDB URL (default "http://localhost:5984/")
      --fs-url string          filesystem url (default "file://localhost//storage")
      --host string            server host (default "localhost")
      --log-level string       define the log level (default "info")
      --mail-disable-tls       disable smtp over tls
      --mail-host string       mail smtp host (default "localhost")
      --mail-password string   mail smtp password
      --mail-port int          mail smtp port (default 465)
      --mail-username string   mail smtp username
  -p, --port int               server port (default 8080)
      --subdomains string      how to structure the subdomains for apps (can be nested or flat) (default "nested")
```

### SEE ALSO
* [cozy-stack files](cozy-stack_files.md)	 - Interact with the cozy filesystem


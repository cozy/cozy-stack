## cozy-stack instances fsck

Check and repair a vfs

### Synopsis


The cozy-stack fsck command checks that the files in the VFS are not
desynchronized, ie a file present in CouchDB but not swift/localfs, or present
in swift/localfs but not couchdb.


```
cozy-stack instances fsck <domain> [flags]
```

### Options

```
      --dry     Don't modify the VFS, only show the inconsistencies
  -h, --help    help for fsck
      --prune   Try to solve inconsistencies by modifying the file system
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

* [cozy-stack instances](cozy-stack_instances.md)	 - Manage instances of a stack


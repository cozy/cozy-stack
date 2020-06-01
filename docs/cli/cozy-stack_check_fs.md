## cozy-stack check fs

Check a vfs

### Synopsis


This command checks that the files in the VFS are not desynchronized, ie a file
present in CouchDB but not swift/localfs, or present in swift/localfs but not
couchdb.

There are 2 steps:

- index integrity checks that there are nothing wrong in the index (CouchDB),
  like a file present in a directory that has been deleted
- files consistency checks that the files are the same in the index (CouchDB)
  and the storage (Swift or localfs).

By default, both operations are done, but you can choose one or the other via
the flags.


```
cozy-stack check fs <domain> [flags]
```

### Options

```
      --fail-fast           Stop the FSCK on the first error
      --files-consistency   Check the files consistency only (between CouchDB and Swift)
  -h, --help                help for fs
      --index-integrity     Check the index integrity only
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


## cozy-stack fix

A set of tools to fix issues or migrate content.

### Synopsis

A set of tools to fix issues or migrate content.

```
cozy-stack fix <command> [flags]
```

### Options

```
  -h, --help   help for fix
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
* [cozy-stack fix contact-emails](cozy-stack_fix_contact-emails.md)	 - Detect and try to fix invalid emails on contacts
* [cozy-stack fix content-mismatch](cozy-stack_fix_content-mismatch.md)	 - Fix the content mismatch differences for 64K issue
* [cozy-stack fix indexes](cozy-stack_fix_indexes.md)	 - Rebuild the CouchDB views and indexes
* [cozy-stack fix jobs](cozy-stack_fix_jobs.md)	 - Take a look at the consistency of the jobs
* [cozy-stack fix md5](cozy-stack_fix_md5.md)	 - Fix missing md5 from contents in the vfs
* [cozy-stack fix mime](cozy-stack_fix_mime.md)	 - Fix the class computed from the mime-type
* [cozy-stack fix orphan-account](cozy-stack_fix_orphan-account.md)	 - Remove the orphan accounts
* [cozy-stack fix redis](cozy-stack_fix_redis.md)	 - Rebuild scheduling data strucutures in redis
* [cozy-stack fix thumbnails](cozy-stack_fix_thumbnails.md)	 - Rebuild thumbnails image for images files


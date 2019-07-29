## cozy-stack fixer

A set of tools to fix issues or migrate content for retro-compatibility.

### Synopsis

A set of tools to fix issues or migrate content for retro-compatibility.

```
cozy-stack fixer <command> [flags]
```

### Options

```
  -h, --help   help for fixer
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
* [cozy-stack fixer albums-created-at](cozy-stack_fixer_albums-created-at.md)	 - Add a created_at field for albums where it's missing
* [cozy-stack fixer contact-emails](cozy-stack_fixer_contact-emails.md)	 - Detect and try to fix invalid emails on contacts
* [cozy-stack fixer content-mismatch](cozy-stack_fixer_content-mismatch.md)	 - Fix the content mismatch differences for 64K issue
* [cozy-stack fixer jobs](cozy-stack_fixer_jobs.md)	 - Take a look at the consistency of the jobs
* [cozy-stack fixer link-app](cozy-stack_fixer_link-app.md)	 - Link an old OAuth client to a webapp
* [cozy-stack fixer md5](cozy-stack_fixer_md5.md)	 - Fix missing md5 from contents in the vfs
* [cozy-stack fixer mime](cozy-stack_fixer_mime.md)	 - Fix the class computed from the mime-type
* [cozy-stack fixer onboardings](cozy-stack_fixer_onboardings.md)	 - Add the onboarding_finished flag to user that have registered their passphrase
* [cozy-stack fixer orphan-account](cozy-stack_fixer_orphan-account.md)	 - Rebuild scheduling data strucutures in redis
* [cozy-stack fixer redis](cozy-stack_fixer_redis.md)	 - Rebuild scheduling data strucutures in redis
* [cozy-stack fixer thumbnails](cozy-stack_fixer_thumbnails.md)	 - Rebuild thumbnails image for images files


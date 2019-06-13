## cozy-stack jobs purge-old-jobs

Purge old jobs from an instance

### Synopsis

Purge old jobs from an instance

```
cozy-stack jobs purge-old-jobs <domain> [flags]
```

### Examples

```
$ cozy-stack jobs purge-old-jobs example.mycozy.cloud
```

### Options

```
      --duration string   duration to look for (ie. 3D, 2M)
  -h, --help              help for purge-old-jobs
      --workers strings   worker types to iterate over (all workers by default)
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

* [cozy-stack jobs](cozy-stack_jobs.md)	 - Launch and manage jobs and workers


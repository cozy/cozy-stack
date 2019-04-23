## cozy-stack instances token-oauth

Generate a new OAuth access token

### Synopsis

Generate a new OAuth access token

```
cozy-stack instances token-oauth <domain> <clientid> <scopes> [flags]
```

### Examples

```
$ cozy-stack instances token-oauth cozy.tools:8080 727e677187a51d14ccd59cc0bd000a1d io.cozy.files io.cozy.jobs:POST:sendmail:worker
```

### Options

```
      --expire duration   Make the token expires in this amount of time
  -h, --help              help for token-oauth
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


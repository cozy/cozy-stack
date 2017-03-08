## cozy-stack instances

Manage instances of a stack

### Synopsis



cozy-stack instances allows to manage the instances of this stack

An instance is a logical space owned by one user and identified by a domain.
For example, bob.cozycloud.cc is the instance of Bob. A single cozy-stack
process can manage several instances.

Each instance has a separate space for storing files and a prefix used to
create its CouchDB databases.


```
cozy-stack instances [command]
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
* [cozy-stack](cozy-stack.md)	 - cozy-stack is the main command
* [cozy-stack instances add](cozy-stack_instances_add.md)	 - Manage instances of a stack
* [cozy-stack instances client-oauth](cozy-stack_instances_client-oauth.md)	 - Register a new OAuth client
* [cozy-stack instances destroy](cozy-stack_instances_destroy.md)	 - Remove instance
* [cozy-stack instances ls](cozy-stack_instances_ls.md)	 - List instances
* [cozy-stack instances token-app](cozy-stack_instances_token-app.md)	 - Generate a new application token
* [cozy-stack instances token-oauth](cozy-stack_instances_token-oauth.md)	 - Generate a new OAuth access token


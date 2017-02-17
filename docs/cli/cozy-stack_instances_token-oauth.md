## cozy-stack instances token-oauth

Generate a new OAuth access token

### Synopsis


Generate a new OAuth access token

```
cozy-stack instances token-oauth [domain] [clientid] [scopes]
```

### Options

```
      --expire duration   Make the token expires in this amount of time
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
* [cozy-stack instances](cozy-stack_instances.md)	 - Manage instances of a stack


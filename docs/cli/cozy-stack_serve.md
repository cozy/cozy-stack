## cozy-stack serve

Starts the stack and listens for HTTP calls

### Synopsis


Starts the stack and listens for HTTP calls
It will accept HTTP requests on localhost:8080 by default.
Use the --port and --host flags to change the listening option.

If you are the developer of a client-side app, you can use --appdir
to mount a directory as the application with the 'app' slug.


```
cozy-stack serve
```

### Examples

```
The most often, this command is used in its simple form:

	$ cozy-stack serve

But if you want to develop two apps in local (to test their interactions for
example), you can use the --appdir flag like this:

	$ cozy-stack serve --appdir appone:/path/to/app_one,apptwo:/path/to/app_two

```

### Options

```
      --allow-root                     Allow to start as root (disabled by default)
      --appdir stringSlice             Mount a directory as the 'app' application
      --assets string                  path to the directory with the assets (use the packed assets by default)
      --cache-url string               URL for the cache, redis or in-memory
      --couchdb-url string             CouchDB URL (default "http://localhost:5984/")
      --downloads-url string           URL for the download secret storage, redis or in-memory
      --fs-url string                  filesystem url (default "file://localhost//storage")
      --jobs-url string                URL for the jobs system synchronization, redis or in-memory
      --jobs-workers int               Number of parallel workers (0 to disable the processing of jobs) (default 8)
      --konnectors-cmd string          konnectors command to be executed
      --konnectors-oauthstate string   URL for the storage of OAuth state for konnectors, redis or in-memory
      --lock-url string                URL for the locks, redis or in-memory
      --log-syslog                     use the local syslog for logging
      --mail-disable-tls               disable smtp over tls
      --mail-host string               mail smtp host (default "localhost")
      --mail-password string           mail smtp password
      --mail-port int                  mail smtp port (default 465)
      --mail-username string           mail smtp username
      --no-admin                       Start without the admin interface
      --sessions-url string            URL for the sessions storage, redis or in-memory
      --subdomains string              how to structure the subdomains for apps (can be nested or flat) (default "nested")
```

### Options inherited from parent commands

```
      --admin-host string   administration server host (default "localhost")
      --admin-port int      administration server port (default 6060)
      --client-use-https    if set the client will use https to communicate with the server
  -c, --config string       configuration file (default "$HOME/.cozy.yaml")
      --host string         server host (default "localhost")
      --log-level string    define the log level (default "info")
  -p, --port int            server port (default 8080)
```

### SEE ALSO
* [cozy-stack](cozy-stack.md)	 - cozy-stack is the main command


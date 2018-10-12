## cozy-stack serve

Starts the stack and listens for HTTP calls

### Synopsis

Starts the stack and listens for HTTP calls
It will accept HTTP requests on localhost:8080 by default.
Use the --port and --host flags to change the listening option.

The SIGINT signal will trigger a graceful stop of cozy-stack: it will wait that
current HTTP requests and jobs are finished (in a limit of 2 minutes) before
exiting.

If you are the developer of a client-side app, you can use --appdir
to mount a directory as the application with the 'app' slug.


```
cozy-stack serve [flags]
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
      --allow-root                       Allow to start as root (disabled by default)
      --appdir strings                   Mount a directory as the 'app' application
      --assets string                    path to the directory with the assets (use the packed assets by default)
      --couchdb-url string               CouchDB URL (default "http://localhost:5984/")
      --csp-whitelist string             Whitelisted domains for the default allowed origins of the Content Secury Policy
      --dev                              Allow to run without in dev release mode (disabled by default)
      --disable-csp                      Disable the Content Security Policy (only available for development)
      --doctypes string                  path to the directory with the doctypes (for developing/testing a remote doctype)
      --downloads-url string             URL for the download secret storage, redis or in-memory
      --fs-url string                    filesystem url (default "file:///storage")
      --geodb string                     define the location of the database for IP -> City lookups (default ".")
  -h, --help                             help for serve
      --hooks string                     define the directory used for hook scripts (default ".")
      --jobs-url string                  URL for the jobs system synchronization, redis or in-memory
      --konnectors-cmd string            konnectors command to be executed
      --konnectors-oauthstate string     URL for the storage of OAuth state for konnectors, redis or in-memory
      --lock-url string                  URL for the locks, redis or in-memory
      --log-level string                 define the log level (default "info")
      --log-syslog                       use the local syslog for logging
      --mail-disable-tls                 disable smtp over tls
      --mail-host string                 mail smtp host (default "localhost")
      --mail-noreply-address string      mail address used for sending mail as a noreply (forgot passwords for example)
      --mail-noreply-name string         mail name used for sending mail as a noreply (forgot passwords for example)
      --mail-password string             mail smtp password
      --mail-port int                    mail smtp port (default 465)
      --mail-username string             mail smtp username
      --password-reset-interval string   minimal duration between two password reset (default "15m")
      --realtime-url string              URL for realtime in the browser via webocket, redis or in-memory
      --sessions-url string              URL for the sessions storage, redis or in-memory
      --subdomains string                how to structure the subdomains for apps (can be nested or flat) (default "nested")
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


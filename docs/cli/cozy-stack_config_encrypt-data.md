## cozy-stack config encrypt-data

Encrypt data with the specified encryption keyfile.

### Synopsis

cozy-stack config encrypt-data encrypts any valid JSON data

```
cozy-stack config encrypt-data <encoding keyfile> <text> [flags]
```

### Examples

```

$ ./cozy-stack config encrypt-data ~/.cozy/key.enc "{\"foo\": \"bar\"}"
$ bmFjbNFjY+XZkS26YtVPUIKKm/JdnAGwG30n6A4ypS1p1dHev8hOtaRbW+lGneoO7PS9JCW8U5GSXhASu+c3UkaZ

```

### Options

```
  -h, --help   help for encrypt-data
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

* [cozy-stack config](cozy-stack_config.md)	 - Show and manage configuration elements


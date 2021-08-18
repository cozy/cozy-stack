## cozy-stack tools encrypt-with-rsa

encrypt a payload in RSA

### Synopsis


This command is used by integration tests to encrypt bitwarden organization
keys. It takes the public or private key of the user and the payload (= the
organization key) as inputs (both encoded in base64), and print on stdout the
encrypted data (encoded as base64 too).


```
cozy-stack tools encrypt-with-rsa <key> <payload [flags]
```

### Options

```
  -h, --help   help for encrypt-with-rsa
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

* [cozy-stack tools](cozy-stack_tools.md)	 - Regroup some tools for debugging and tests


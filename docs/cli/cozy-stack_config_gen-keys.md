## cozy-stack config gen-keys

Generate an key pair for encryption and decryption of credentials

### Synopsis


cozy-stack config gen-keys generate a key-pair and save them in the
specified path.

The decryptor key filename is given the ".dec" extension suffix.
The encryptor key filename is given the ".enc" extension suffix.

The files permissions are 0400.

example: cozy-stack config gen-keys ~/credentials-key
keyfiles written in:
	~/credentials-key.enc
	~/credentials-key.dec


```
cozy-stack config gen-keys <filepath> [flags]
```

### Options

```
  -h, --help   help for gen-keys
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


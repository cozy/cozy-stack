## cozy-stack tools unxor-document-id

transform the id of a shared document

### Synopsis


This command can be used when you have the identifier of a shared document on a
recipient instance, and you want the identifier of the same document on the
owner's instance.


```
cozy-stack tools unxor-document-id <domain> <sharing_id> <document_id> [flags]
```

### Examples

```

If you a log message like:

	PUT http://bob.localhost:8080/sharings/7f47c470c7b1013a8a8818c04daba326/io.cozy.files/4ded650c803f67a4 500 Internal Server Error

you can execute this command:

    $ cozy-stack tools unxor-document-id bob.localhost:8080 7f47c470c7b1013a8a8818c04daba326 4ded650c803f67a4

```

### Options

```
  -h, --help   help for unxor-document-id
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


## cozy-stack completion

Output shell completion code for the specified shell

### Synopsis


Output shell completion code for the specified shell (bash or zsh).
The shell code must be evalutated to provide interactive
completion of kubectl commands.  This can be done by sourcing it from
the .bash_profile.

Note: this requires the bash-completion framework, which is not installed
by default on Mac.  This can be installed by using homebrew:

    $ brew install bash-completion

Once installed, bash_completion must be evaluated.  This can be done by adding the
following line to the .bash_profile

    $ source $(brew --prefix)/etc/bash_completion

```
cozy-stack completion <shell> [flags]
```

### Examples

```
# cozy-stack completion bash > /etc/bash_completion.d/cozy-stack
```

### Options

```
  -h, --help   help for completion
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


## cozy-stack completion

Output shell completion code for the specified shell

### Synopsis


Output shell completion code for the specified shell (bash, zsh, or fish). The
shell code must be evalutated to provide interactive completion of cozy-stack
commands.

Bash:

  $ source <(cozy-stack completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ cozy-stack completion bash > /etc/bash_completion.d/cozy-stack
  # macOS:
  $ cozy-stack completion bash > $(brew --prefix)/etc/bash_completion.d/cozy-stack

Note: this requires the bash-completion framework, which is not installed by
default on Mac.  This can be installed by using homebrew:

    $ brew install bash-completion

Once installed, bash_completion must be evaluated.  This can be done by adding the
following line to the .bash_profile

    $ source $(brew --prefix)/etc/bash_completion

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ cozy-stack completion zsh > "${fpath[1]}/_cozy-stack"

  # You will need to start a new shell for this setup to take effect.

fish:

  $ cozy-stack completion fish | source

  # To load completions for each session, execute once:
  $ cozy-stack completion fish > /etc/fish/completions/cozy-stack.fish


```
cozy-stack completion <shell> [flags]
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


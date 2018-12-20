## cozy-stack instances client-oauth

Register a new OAuth client

### Synopsis

It registers a new OAuth client and returns its client_id

```
cozy-stack instances client-oauth <domain> <redirect_uri> <client_name> <software_id> [flags]
```

### Options

```
      --allow-login-scope               Allow login scope
  -h, --help                            help for client-oauth
      --json                            Output more informations in JSON format
      --onboarding-app string           Specify an OnboardingApp
      --onboarding-permissions string   Specify an OnboardingPermissions
      --onboarding-secret string        Specify an OnboardingSecret
      --onboarding-state string         Specify an OnboardingState
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

* [cozy-stack instances](cozy-stack_instances.md)	 - Manage instances of a stack


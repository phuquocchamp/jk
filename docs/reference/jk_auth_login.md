## jk auth login

Store an API token in the OS keyring for the profile

### Synopsis

Store a Jenkins API token in the OS keyring (service "jk", account = profile).

With --with-token the token is read from stdin, e.g.
  printf %s "$TOKEN" | jk auth login --with-token
Without it, a hidden prompt is shown only when stdin is a terminal.
The environment variable JENKINS_API_TOKEN takes precedence over the keyring.

```
jk auth login [flags]
```

### Options

```
  -h, --help         help for login
      --with-token   read the token from stdin
```

### Options inherited from parent commands

```
      --dry-run          show the plan for write operations without executing
      --fields strings   comma-separated top-level fields to keep (per item for lists)
      --jq string        jq expression applied to JSON output
      --json             JSON output (default when stdout is not a TTY)
      --profile string   config profile to use
      --tier string      lower the effective tier: read|write|danger
  -y, --yes              confirm dangerous operations
```

### SEE ALSO

* [jk auth](jk_auth.md)	 - Manage credentials and check who you are


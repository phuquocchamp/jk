---
title: "jk auth login"
description: "Check an API token against Jenkins, store it in the OS keyring and save the profile"
---

Check an API token against Jenkins, store it in the OS keyring and save the profile

### Synopsis

Store a Jenkins API token in the OS keyring (service "jk", account = profile).

The URL and user come from --url / --user, JENKINS_URL / JENKINS_USER, or the
profile in the config file. When they are missing and stdin is a terminal,
login asks for them.

Before storing, jk calls whoami on that URL as that user with the new token,
and stores nothing if Jenkins rejects it. --no-verify skips the check (e.g.
Jenkins is unreachable from here).

When the token checks out and the profile is not in the config file yet,
login writes it there (url, user and policy tier read), creating the file
with mode 0600 if needed. Existing profiles are never changed; raise the tier
by editing the file. With --no-verify nothing is written to the config.

With --with-token the token is read from stdin, e.g.
  printf %s "$TOKEN" | jk auth login --url https://jenkins.example.com --user jane --with-token
Without it, a hidden prompt is shown only when stdin is a terminal.
The environment variable JENKINS_API_TOKEN takes precedence over the keyring.

```
jk auth login [flags]
```

### Options

```
  -h, --help          help for login
      --no-verify     store the token without checking it against Jenkins (and without saving the profile)
      --url string    Jenkins URL for this login (default: JENKINS_URL or the profile's url)
      --user string   Jenkins user for this login (default: JENKINS_USER or the profile's user)
      --with-token    read the token from stdin
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

* [jk auth](/jk/reference/jk_auth/)	 - Manage credentials and check who you are


---
title: "jk auth"
description: "Manage credentials and check who you are"
---

Manage credentials and check who you are

```
jk auth [flags]
```

### Options

```
  -h, --help   help for auth
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

* [jk](/jk/reference/jk/)	 - Agent-first Jenkins CLI and MCP server
* [jk auth login](/jk/reference/jk_auth_login/)	 - Check an API token against Jenkins, store it in the OS keyring and save the profile
* [jk auth logout](/jk/reference/jk_auth_logout/)	 - Remove the profile's API token from the OS keyring
* [jk auth status](/jk/reference/jk_auth_status/)	 - Check which Jenkins user the token authenticates as; returns id, full name and base URL.


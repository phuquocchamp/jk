---
title: "jk auth status"
description: "Check which Jenkins user the token authenticates as; returns id, full name and base URL."
---

Check which Jenkins user the token authenticates as; returns id, full name and base URL.

### Synopsis

Check which Jenkins user the token authenticates as; returns id, full name and base URL.

Also shows the profile, effective tier and where the token came from
(env:&lt;VAR> or keyring). The token itself is never printed.

```
jk auth status [flags]
```

### Options

```
  -h, --help   help for status
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


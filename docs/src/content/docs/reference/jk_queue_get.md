---
title: "jk queue get"
description: "Follow a triggered build: returns why a queue item waits, or its build number once started."
---

Follow a triggered build: returns why a queue item waits, or its build number once started.

```
jk queue get <id> [flags]
```

### Options

```
  -h, --help   help for get
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

* [jk queue](/jk/reference/jk_queue/)	 - Follow queued builds


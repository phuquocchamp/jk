---
title: "jk job list"
description: "Discover jobs: lists direct children of a folder or multibranch project (root if empty); returns name, path, kind, color."
---

Discover jobs: lists direct children of a folder or multibranch project (root if empty); returns name, path, kind, color.

```
jk job list [folder] [flags]
```

### Options

```
  -h, --help        help for list
      --limit int   max entries to return (default 50)
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

* [jk job](/jk/reference/jk_job/)	 - Discover and inspect jobs


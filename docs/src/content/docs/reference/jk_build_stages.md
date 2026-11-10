---
title: "jk build stages"
description: "Find which pipeline stage failed: returns stages with status and duration."
---

Find which pipeline stage failed: returns stages with status and duration.

### Synopsis

Find which pipeline stage failed: returns stages with status and duration.

&lt;job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk build stages <job> [ref] [flags]
```

### Options

```
  -h, --help   help for stages
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

* [jk build](/jk/reference/jk_build/)	 - Inspect, trigger, replay and stop builds


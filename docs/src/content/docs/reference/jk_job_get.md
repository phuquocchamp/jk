---
title: "jk job get"
description: "Inspect a job before building: returns kind, last build numbers and parameter definitions."
---

Inspect a job before building: returns kind, last build numbers and parameter definitions.

### Synopsis

Inspect a job before building: returns kind, last build numbers and parameter definitions.

&lt;job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk job get <job> [flags]
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

* [jk job](/jk/reference/jk_job/)	 - Discover and inspect jobs


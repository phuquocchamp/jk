---
title: "jk build trigger"
description: "Start a build (with parameters if the job has them); returns queue id, and build number/result when waiting. Supports dry_run."
---

Start a build (with parameters if the job has them); returns queue id, and build number/result when waiting. Supports dry_run.

### Synopsis

Start a build (with parameters if the job has them); returns queue id, and build number/result when waiting. Supports dry_run.

Parameters are passed with -p name=value (repeatable). Jobs with parameter
definitions are triggered through buildWithParameters. --wait polls the queue
item and then the build until it finishes or --timeout (seconds or a Go
duration such as 10m; default 600s) expires. --dry-run prints the plan.

&lt;job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk build trigger <job> [-p k=v ...] [--wait [--timeout 10m]] [flags]
```

### Options

```
  -h, --help                help for trigger
  -p, --param stringArray   build parameter name=value (repeatable)
      --timeout string      wait timeout: seconds or a duration like 10m (default 600s)
      --wait                wait for the build to start and finish
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


---
title: "jk build wait"
description: "Wait for a queued or running build to finish (bounded); returns result, or timed_out with next to call again."
---

Wait for a queued or running build to finish (bounded); returns result, or timed_out with next to call again.

### Synopsis

Wait for a queued or running build to finish (bounded); returns result, or timed_out with next to call again.

Pass the build number as [ref], or --queue-id with the queue_id that
'build trigger' printed when the build had not started yet. --timeout takes
seconds or a Go duration (default 600s). Timing out is not an error: the
output has timed_out=true and the command to run next.

&lt;job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk build wait <job> [ref] [--queue-id N] [--timeout 10m] [flags]
```

### Options

```
  -h, --help             help for wait
      --queue-id int     queue item id from 'build trigger'
      --timeout string   wait timeout: seconds or a duration like 10m (default 600s)
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

* [jk build](/jk/reference/jk_build/)	 - Inspect, diagnose, trigger, replay and stop builds


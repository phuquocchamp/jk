---
title: "jk build diagnose"
description: "Start here for a failed build: one call returns the result, the first failing stage and step (command + error), the end of that step's log, failed tests and SCM changes."
---

Start here for a failed build: one call returns the result, the first failing stage and step (command + error), the end of that step's log, failed tests and SCM changes.

### Synopsis

Start here for a failed build: one call returns the result, the first failing stage and step (command + error), the end of that step's log, failed tests and SCM changes.

[ref] defaults to lastFailed here. --lines sets how much of the failing step's
log is included (default 60, capped by policy max_log_lines).

&lt;job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk build diagnose <job> [ref] [flags]
```

### Options

```
  -h, --help            help for diagnose
  -n, --lines int       lines of the failing step's log (default 60)
      --max-tests int   max failed test cases (default 10)
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


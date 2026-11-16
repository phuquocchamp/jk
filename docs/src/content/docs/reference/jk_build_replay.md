---
title: "jk build replay"
description: "Re-run a pipeline build with a modified Jenkinsfile (main script; loaded scripts unchanged). Runs arbitrary pipeline code: requires confirm=true, otherwise NEEDS_CONFIRMATION with a diff against the original. Supports dry_run."
---

Re-run a pipeline build with a modified Jenkinsfile (main script; loaded scripts unchanged). Runs arbitrary pipeline code: requires confirm=true, otherwise NEEDS_CONFIRMATION with a diff against the original. Supports dry_run.

### Synopsis

Re-run a pipeline build with a modified Jenkinsfile (main script; loaded scripts unchanged). Runs arbitrary pipeline code: requires confirm=true, otherwise NEEDS_CONFIRMATION with a diff against the original. Supports dry_run.

--script-file is the Jenkinsfile to run instead of the original main script;
use "-" to read it from stdin ('jk build script' prints the original). Loaded
libraries/secondary scripts are not replaced. Replay runs arbitrary pipeline
code (tier danger): pass --yes to confirm. Without it the command exits 7
(NEEDS_CONFIRMATION) with the plan and a diff against the original; on an
interactive terminal it asks first. --dry-run prints the plan.

&lt;job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk build replay <job> <ref> --script-file F [flags]
```

### Options

```
  -h, --help                 help for replay
      --script-file string   Jenkinsfile to replay with ("-" for stdin)
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


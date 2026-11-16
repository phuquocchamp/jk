---
title: "jk build script"
description: "Read the Jenkinsfile a build ran (from its replay form) before editing it for build_replay; needs the Replay permission."
---

Read the Jenkinsfile a build ran (from its replay form) before editing it for build_replay; needs the Replay permission.

### Synopsis

Read the Jenkinsfile a build ran (from its replay form) before editing it for build_replay; needs the Replay permission.

Human output is the script itself, so 'jk build script JOB 42 > Jenkinsfile'
gives a file to edit and pass to 'jk build replay --script-file'.

&lt;job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk build script <job> [ref] [flags]
```

### Options

```
  -h, --help   help for script
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


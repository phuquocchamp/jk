---
title: "jk log tail"
description: "Read the end of a build log: whole console, one stage, or only the failing step (step=failed); returns the last N lines, capped by policy, with ANSI and timestamps stripped."
---

Read the end of a build log: whole console, one stage, or only the failing step (step=failed); returns the last N lines, capped by policy, with ANSI and timestamps stripped.

### Synopsis

Read the end of a build log: whole console, one stage, or only the failing step (step=failed); returns the last N lines, capped by policy, with ANSI and timestamps stripped.

--lines defaults to and is capped by the policy max_log_lines. --stage limits
the output to one pipeline stage (name or id, see 'build stages'): the logs of
its steps, including nested stages, read from the last step backwards until
there are enough lines. --step failed reads only the failing step (of --stage,
or of the stage where the build first failed); --step also takes a step id.
ANSI codes and timestamper prefixes are stripped unless --raw is set.

&lt;job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk log tail <job> [ref] [flags]
```

### Options

```
  -h, --help           help for tail
  -n, --lines int      number of trailing lines (default: policy max_log_lines)
      --raw            keep ANSI codes and timestamp prefixes
      --stage string   only this stage's log (name or id)
      --step string    "failed" for only the failing step's log, or a step id
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

* [jk log](/jk/reference/jk_log/)	 - Read and search build logs


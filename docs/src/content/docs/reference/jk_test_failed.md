---
title: "jk test failed"
description: "List failed JUnit test cases of a build with trimmed error details; has_report=false when there is no report."
---

List failed JUnit test cases of a build with trimmed error details; has_report=false when there is no report.

### Synopsis

List failed JUnit test cases of a build with trimmed error details; has_report=false when there is no report.

&lt;job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk test failed <job> [ref] [flags]
```

### Options

```
  -h, --help        help for failed
      --limit int   max failed cases to return (default 50)
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

* [jk test](/jk/reference/jk_test/)	 - Inspect JUnit test results


## jk build stop

Abort a running build; requires confirm=true, otherwise returns NEEDS_CONFIRMATION with the plan.

### Synopsis

Abort a running build; requires confirm=true, otherwise returns NEEDS_CONFIRMATION with the plan.

Pass --yes to confirm. Without it the command exits 7 (NEEDS_CONFIRMATION)
with the plan in the error details; on an interactive terminal it asks first.

<job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk build stop <job> <ref> [flags]
```

### Options

```
  -h, --help   help for stop
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

* [jk build](jk_build.md)	 - Inspect, trigger, replay and stop builds


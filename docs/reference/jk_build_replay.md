## jk build replay

Re-run a pipeline build with a modified Jenkinsfile (main script only); returns queue id if known. Supports dry_run.

### Synopsis

Re-run a pipeline build with a modified Jenkinsfile (main script only); returns queue id if known. Supports dry_run.

--script-file is the Jenkinsfile to run instead of the original main script;
use "-" to read it from stdin. Loaded libraries/secondary scripts are not
replaced. --dry-run prints the plan.

<job> is a job path like folder/sub/job (multibranch: project/branch, with
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

* [jk build](jk_build.md)	 - Inspect, trigger, replay and stop builds


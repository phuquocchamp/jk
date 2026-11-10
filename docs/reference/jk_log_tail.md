## jk log tail

Read the end of a build log (whole console or one stage); returns the last N lines, capped by policy.

### Synopsis

Read the end of a build log (whole console or one stage); returns the last N lines, capped by policy.

--lines defaults to and is capped by the policy max_log_lines. --stage limits
the output to one pipeline stage (name or id, see 'build stages').

<job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk log tail <job> [ref] [flags]
```

### Options

```
  -h, --help           help for tail
  -n, --lines int      number of trailing lines (default: policy max_log_lines)
      --stage string   only this stage's log (name or id)
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

* [jk log](jk_log.md)	 - Read and search build logs


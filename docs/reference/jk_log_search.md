## jk log search

Grep a build log by regex without downloading it; returns matching lines with line numbers and context.

### Synopsis

Grep a build log by regex without downloading it; returns matching lines with line numbers and context.

--pattern is a Go RE2 regular expression matched per line.

<job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).

```
jk log search <job> [ref] --pattern RE [flags]
```

### Options

```
  -C, --context int       lines of context around each match
  -h, --help              help for search
      --max-matches int   stop after this many matches (default 50)
  -e, --pattern string    regular expression (required)
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


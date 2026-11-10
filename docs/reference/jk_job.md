## jk job

Discover and inspect jobs

```
jk job [flags]
```

### Options

```
  -h, --help   help for job
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

* [jk](jk.md)	 - Agent-first Jenkins CLI and MCP server
* [jk job get](jk_job_get.md)	 - Inspect a job before building: returns kind, last build numbers and parameter definitions.
* [jk job list](jk_job_list.md)	 - Discover jobs: lists direct children of a folder or multibranch project (root if empty); returns name, path, kind, color.


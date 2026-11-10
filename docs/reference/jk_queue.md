## jk queue

Follow queued builds

```
jk queue [flags]
```

### Options

```
  -h, --help   help for queue
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
* [jk queue get](jk_queue_get.md)	 - Follow a triggered build: returns why a queue item waits, or its build number once started.


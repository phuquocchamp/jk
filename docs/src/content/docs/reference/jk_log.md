---
title: "jk log"
description: "Read and search build logs"
---

Read and search build logs

```
jk log [flags]
```

### Options

```
  -h, --help   help for log
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

* [jk](/jk/reference/jk/)	 - Agent-first Jenkins CLI and MCP server
* [jk log search](/jk/reference/jk_log_search/)	 - Grep a build log by regex without downloading it; returns matching lines with line numbers and context.
* [jk log tail](/jk/reference/jk_log_tail/)	 - Read the end of a build log (whole console or one stage); returns the last N lines, capped by policy.


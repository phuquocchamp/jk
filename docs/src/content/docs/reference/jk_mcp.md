---
title: "jk mcp"
description: "Run the MCP server on stdio"
---

Run the MCP server on stdio

### Synopsis

Run jk as an MCP server over stdio. Tools above the effective tier
(policy.tier, optionally lowered with --tier) are not registered.

```
jk mcp [flags]
```

### Options

```
  -h, --help   help for mcp
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


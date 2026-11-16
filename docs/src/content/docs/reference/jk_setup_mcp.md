---
title: "jk setup mcp"
description: "Register 'jk mcp' with Claude Code, or print the JSON entry for other clients"
---

Register 'jk mcp' with Claude Code, or print the JSON entry for other clients

### Synopsis

--client claude (the default when 'claude' is on PATH) runs
  claude mcp add --scope SCOPE jk -- /absolute/path/to/jk mcp [--tier TIER]
--client print outputs the server entry (with the absolute path to jk) for
Claude Desktop, Cursor and other clients configured with JSON.

The token is never passed to the client: 'jk mcp' reads it from the keyring
or JENKINS_API_TOKEN at start. --tier pins the server below the config tier.
--dry-run shows the command without running it.

```
jk setup mcp [--client claude|print] [--scope local|project|user] [--tier read] [flags]
```

### Options

```
      --client string   claude or print (default: claude when it is on PATH)
  -h, --help            help for mcp
      --scope string    Claude Code scope: local, project or user (default "local")
      --tier string     pin the server's tier (read, write, danger)
```

### Options inherited from parent commands

```
      --dry-run          show the plan for write operations without executing
      --fields strings   comma-separated top-level fields to keep (per item for lists)
      --jq string        jq expression applied to JSON output
      --json             JSON output (default when stdout is not a TTY)
      --profile string   config profile to use
  -y, --yes              confirm dangerous operations
```

### SEE ALSO

* [jk setup](/jk/reference/jk_setup/)	 - Install the agent skill and register jk as an MCP server


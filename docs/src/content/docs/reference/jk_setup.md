---
title: "jk setup"
description: "Install the agent skill and register jk as an MCP server"
---

Install the agent skill and register jk as an MCP server

### Synopsis

Without a subcommand, prints the steps to set jk up for an agent.

  jk setup skill   writes the jenkins skill to ~/.claude/skills/jenkins
  jk setup mcp     registers 'jk mcp' with Claude Code, or prints the JSON entry

```
jk setup [flags]
```

### Options

```
  -h, --help   help for setup
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
* [jk setup mcp](/jk/reference/jk_setup_mcp/)	 - Register 'jk mcp' with Claude Code, or print the JSON entry for other clients
* [jk setup skill](/jk/reference/jk_setup_skill/)	 - Write the jenkins agent skill (default ~/.claude/skills/jenkins)


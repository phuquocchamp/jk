---
title: "jk setup skill"
description: "Write the jenkins agent skill (default ~/.claude/skills/jenkins)"
---

Write the jenkins agent skill (default ~/.claude/skills/jenkins)

### Synopsis

Writes the jenkins skill bundled in this binary to DIR/jenkins (default
~/.claude/skills). A file that differs from the bundled copy is not replaced
without --force (exit 5, CONFLICT). --dry-run lists what would be written.

```
jk setup skill [--dir DIR] [--force] [flags]
```

### Options

```
      --dir string   skills directory (default ~/.claude/skills)
      --force        replace files that differ from the bundled skill
  -h, --help         help for skill
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

* [jk setup](/jk/reference/jk_setup/)	 - Install the agent skill and register jk as an MCP server


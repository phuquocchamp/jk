---
title: "jk"
description: "Agent-first Jenkins CLI and MCP server"
---

Agent-first Jenkins CLI and MCP server

### Synopsis

jk reads and drives Jenkins from the shell or as an MCP server.

Output is JSON when --json or --jq is set or when stdout is not a terminal.
Errors go to stderr ({"error":{...}} in JSON mode) with stable exit codes:
2 usage, 3 not found, 4 policy denied/forbidden, 5 conflict, 6 auth required,
7 needs confirmation, 1 upstream/internal.

### Options

```
      --dry-run          show the plan for write operations without executing
      --fields strings   comma-separated top-level fields to keep (per item for lists)
  -h, --help             help for jk
      --jq string        jq expression applied to JSON output
      --json             JSON output (default when stdout is not a TTY)
      --profile string   config profile to use
      --tier string      lower the effective tier: read|write|danger
  -y, --yes              confirm dangerous operations
```

### SEE ALSO

* [jk auth](/jk/reference/jk_auth/)	 - Manage credentials and check who you are
* [jk build](/jk/reference/jk_build/)	 - Inspect, trigger, replay and stop builds
* [jk completion](/jk/reference/jk_completion/)	 - Generate the autocompletion script for the specified shell
* [jk job](/jk/reference/jk_job/)	 - Discover and inspect jobs
* [jk log](/jk/reference/jk_log/)	 - Read and search build logs
* [jk mcp](/jk/reference/jk_mcp/)	 - Run the MCP server on stdio
* [jk queue](/jk/reference/jk_queue/)	 - Follow queued builds
* [jk test](/jk/reference/jk_test/)	 - Inspect JUnit test results
* [jk version](/jk/reference/jk_version/)	 - Print the jk version


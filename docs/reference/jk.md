## jk

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

* [jk auth](jk_auth.md)	 - Manage credentials and check who you are
* [jk build](jk_build.md)	 - Inspect, trigger, replay and stop builds
* [jk completion](jk_completion.md)	 - Generate the autocompletion script for the specified shell
* [jk job](jk_job.md)	 - Discover and inspect jobs
* [jk log](jk_log.md)	 - Read and search build logs
* [jk mcp](jk_mcp.md)	 - Run the MCP server on stdio
* [jk queue](jk_queue.md)	 - Follow queued builds
* [jk test](jk_test.md)	 - Inspect JUnit test results
* [jk version](jk_version.md)	 - Print the jk version


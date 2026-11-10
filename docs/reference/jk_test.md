## jk test

Inspect JUnit test results

```
jk test [flags]
```

### Options

```
  -h, --help   help for test
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
* [jk test failed](jk_test_failed.md)	 - List failed JUnit test cases of a build with trimmed error details; has_report=false when there is no report.


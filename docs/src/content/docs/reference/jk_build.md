---
title: "jk build"
description: "Inspect, trigger, replay and stop builds"
---

Inspect, trigger, replay and stop builds

```
jk build [flags]
```

### Options

```
  -h, --help   help for build
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
* [jk build get](/jk/reference/jk_build_get/)	 - Check a build's outcome: returns result, building flag, duration, causes, parameters, revision and changes.
* [jk build replay](/jk/reference/jk_build_replay/)	 - Re-run a pipeline build with a modified Jenkinsfile (main script only); returns queue id if known. Supports dry_run.
* [jk build stages](/jk/reference/jk_build_stages/)	 - Find which pipeline stage failed: returns stages with status and duration.
* [jk build stop](/jk/reference/jk_build_stop/)	 - Abort a running build; requires confirm=true, otherwise returns NEEDS_CONFIRMATION with the plan.
* [jk build trigger](/jk/reference/jk_build_trigger/)	 - Start a build (with parameters if the job has them); returns queue id, and build number/result when waiting. Supports dry_run.


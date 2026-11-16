---
title: "jk build"
description: "Inspect, diagnose, trigger, replay and stop builds"
---

Inspect, diagnose, trigger, replay and stop builds

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
* [jk build diagnose](/jk/reference/jk_build_diagnose/)	 - Start here for a failed build: one call returns the result, the first failing stage and step (command + error), the end of that step's log, failed tests and SCM changes.
* [jk build get](/jk/reference/jk_build_get/)	 - Check a build's outcome: returns result, building flag, duration, causes, parameters, revision and changes.
* [jk build replay](/jk/reference/jk_build_replay/)	 - Re-run a pipeline build with a modified Jenkinsfile (main script; loaded scripts unchanged). Runs arbitrary pipeline code: requires confirm=true, otherwise NEEDS_CONFIRMATION with a diff against the original. Supports dry_run.
* [jk build script](/jk/reference/jk_build_script/)	 - Read the Jenkinsfile a build ran (from its replay form) before editing it for build_replay; needs the Replay permission.
* [jk build stages](/jk/reference/jk_build_stages/)	 - Find which pipeline stage failed: returns stages (nested, with parent_id) with status, error and the failing step; first_failed names the stage to read, inherited marks parents that only failed because of it.
* [jk build stop](/jk/reference/jk_build_stop/)	 - Abort a running build; requires confirm=true, otherwise returns NEEDS_CONFIRMATION with the plan.
* [jk build trigger](/jk/reference/jk_build_trigger/)	 - Start a build; unknown parameter names and invalid choice values are rejected. Returns queue id, and build number/result when waiting (MCP waits are capped; continue with build_wait). Supports dry_run.
* [jk build wait](/jk/reference/jk_build_wait/)	 - Wait for a queued or running build to finish (bounded); returns result, or timed_out with next to call again.


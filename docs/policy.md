# Policy

A policy in each [config profile](configuration.md) decides which operations
jk will run. The default is **read only**.

## Tiers

| tier | allows |
|---|---|
| `read` (default) | `whoami`, `job_list`, `job_get`, `build_get`, `build_stages`, `build_log`, `log_search`, `test_failed`, `queue_get` |
| `write` | read, plus `build_trigger` and `build_replay` |
| `danger` | write, plus `build_stop` |

- **Tighten-only.** `--tier` (CLI and `jk mcp --tier`) can only lower the
  tier set in the config. Asking for more is `POLICY_DENIED`. The `JENKINS_*`
  variables cannot raise it either.
- **In MCP**, tools above the effective tier are not registered at all.

!!! danger "The tier is not a sandbox"
    An agent that can run shell commands can point `XDG_CONFIG_HOME` at a
    config file it wrote, or pick a more permissive `--profile`. The real
    boundary is the Jenkins user's permissions, or an MCP server started by you
    with a fixed `--tier` (it logs its profile, config path and tier to stderr
    at startup).

## Allow and deny job globs

```yaml
policy:
  allow: { jobs: ["team-a/**"] }   # empty = all jobs
  deny:  { jobs: ["prod/**"] }     # deny wins over allow
```

- `allow.jobs` / `deny.jobs` are
  [doublestar](https://github.com/bmatcuk/doublestar) globs on the job path with
  `/` separators. They apply to every job operation.
- `team-a/**` matches `team-a` and everything below it.
- Matching is case-insensitive, like Jenkins' own job lookup, so
  `deny: ["prod/**"]` also blocks `PROD/app`.
- `job list` also shows folders on the way to an allowed job: with
  `allow: ["team-a/app/**"]`, the root listing shows `team-a` and
  `job list team-a` shows `team-a/app`. Other operations on `team-a` itself
  stay denied.

## Safety rails for writes

- **Dry run.** Write operations accept `--dry-run` (MCP: `dry_run`), which
  returns the request plan without sending it.
- **Confirmation.** `build stop` needs `--yes` (MCP: `confirm: true`). Without
  it, it exits 7 (`NEEDS_CONFIRMATION`) with the plan in `error.details`, and
  the hint names the exact build number to confirm, so a later `last` cannot
  point at another build. It prompts only when stdin and stdout are both
  terminals.
- **No double triggers.** If an error happens after a build was queued, the
  error's `details` contain the `queue_id`. Follow it with `queue get` instead
  of triggering again.

# jk command reference

Every command, argument and flag, checked against `internal/cli/*.go`.
Use this when you need an exact flag name or default. For workflows, see
[recipes.md](recipes.md); for rules and exit codes, see [../SKILL.md](../SKILL.md).

## Arguments

- `<job>`: `folder/sub/job`, `project/branch` (a `/` inside a branch name is
  written `%2F`), or a pasted Jenkins URL.
- `[ref]`: a build number, `last` (default), `lastFailed`, `lastSuccessful` or
  `lastCompleted`. A build number inside a pasted URL is used when no ref is
  given. `build replay` and `build stop` require an explicit ref.

## Global flags (every command)

| flag | meaning |
|---|---|
| `--profile NAME` | config profile (default: `default_profile`, then `default`) |
| `--json` | JSON output (automatic when stdout is not a TTY, or with `--jq`) |
| `--fields a,b` | keep only these top-level keys (per item for lists) |
| `--jq EXPR` | filter the JSON with a jq (gojq) expression |
| `-y, --yes` | confirm a danger operation (needs the user's approval) |
| `--dry-run` | return a write operation's plan without sending it |
| `--tier read\|write\|danger` | lower the effective tier; asking for more is `POLICY_DENIED` |

## Commands

| command | args | flags (default) | tier | MCP tool |
|---|---|---|---|---|
| `auth login` | none | `--with-token` (read token from stdin; else hidden TTY prompt) | – | – |
| `auth logout` | none | – | – | – |
| `auth status` | none | – | read | `jenkins_whoami` |
| `job list` | `[folder]` | `--limit N` (50) | read | `jenkins_job_list` |
| `job get` | `<job>` | – | read | `jenkins_job_get` |
| `build get` | `<job> [ref]` | – | read | `jenkins_build_get` |
| `build stages` | `<job> [ref]` | – | read | `jenkins_build_stages` |
| `log tail` | `<job> [ref]` | `-n, --lines N` (policy `max_log_lines`), `--stage NAME\|ID` | read | `jenkins_build_log` |
| `log search` | `<job> [ref]` | `-e, --pattern RE` (required, Go regex), `-C, --context N` (0), `--max-matches N` (50) | read | `jenkins_log_search` |
| `test failed` | `<job> [ref]` | `--limit N` (50) | read | `jenkins_test_failed` |
| `queue get` | `<id>` | – | read | `jenkins_queue_get` |
| `build trigger` | `<job>` | `-p, --param k=v` (repeatable), `--wait`, `--timeout` (seconds or `20m`; 600s; only with `--wait`), `--dry-run` | write | `jenkins_build_trigger` |
| `build replay` | `<job> <ref>` | `--script-file F` (required; `-` = stdin), `--dry-run` | write | `jenkins_build_replay` |
| `build stop` | `<job> <ref>` | `--yes` (required to act), `--dry-run` | danger | `jenkins_build_stop` |
| `mcp` | none | global `--profile`, `--tier` | – | – |
| `version` | none | – | – | – |
| `completion` | `bash\|zsh\|fish\|powershell` | – | – | – |

In MCP, `dry_run` replaces `--dry-run`, `confirm: true` replaces `--yes`, and
`ref` may be a string (`"last"`, `"42"`) or an integer. Tools above the
effective tier are not registered.

## Result fields worth knowing

- Durations are `*_ms` (milliseconds); timestamps are RFC3339.
- Logs: `truncated: true` means lines were cut by `max_log_lines`;
  `spilled_to` is a local file path when the result was over 64 KB.
- `log tail`: `total_lines` is the length of the (stage) log. A failed stage
  with `total_lines: 0` has no log of its own.
- `log search`: `scanned_lines` is how many lines were read before the scan
  ended. It is the log length only when `truncated` is false.
- `build stages`: `status` is `SUCCESS`, `FAILED`, `UNSTABLE`, `ABORTED`,
  `IN_PROGRESS`, `PAUSED_PENDING_INPUT` or `NOT_EXECUTED` (skipped, not failed).
- `--jq` prints one JSON document per result; wrap in `[...]` to get one array.
- `test failed`: `has_report: false` means the build has no test report.
- `build trigger --wait`: `timed_out: true` means the build is still running
  (not an error). An error after queuing carries `details.queue_id`.
- `build stop` without `--yes`: exit 7, plan in `error.details`.

## Output shapes

Top-level keys of each command's JSON, checked against `internal/core/types.go`
and `internal/cli/auth.go`. Keys marked `?` are left out when empty.

| command | keys | list items |
|---|---|---|
| `auth status` | `profile`, `profile_source?`, `url`, `user?`, `id`, `full_name`, `token_source`, `user_source?`, `tier`, `config_path?`, `warnings?` | – |
| `job list` | `folder`, `jobs[]`, `total`, `truncated` | `name`, `path`, `kind`, `container`, `color?`, `url` |
| `job get` | `name`, `path`, `url`, `kind`, `class`, `color?`, `buildable`, `last_build?`, `last_successful_build?`, `last_failed_build?`, `last_completed_build?`, `parameters[]?` | `name`, `type`, `description?`, `default?`, `choices?` |
| `build get` | `job`, `number`, `result?`, `building`, `duration_ms`, `estimated_duration_ms?`, `timestamp`, `url`, `causes?`, `parameters[]?`, `revision?`, `changes[]?` | parameters: `name`, `value`; changes: `commit_id`, `message`, `author?` |
| `build stages` | `job`, `number`, `status`, `duration_ms`, `stages[]` | `id`, `name`, `status`, `start_time`, `duration_ms` |
| `log tail` | `job`, `number`, `stage?`, `text`, `lines`, `total_lines`, `truncated`, `spilled_to?` | – |
| `log search` | `job`, `number`, `pattern`, `matches[]`, `scanned_lines`, `truncated`, `spilled_to?` | `line`, `text`, `before?`, `after?` |
| `test failed` | `job`, `number`, `has_report`, `fail_count`, `pass_count`, `skip_count`, `failed[]`, `truncated` | `class_name`, `name`, `status`, `error_details?`, `duration_ms` |
| `queue get` | `id`, `why?`, `blocked`, `cancelled`, `build_number?`, `build_url?`, `task_name?`, `task_url?` | – |
| `build trigger` | `job`, `dry_run`, `plan?`, `queue_id?`, `number?`, `result?`, `building`, `url?`, `timed_out` | – |
| `build replay` | `job`, `from_number`, `dry_run`, `plan?`, `queue_id?` | – |
| `build stop` | `job`, `number`, `dry_run`, `plan?`, `stopped` | – |
| any error (stderr) | `error.code`, `error.message`, `error.hint?`, `error.retryable`, `error.details?` | – |

`plan` (dry run, or `error.details` of NEEDS_CONFIRMATION): `op`, `method`,
`path`, `params?`, `note?`, `number?`.

## Config and environment

- Config file (optional): `$XDG_CONFIG_HOME/qtools/jk.yaml`, else
  `~/.config/qtools/jk.yaml`. Profiles hold `url`, `user` and `policy`
  (`tier`, `allow.jobs`, `deny.jobs`, `max_log_lines`). Unknown keys are a
  USAGE error.
- `JENKINS_URL` and `JENKINS_USER` override the
  profile's `url`/`user`.
- Token: `JENKINS_API_TOKEN`, then the OS keyring.
- Env vars never raise the tier; only the config file sets it.

| tier | allows |
|---|---|
| `read` (default) | auth status, job list/get, build get/stages, log tail/search, test failed, queue get |
| `write` | read + build trigger, build replay |
| `danger` | write + build stop |

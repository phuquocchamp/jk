---
name: jk
description: Read and drive Jenkins with the jk CLI. Use when the user asks why a Jenkins build or pipeline failed, wants a build log, failing stage or failed tests, wants to list jobs or branches, check a queue item, trigger or replay a build, or stop a running build, or pastes a Jenkins job or build URL.
allowed-tools: Bash(jk:*)
---

# jk

`jk` is a Jenkins CLI built for agents. Stdout carries data only, and
errors go to stderr as `{"error":{"code","message","hint","retryable","details"}}`
with a stable exit code. If the jk MCP server is connected, the same
operations are available as `jenkins_*` tools with the same inputs.

## Rules

- **Always pass `--json`.** Output is JSON anyway when stdout is not a
  terminal, but be explicit.
- **Narrow every read.** Use `--fields` or `--jq` to keep only what you need, and
  prefer `log search` or `log tail --stage` over large tails.
- **Read the error's `hint`** before retrying. Retry only when `retryable` is true.
- **Never pass `--yes`** (or `confirm: true` in MCP) without the user's explicit
  approval for that exact build. First run without `--yes`: it returns exit 7
  with the plan in `details`. Show the plan, ask, and then run again with the
  build number from the plan, not `last`.
- **Use `--dry-run` first** for `build trigger` and `build replay` when the
  parameters or script came from you rather than the user.
- **Do not re-trigger** after a trigger error that includes a `queue_id`; follow
  it with `queue get` instead.
- **Never print, echo or log the token.** Do not run `auth login` yourself unless
  the user asks; tell them how to do it.
- If the result has `spilled_to`, the text was too large to inline. Read that
  local file with grep or a ranged read instead of asking for more lines.

## Job paths and refs

- Job path: `folder/sub/job`; multibranch: `project/branch`. A branch name that
  contains `/` must be written with `%2F`: `project/feature%2Flogin`.
- A pasted Jenkins URL works anywhere a job is expected, for example
  `jk build get https://jenkins.example.com/job/app/job/main/42/console`.
  The build number in the URL is used when no ref is given.
- Ref: a build number, or `last` (default), `lastFailed`, `lastSuccessful`,
  `lastCompleted`. MCP accepts the number as `42` or `"42"`.
- `build_trigger`/`build_replay` errors are not retryable when the request may
  already have reached Jenkins; check `job_get`/`build_get` before re-running.

## Why did the build fail?

```sh
jk build get   JOB lastFailed --json --fields number,result,causes,revision,changes
jk build stages JOB 57 --json --jq '.stages[] | select(.status != "SUCCESS") | {id,name,status}'
jk log tail    JOB 57 --stage "Build" -n 200 --json --fields text,truncated
jk log search  JOB 57 -e '(?i)error|exception|failed' -C 3 --max-matches 20 --json
jk test failed JOB 57 --limit 20 --json
```

1. `build get` gives the build number and result; use that number from then on,
   so later commands look at the same build.
2. `build stages` shows which stage failed.
3. Read only that stage's log with `log tail --stage`.
4. If the stage log is not enough, `log search` the full console.
5. If there is a test report, `test failed` lists failed cases with trimmed
   error details (`has_report: false` means there is no report, not an error).

Summarize the failing stage, the first real error, and the failing tests.
Quote short log lines rather than whole logs.

## Other commands

| task | command |
|---|---|
| who am I, which profile and tier | `jk auth status --json` |
| list jobs or branches | `jk job list [folder] --json --fields name,path,kind,color` |
| job parameters and last builds | `jk job get JOB --json` |
| follow a queue item | `jk queue get ID --json` |
| trigger (write tier) | `jk build trigger JOB -p KEY=VALUE --wait --timeout 20m --json` |
| replay with a new Jenkinsfile (write tier) | `jk build replay JOB 57 --script-file Jenkinsfile --json` |
| stop (danger tier, needs approval) | `jk build stop JOB 57 --json`, then `--yes` after approval |

## Exit codes

| exit | code | what to do |
|---|---|---|
| 0 | | success |
| 1 | UPSTREAM / INTERNAL | retry only if `retryable` is true; otherwise report |
| 2 | USAGE | fix the command; see `jk <cmd> --help` |
| 3 | NOT_FOUND | check the job path (`job list`), `%2F` in branch names, or the build number |
| 4 | POLICY_DENIED / FORBIDDEN | the profile's tier or allow/deny rules, or Jenkins permissions, block this; tell the user, do not work around it |
| 5 | CONFLICT | Jenkins state conflict; re-read and report |
| 6 | AUTH_REQUIRED | the user must set `JENKINS_API_TOKEN` or run `jk auth login` |
| 7 | NEEDS_CONFIRMATION | show `details` (the plan) and ask the user before adding `--yes` |

More recipes: [references/recipes.md](references/recipes.md).
Every command, flag and default: [references/commands.md](references/commands.md).

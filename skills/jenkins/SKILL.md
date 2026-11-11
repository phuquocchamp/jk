---
name: jenkins
description: Jenkins — read and drive Jenkins with the jk CLI. Use when the user asks why a Jenkins build or pipeline failed, wants a build log, failing stage or failed tests, wants to list jobs or branches, check a queue item, trigger or replay a build, or stop a running build, or pastes a Jenkins job or build URL.
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
- **Diagnose at `--tier read`.** Investigating a build never needs a write, so
  add `--tier read` to every command in that work. It can only lower the
  profile's tier, so no command can trigger, replay or stop a build by mistake.
- **Narrow every read.** Use `--fields` or `--jq` to keep only what you need, and
  prefer `log tail --stage` over large tails or console-wide searches.
- **Wrap jq filters in `[...]`.** `--jq '.stages[] | ...'` prints one JSON value
  per item; `--jq '[.stages[] | ...]'` prints a single array you can parse.
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

## Reading the output

- **One `jk` command per Bash call.** This skill's permission,
  `Bash(jk:*)`, covers a command that starts with `jk` and nothing else.
  Shell variables (`J=...; jk ...`), `;`, `&&`, pipes and `echo $?` make it a
  different command, which is denied or needs the user's approval. Write the
  job path out in full each time. The exit code and stderr already appear in
  the tool result.
- **Keep the streams apart.** Data is on stdout, errors on stderr. Never add
  `2>&1`: the error JSON then parses as data, and `.number` quietly reads
  `null`.
- **Check the exit code first.** On a non-zero exit, stdout is empty; read
  `error.code`, `error.hint` and `error.details` from stderr.
- **Filter with `--jq`, not `| jq`.** jq (gojq) is built in, so no separate
  `jq` binary is needed. Each result is printed as one JSON document, and
  strings stay quoted (`"FAILURE"`).
- **`--fields` runs before `--jq`.** `--jq` sees only the kept fields; a dropped
  field reads `null`. Unknown names in `--fields` are ignored silently, so a
  typo gives a missing key, not an error.
- **On list results, `--fields` applies to each item** and drops the list's own
  keys unless you name them: `job list X --fields name,path,total,truncated`.
  Without `truncated` you cannot tell that the list was cut at `--limit`.
- **`--fields` drops context too.** `--fields text,truncated` leaves out `job`
  and `number`; keep `number` when you run several builds side by side.
- **Missing is not null.** Optional fields are left out when empty: `result`
  while a build runs, `changes` when there are none, `last_failed_build` when
  nothing failed. Test with `has("x")` or use `.x // default`.
- Result keys per command: [references/commands.md](references/commands.md#output-shapes).

## Job paths and refs

- Job path: `folder/sub/job`; multibranch: `project/branch`. A branch name that
  contains `/` must be written with `%2F`: `project/feature%2Flogin`.
- A pasted Jenkins URL works anywhere a job is expected, for example
  `jk build get https://jenkins.example.com/job/app/job/main/42/console`.
  The build number in the URL is used when no ref is given. URLs that go
  through a view (`/job/app/view/tags/job/1.2.0/`) work too.
- Ref: a build number, or `last` (default), `lastFailed`, `lastSuccessful`,
  `lastCompleted`. MCP accepts the number as `42` or `"42"`.
- `build_trigger`/`build_replay` errors are not retryable when the request may
  already have reached Jenkins; check `job_get`/`build_get` before re-running.

## Why did the build fail?

```sh
jk job get     JOB --tier read --json --fields last_build,last_failed_build,last_successful_build
jk build get   JOB 57 --tier read --json --fields number,result,causes,revision,changes
jk build stages JOB 57 --tier read --json \
  --jq '[.stages[] | select(.status | IN("FAILED","UNSTABLE","ABORTED")) | {id,name,status,start_time}]'
jk log tail    JOB 57 --stage 479 -n 200 --tier read --json --fields text,total_lines,truncated
jk test failed JOB 57 --limit 20 --tier read --json
jk log search  JOB 57 -e 'ERROR|Exception|exit code [1-9]' -C 3 --max-matches 20 --tier read --json
```

1. `job get` shows `last_build` and `last_failed_build`. They are often
   different: the latest build may be green while the user asks about the
   failure. Pick the build number once and use it in every later command.
2. `build get` gives the result, causes and changes for that build.
3. `build stages` lists the stages that failed. `NOT_EXECUTED` means the stage
   was skipped, not that it failed, so the filter leaves it out.
4. Tail the failed stages in `start_time` order, by **id**. A failed stage with
   `total_lines: 0` is a parent or a stage that failed because an earlier one
   did; move to the next. The first stage with log text usually holds the error.
5. If there is a test report, `test failed` lists failed cases with trimmed
   error details (`has_report: false` means there is no report, not an error).
6. Use `log search` on the whole console only when the stage logs don't explain
   the failure. Console-wide matches are noisy (test names, agent reconnects,
   tool warnings). `truncated: true` means the scan stopped at `--max-matches`
   and the error may be further down; `scanned_lines` is how far it read, not
   the log length.

Summarize the failing stage, the first real error, and the failing tests.
Quote short log lines rather than whole logs.

## Other commands

| task | command |
|---|---|
| who am I, which profile and tier | `jk auth status --json` |
| list jobs or branches | `jk job list [folder] --json --fields name,path,kind,color,total,truncated` |
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

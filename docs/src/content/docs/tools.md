---
title: "Tools"
---

jk has 15 operations. Each one is a CLI command and an MCP tool with the same
inputs. MCP tool names are `jenkins_` + the operation name; in Claude Code they
show up as `mcp__jk__jenkins_*`.

**12 read · 1 write · 2 danger.** See [Policy](/jk/policy/) for what each tier
allows, and the [CLI reference](/jk/reference/jk/) for every flag.

## By area

| Area | CLI | MCP tool | Tier | What it returns |
|---|---|---|---|---|
| **Auth** | `auth status` | `jenkins_whoami` | read | user id and name, profile, tier, token source |
| **Jobs** | `job list [folder] [--limit 50]` | `jenkins_job_list` | read | direct children of a folder or multibranch project |
| | `job get <job>` | `jenkins_job_get` | read | kind, last build numbers, parameter definitions |
| **Builds** | `build diagnose <job> [ref] [-n 60] [--max-tests 10]` | `jenkins_build_diagnose` | read | in one call: result, first failing stage and step (command, error), the end of that step's log, failed tests, changes; ref defaults to `lastFailed` |
| | `build get <job> [ref]` | `jenkins_build_get` | read | result, duration, causes, parameters, revision, changes |
| | `build stages <job> [ref]` | `jenkins_build_stages` | read | stages (nested, `parent_id`) with status, error and failing step; `first_failed`; `inherited` marks parents that only failed because a nested stage did; `source` is `pipeline-graph-view` or `wfapi` (then stages skipped after the failure are inferred: `SKIPPED` + `skipped_after_failure`) |
| | `build script <job> [ref]` | `jenkins_build_replay_script` | read | the Jenkinsfile a build ran (from its replay form) |
| | `build wait <job> [ref] [--queue-id N] [--timeout 10m]` | `jenkins_build_wait` | read | waits for a queued or running build; `timed_out` + `next` when time runs out |
| | `build trigger <job> [-p k=v ...] [--allow-unknown] [--wait [--timeout 10m]]` | `jenkins_build_trigger` | write | queue id; build number and result with `--wait`. Unknown parameter names and invalid choices are rejected. MCP waits at most 45s, then returns `next` (`build_wait`) |
| | `build replay <job> <ref> --script-file F [--yes]` | `jenkins_build_replay` | danger | re-runs a Pipeline build with a new Jenkinsfile (`-` = stdin); needs `--yes`, the plan has a diff against the original |
| | `build stop <job> <ref> [--yes]` | `jenkins_build_stop` | danger | aborts a running build |
| **Logs** | `log tail <job> [ref] [-n N] [--stage NAME] [--step failed\|ID] [--raw]` | `jenkins_build_log` | read | last N lines of the console (read from an offset near the end), of one stage (step logs read from the last step back until there are enough lines) or of the failing step only (`--step failed`); ANSI codes and timestamps stripped unless `--raw` |
| | `log search <job> [ref] -e RE [-C N] [--max-matches 50]` | `jenkins_log_search` | read | regex matches in the streamed console, with context |
| **Tests** | `test failed <job> [ref] [--limit 50]` | `jenkins_test_failed` | read | failed and regressed JUnit cases |
| **Queue** | `queue get <id>` | `jenkins_queue_get` | read | why a queue item waits, or its build number |

Commands outside the catalog: `auth login`, `auth logout`, `mcp`, `setup` (`setup skill`, `setup mcp`), `version`,
`completion`.

MCP tools accept `ref` as a string (`"42"`, `"last"`) or an integer (`42`).

## Typical flows

**Why did the build fail?**

```sh
jk build diagnose team-a/api/main          # one call: stage, step, error, log, highlights, tests
```

When the excerpt is not enough:

```sh
jk build stages team-a/api/main lastFailed                  # first_failed, failed_step, inherited
jk log tail team-a/api/main lastFailed --step failed -n 500  # the whole failing step
jk log tail team-a/api/main lastFailed --stage Test -n 200   # the whole stage
jk test failed team-a/api/main lastFailed
```

**Fix a Jenkinsfile and re-run it (danger tier)**

```sh
jk build script team-a/api/main 57 > Jenkinsfile    # the script that build ran
# edit Jenkinsfile
jk build replay team-a/api/main 57 --script-file Jenkinsfile --dry-run   # plan with a diff
jk build replay team-a/api/main 57 --script-file Jenkinsfile --yes
jk build wait team-a/api/main 58
```

**Run a deploy and wait for it**

```sh
jk job get team-a/deploy --fields parameters          # what it expects
jk build trigger team-a/deploy -p ENV=staging --dry-run
jk build trigger team-a/deploy -p ENV=staging --wait --timeout 20m
```

## Not supported

jk covers reading builds and a small set of build actions. These Jenkins
features have no command or tool today:

| Area | Not supported |
|---|---|
| Build history | Listing a job's builds. Builds are addressed one at a time by number or `last*` ref. |
| Artifacts | Listing or downloading archived artifacts. |
| Pipeline input | Approving or aborting an `input` step. |
| Queue | Cancelling a queue item (only `queue get`). |
| Job management | Creating, copying, deleting, enabling, disabling or configuring jobs (`config.xml`), and scanning multibranch projects. |
| Controller admin | Agents/nodes and executors, credentials, plugins, users, views, system configuration. |
| Trigger | File parameters. Parameters are sent as form values only. |
| Replay | Replacing scripts loaded with `load` or from shared libraries; only the main Jenkinsfile (`mainScript`) is replaced. Pull request builds from untrusted sources have no replay form. |
| Stages | Freestyle jobs: stages and stage logs need a Pipeline build (Pipeline Graph View or `pipeline-rest-api`). |
| Tests | Passed cases and test history. Only failed and regressed JUnit cases are returned. |
| MCP transport | HTTP / SSE. `jk mcp` serves over stdio only. |

## Limitations

- **Replay** needs the Replay permission and a build that still exists. Builds
  that load other scripts usually make Jenkins answer with a server error,
  which jk reports as not retryable.
- **Stage logs** are the full logs of the stage's steps (`progressiveText`),
  read from the last step backwards until there are enough lines. When not
  every step was read, `total_lines` is left out and `steps_read` <
  `steps_total`.
- **Stages without Pipeline Graph View** come from `wfapi`, which reports stages
  skipped after a failure as `FAILED`. jk infers them (`SKIPPED` plus
  `skipped_after_failure`) when no step of the stage failed.
- **Branch names with `/`** must be given in the `%2F` form;
  `proj/feature/foo` is read as three path levels.
- **`log search`** uses Go RE2 syntax (no backreferences or lookaround).

# Tools

jk has 12 operations. Each one is a CLI command and an MCP tool with the same
inputs. MCP tool names are `jenkins_` + the operation name; in Claude Code they
show up as `mcp__jk__jenkins_*`.

**9 read · 2 write · 1 danger.** See [Policy](policy.md) for what each tier
allows, and the [CLI reference](reference/jk.md) for every flag.

## By area

| Area | CLI | MCP tool | Tier | What it returns |
|---|---|---|---|---|
| **Auth** | `auth status` | `jenkins_whoami` | read | user id and name, profile, tier, token source |
| **Jobs** | `job list [folder] [--limit 50]` | `jenkins_job_list` | read | direct children of a folder or multibranch project |
| | `job get <job>` | `jenkins_job_get` | read | kind, last build numbers, parameter definitions |
| **Builds** | `build get <job> [ref]` | `jenkins_build_get` | read | result, duration, causes, parameters, revision, changes |
| | `build stages <job> [ref]` | `jenkins_build_stages` | read | pipeline stages with status and duration |
| | `build trigger <job> [-p k=v ...] [--wait [--timeout 10m]]` | `jenkins_build_trigger` | write | queue id; build number and result with `--wait` |
| | `build replay <job> <ref> --script-file F` | `jenkins_build_replay` | write | re-runs a Pipeline build with a new Jenkinsfile (`-` = stdin) |
| | `build stop <job> <ref> [--yes]` | `jenkins_build_stop` | danger | aborts a running build |
| **Logs** | `log tail <job> [ref] [-n N] [--stage NAME]` | `jenkins_build_log` | read | last N lines of the console or of one stage |
| | `log search <job> [ref] -e RE [-C N] [--max-matches 50]` | `jenkins_log_search` | read | regex matches in the streamed console, with context |
| **Tests** | `test failed <job> [ref] [--limit 50]` | `jenkins_test_failed` | read | failed and regressed JUnit cases |
| **Queue** | `queue get <id>` | `jenkins_queue_get` | read | why a queue item waits, or its build number |

Commands outside the catalog: `auth login`, `auth logout`, `mcp`, `version`,
`completion`.

MCP tools accept `ref` as a string (`"42"`, `"last"`) or an integer (`42`).

## Typical flows

**Why did the build fail?**

```sh
jk build get team-a/api/main lastFailed --fields number,result,causes
jk build stages team-a/api/main lastFailed
jk log tail team-a/api/main lastFailed --stage Test -n 200
jk test failed team-a/api/main lastFailed
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
| Replay | Anything beyond the main Jenkinsfile (`mainScript`): scripts loaded with `load` or from shared libraries. |
| Stages | Freestyle jobs: stages and stage logs need a Pipeline build and `pipeline-rest-api`. |
| Tests | Passed cases and test history. Only failed and regressed JUnit cases are returned. |
| MCP transport | HTTP / SSE. `jk mcp` serves over stdio only. |

## Limitations

- **Replay** needs the Replay permission and a build that still exists. Builds
  that load other scripts usually make Jenkins answer with a server error,
  which jk reports as not retryable.
- **Stage logs** come from `pipeline-rest-api` (`wfapi`), which may truncate
  long stage logs; then `truncated` is true. Use `log search` on the full
  console.
- **Branch names with `/`** must be given in the `%2F` form;
  `proj/feature/foo` is read as three path levels.
- **`log search`** uses Go RE2 syntax (no backreferences or lookaround).

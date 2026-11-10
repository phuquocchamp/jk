# jk — Jenkins from the shell and from agents

`jk` is an agent-first Jenkins CLI and MCP server in one Go binary.

- `jk ...` is a CLI whose stdout carries only data (JSON when piped) and
  whose errors have stable codes and exit codes.
- `jk mcp` serves the same 12 operations as MCP tools over stdio.
- A policy in the config file decides what an agent may do: read only (the
  default), trigger and replay builds, or stop builds. Flags can only lower it.

It talks to Jenkins over its REST API with a user API token. No Jenkins plugin
is needed beyond the usual Pipeline ones (`pipeline-rest-api` for stages and
stage logs, `junit` for test reports).

## Install

```sh
go install github.com/phuquocchamp/jk/cmd/jk@latest
```

Or download a release archive for macOS, Linux or Windows (amd64 and arm64)
from the GitHub releases page and put `jk` on your `PATH`. Binaries are
static (`CGO_ENABLED=0`).

From source:

```sh
make build          # writes bin/jk
bin/jk version
```

## Authenticate

Create an API token in Jenkins (your user, then **Security**, then **API Token**).

**Environment variables** (simplest, and what CI uses):

```sh
export JENKINS_URL=https://jenkins.example.com
export JENKINS_USER=jane
export JENKINS_API_TOKEN=...      # never commit this
jk auth status
```

Inside a Jenkins build, Jenkins itself sets `JENKINS_URL` to the controller running
the job, and it overrides the config profile's `url`. Unset it (or export the
URL you want) if the tool should talk to a different controller.

**OS keyring** (macOS Keychain, Windows Credential Manager, Secret Service on
Linux):

```sh
printf %s "$TOKEN" | jk auth login --with-token   # or just: jk auth login (hidden prompt, TTY only)
jk auth status --json
jk auth logout
```

The token is stored under service `jk`, account `<profile>`. Lookup order:
`JENKINS_API_TOKEN`, then the keyring. `auth status`
shows where the token came from (`env:JENKINS_API_TOKEN` or `keyring`), never
the token itself.

## Configure

The config file is optional: `$XDG_CONFIG_HOME/qtools/jk.yaml`, or
`~/.config/qtools/jk.yaml` when `XDG_CONFIG_HOME` is not set.

```yaml
profiles:
  default:
    url: https://jenkins.example.com
    user: jane
    policy:
      tier: write                       # read | write | danger (default read)
      allow: { jobs: ["team-a/**"] }    # empty = all jobs
      deny:  { jobs: ["prod/**"] }      # deny wins over allow
      max_log_lines: 2000               # hard cap for log tail/search output
  other:
    url: https://ci.example.org
    user: jane
default_profile: default
```

- The profile is `--profile`, then `default_profile`, then `default`.
- `JENKINS_URL` / `JENKINS_USER` override the
  profile's `url` and `user`.
- The file is parsed strictly: an unknown key (for example a typo like `dney:`)
  is an error instead of a silently ignored rule.
- jk warns on stderr if the file is readable by group or others
  (`chmod 600` it). Do not put tokens in it.

### Policy tiers

| tier | allows |
|---|---|
| `read` (default) | `whoami`, `job_list`, `job_get`, `build_get`, `build_stages`, `build_log`, `log_search`, `test_failed`, `queue_get` |
| `write` | read, plus `build_trigger` and `build_replay` |
| `danger` | write, plus `build_stop` |

- **Tighten-only:** `--tier` (CLI and `jk mcp --tier`) can only lower the
  tier set in the config. Asking for more is `POLICY_DENIED`. The
  `JENKINS_*` variables cannot raise it either.
- **The tier is not a sandbox.** An agent that can run shell commands can point
  `XDG_CONFIG_HOME` at a config file it wrote, or pick a more permissive
  `--profile`. The real boundary is the Jenkins user's permissions, or an MCP
  server started by you with a fixed `--tier` (it logs its profile, config path
  and tier to stderr at startup).
- `allow.jobs` / `deny.jobs` are [doublestar](https://github.com/bmatcuk/doublestar)
  globs on the job path with `/` separators. They apply to every job operation.
  `team-a/**` matches `team-a` and everything below it. Matching is
  case-insensitive, like Jenkins' own job lookup, so `deny: ["prod/**"]` also
  blocks `PROD/app`.
- `job list` also shows folders on the way to an allowed job: with
  `allow: ["team-a/app/**"]`, the root listing shows `team-a` and
  `job list team-a` shows `team-a/app`. Other operations on `team-a` itself
  stay denied.
- Write operations accept `--dry-run` (MCP: `dry_run`), which returns the
  request plan without sending it.
- `build stop` needs `--yes` (MCP: `confirm: true`). Without it, it exits 7
  (`NEEDS_CONFIRMATION`) with the plan in `error.details`, and the hint names the
  exact build number to confirm. It prompts only when stdin and stdout are both
  terminals.
- In MCP, tools above the effective tier are not registered at all.

## Commands

Job arguments are a path like `folder/sub/job`, a multibranch branch like
`project/branch`, or a pasted Jenkins URL. A branch name containing `/` is
written with `%2F`: `project/feature%2Flogin`. `[ref]` is a build number or
`last` (default), `lastFailed`, `lastSuccessful`, `lastCompleted`; a build
number in a pasted URL is used when no ref is given. MCP tools accept `ref` as
a string (`"42"`, `"last"`) or an integer (`42`).

| command | tier | MCP tool | what it does |
|---|---|---|---|
| `auth status` | read | `jenkins_whoami` | user id and name, profile, tier, token source |
| `job list [folder] [--limit 50]` | read | `jenkins_job_list` | direct children of a folder or multibranch project |
| `job get <job>` | read | `jenkins_job_get` | kind, last build numbers, parameter definitions |
| `build get <job> [ref]` | read | `jenkins_build_get` | result, duration, causes, parameters, revision, changes |
| `build stages <job> [ref]` | read | `jenkins_build_stages` | pipeline stages with status and duration |
| `log tail <job> [ref] [-n N] [--stage NAME]` | read | `jenkins_build_log` | last N lines of the console or of one stage |
| `log search <job> [ref] -e RE [-C N] [--max-matches 50]` | read | `jenkins_log_search` | regex search in the streamed console, with context |
| `test failed <job> [ref] [--limit 50]` | read | `jenkins_test_failed` | failed and regressed JUnit cases |
| `queue get <id>` | read | `jenkins_queue_get` | why a queue item waits, or its build number |
| `build trigger <job> [-p k=v ...] [--wait [--timeout 10m]]` | write | `jenkins_build_trigger` | start a build, optionally wait for the result |
| `build replay <job> <ref> --script-file F` | write | `jenkins_build_replay` | re-run a Pipeline build with a new Jenkinsfile (`-` = stdin) |
| `build stop <job> <ref> [--yes]` | danger | `jenkins_build_stop` | abort a running build |

Other commands: `auth login`, `auth logout`, `mcp`, `version`, `completion`.

Global flags:

| flag | meaning |
|---|---|
| `--profile NAME` | config profile |
| `--json` | JSON output (automatic when stdout is not a terminal) |
| `--fields a,b` | keep only these keys (per item for lists) |
| `--jq EXPR` | filter the JSON result with jq ([gojq](https://github.com/itchyny/gojq)) |
| `-y, --yes` | confirm a danger operation |
| `--dry-run` | show the plan of a write operation without running it |
| `--tier read\|write` | lower the effective tier for this call |

Examples:

```sh
jk build get team-a/api/main lastFailed --json --fields number,result,causes
jk build stages 'https://jenkins.example.com/job/team-a/job/api/job/main/57/' --json
jk log search team-a/api/main 57 -e '(?i)error' -C 3 --json
jk build trigger team-a/deploy -p ENV=staging --wait --timeout 20m --json
```

Output notes:

- Durations are milliseconds (`*_ms`) and timestamps RFC3339 in JSON; human
  output shows them readably.
- Log output is capped by `max_log_lines`; `truncated: true` says lines were
  left out. When a log result is larger than 64KB, it is written to a file under
  the system temp dir (`$TMPDIR/jk/`, or a private `$TMPDIR/jk-*`
  dir when that one is not owned by you with mode 0700), and `spilled_to` holds
  its path.
- `test failed` returns `has_report: false` when the build has no test report.
- `build trigger --wait` returns `timed_out: true` (not an error) when the
  timeout passes; the build keeps running. If an error happens after the build
  was queued, the error's `details` contain the `queue_id`: do not trigger again.

## Use with Claude Code (MCP)

```sh
claude mcp add jk -- jk mcp
# read-only even if the config allows more:
claude mcp add jk -- jk mcp --tier read
```

Or in a project's `.mcp.json`:

```json
{
  "mcpServers": {
    "jk": {
      "command": "jk",
      "args": ["mcp"],
      "env": { "JENKINS_URL": "https://jenkins.example.com", "JENKINS_USER": "jane" }
    }
  }
}
```

Keep the token in the keyring (`jk auth login`) or in your shell
environment, not in `.mcp.json`. The server needs a URL and a token at start;
otherwise it exits with the error on stderr.

The tools show up in Claude Code as `mcp__jk__jenkins_*` (for example
`mcp__jk__jenkins_build_log`). To pre-approve the read-only ones, add rules
such as `"mcp__jk__jenkins_build_log"` to `permissions.allow` in
`settings.json`, or `"mcp__jk__*"` to allow every tool the tier permits.

Desktop apps (Claude Desktop, Cursor) do not read your shell's `PATH`; use the
absolute path as `command`, e.g. `"command": "/Users/jane/go/bin/jk"`
(`which jk` prints it).

There is also a Claude Code skill for the CLI in
[`skills/jk/`](skills/jk/SKILL.md). Copy it to `~/.claude/skills/`
or a project's `.claude/skills/`.

## Shell completion

```sh
source <(jk completion zsh)    # or bash / fish / powershell
```

## Exit codes

Errors go to stderr. In JSON mode they look like
`{"error":{"code":"NOT_FOUND","message":"...","hint":"...","retryable":false,"details":...}}`.

| exit | code | meaning |
|---|---|---|
| 0 | | success |
| 1 | `UPSTREAM` | Jenkins error or timeout; `retryable` is true for 5xx and timeouts, except for `build trigger`/`build replay` requests that may already have reached Jenkins (check before re-running) |
| 1 | `INTERNAL` | bug in jk |
| 2 | `USAGE` | bad command, flag, argument or config |
| 3 | `NOT_FOUND` | job, build or queue item does not exist |
| 4 | `POLICY_DENIED` | blocked by the profile's tier or allow/deny globs |
| 4 | `FORBIDDEN` | Jenkins returned 403 |
| 5 | `CONFLICT` | Jenkins returned 409 |
| 6 | `AUTH_REQUIRED` | no token, or Jenkins returned 401 |
| 7 | `NEEDS_CONFIRMATION` | danger operation without `--yes` / `confirm` |

## Security notes

- The token is never printed or logged, including in `auth status`, errors and
  debug logs. Requests are logged (at most) as method, path and status on stderr.
- Use a token for a Jenkins user with only the permissions the agent needs.
  The jk policy is a second line of defence, not a replacement for Jenkins
  permissions.
- Default tier is `read`. Raising it takes an edit of the config file (or a
  different config/profile); `--tier` and MCP inputs cannot raise it. See
  "The tier is not a sandbox" above.
- Keep the config file private (`chmod 600`); jk warns otherwise.
- POSTs use the API token with basic auth. If Jenkins answers 403, jk
  fetches a CSRF crumb once and retries.

## Limitations

- **Replay** sends only the main Jenkinsfile (`mainScript`). Builds that also
  load scripts with `load` or from shared libraries in the replay form are not
  supported; Jenkins usually answers with a server error, which jk reports
  as not retryable. Replay needs the Replay permission and a build that still
  exists.
- Stage logs come from `pipeline-rest-api` (`wfapi`), which may truncate long
  stage logs; then `truncated` is true. Use `log search` on the full console.
- A branch name with `/` must be given in the `%2F` form; `proj/feature/foo` is
  read as three path levels.
- `log search` uses Go RE2 syntax (no backreferences or lookaround).

## Development

```sh
make lint-lite   # gofmt check, go vet, go mod tidy -diff, kit import rule
make test        # go test ./...  (tests use httptest; they never call a real Jenkins)
make cross       # CGO_ENABLED=0 builds for linux, darwin, windows
make build       # bin/jk
```

Design and package contracts: [docs/design.md](docs/design.md).

## License

MIT, see [LICENSE](LICENSE).

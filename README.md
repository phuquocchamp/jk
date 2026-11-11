<p align="center">
  <a href="https://phuquocchamp.github.io/jk/">
    <img src="docs/src/assets/banner.svg" alt="jk: an agent-first Jenkins CLI and MCP server" width="100%">
  </a>
</p>

<p align="center">
  Read and drive Jenkins® from your terminal and from your coding agent.
</p>

<p align="center">
  <a href="https://github.com/phuquocchamp/jk/releases"><img alt="Release" src="https://img.shields.io/github/v/release/phuquocchamp/jk?sort=semver&style=flat-square&color=335061"></a>
  <a href="https://github.com/phuquocchamp/jk/actions/workflows/ci.yml"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/phuquocchamp/jk/ci.yml?branch=main&label=ci&style=flat-square&color=335061"></a>
  <a href="https://phuquocchamp.github.io/jk/"><img alt="Docs" src="https://img.shields.io/badge/docs-starlight-335061?style=flat-square"></a>
  <a href="go.mod"><img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/phuquocchamp/jk?style=flat-square&color=335061"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-335061?style=flat-square"></a>
</p>

---

jk is a Jenkins CLI and MCP server in one Go binary. The CLI prints only data
on stdout (JSON when piped) and fails with stable error codes. `jk mcp` serves
the same 12 operations as MCP tools. A policy in your config decides what an
agent may do: read only (the default), trigger and replay builds, or stop them.

## Installation

```sh
# Go
go install github.com/phuquocchamp/jk/cmd/jk@latest

# Prebuilt binaries (macOS, Linux, Windows; amd64 and arm64)
# Download from GitHub Releases and verify against checksums.txt.

# From source
make build                 # writes bin/jk
```

Binaries are static (`CGO_ENABLED=0`). Archives are on the
[releases page](https://github.com/phuquocchamp/jk/releases).

Shell completion:

```sh
source <(jk completion zsh)   # or bash, fish, powershell
```

## Connect To Jenkins

Create an API token in Jenkins (your user → **Security** → **API Token** →
**Add new token**), then log in:

```sh
jk auth login          # asks for the URL, the user and the token
jk auth status         # token_source: keyring
```

`jk auth login` asks Jenkins who the token belongs to before saving anything,
so a mistyped token or URL is caught right there. When the check passes, jk:

- stores the token in macOS Keychain, Windows Credential Manager or the Linux
  Secret Service (service `jk`, account = profile name), and
- creates `~/.config/qtools/jk.yaml` (mode `0600`) with the URL, the user and
  policy tier **read**. The file holds no secret, and login never changes a
  profile that already exists.

In scripts: `printf %s "$TOKEN" | jk auth login --url https://jenkins.example.com --user jane --with-token`.

Working with several controllers? Pin a profile per repository with
`git config jk.profile legacy`; every jk command (and `jk mcp`) run in that
repository then uses it.

In CI, or where there is no keyring, use environment variables instead. An
exported `JENKINS_*` value always wins over the config file and the keyring:

```sh
export JENKINS_URL=https://jenkins.example.com
export JENKINS_USER=jane
export JENKINS_API_TOKEN=...   # never commit
```

Keep the user and the token in the same place. If the token is exported but
the user comes from `jk.yaml` (or the other way round), `jk auth status` warns
that the token may belong to another Jenkins user.

More: [authentication](https://phuquocchamp.github.io/jk/authentication/),
[configuration and profiles](https://phuquocchamp.github.io/jk/configuration/).

## Debug A Failed Build

Go from "it's red" to the failing stage, log lines and tests:

```sh
jk build get    team-a/api/main lastFailed         # what happened
jk build stages team-a/api/main lastFailed         # which stage failed
jk log tail     team-a/api/main lastFailed --stage Test -n 200
jk log search   team-a/api/main 57 -e '(?i)error' -C 3
jk test failed  team-a/api/main lastFailed
```

A `<job>` is a path (`folder/sub/job`), a multibranch branch
(`project/branch`, with `/` in branch names written as `%2F`), or a pasted
Jenkins URL. A `[ref]` is a build number or `last` (default), `lastFailed`,
`lastSuccessful`, `lastCompleted`.

## Run Builds

Write and danger operations need a higher policy tier (see [Policy](#policy)):

```sh
jk build trigger team-a/api/main -p ENV=staging --wait
jk build replay  team-a/api/main 57 --script-file Jenkinsfile
jk build stop    team-a/api/main 58 --yes
jk build trigger team-a/api/main --dry-run         # show the plan, do nothing
```

## Use From An Agent

Register jk as an MCP server in Claude Code:

```sh
claude mcp add jk -- jk mcp
claude mcp add jk -- jk mcp --tier read   # read-only even if the config allows more
```

`jk mcp` resolves the token the same way the CLI does, so the keyring works
here too and the token never has to sit in `.mcp.json`. Don't add it with
`claude mcp add -e JENKINS_API_TOKEN=...`: Claude Code would save it in plain
text in its config. Tools appear as
`mcp__jk__jenkins_*`. Desktop apps need the absolute path to `jk` as
`command`. More: [MCP server](https://phuquocchamp.github.io/jk/mcp/).

For agents that run shell commands, a Claude Code skill for the CLI is in
[`skills/jenkins/`](skills/jenkins/SKILL.md). Copy that folder to
`~/.claude/skills/jenkins/`.

## Tools

12 operations, each a CLI command and an MCP tool (`jenkins_` + name):
**9 read · 2 write · 1 danger**.

| Area | CLI | MCP tool | Tier |
|---|---|---|---|
| **Auth** | `auth status` | `jenkins_whoami` | read |
| **Jobs** | `job list [folder]` · `job get <job>` | `jenkins_job_list` · `jenkins_job_get` | read |
| **Builds** | `build get <job> [ref]` · `build stages <job> [ref]` | `jenkins_build_get` · `jenkins_build_stages` | read |
| | `build trigger <job> [-p k=v ...] [--wait]` · `build replay <job> <ref> --script-file F` | `jenkins_build_trigger` · `jenkins_build_replay` | write |
| | `build stop <job> <ref> [--yes]` | `jenkins_build_stop` | danger |
| **Logs** | `log tail <job> [ref] [-n N] [--stage NAME]` · `log search <job> [ref] -e RE` | `jenkins_build_log` · `jenkins_log_search` | read |
| **Tests** | `test failed <job> [ref]` | `jenkins_test_failed` | read |
| **Queue** | `queue get <id>` | `jenkins_queue_get` | read |

Global flags: `--json`, `--fields a,b`, `--jq EXPR`, `--profile NAME`,
`--tier`, `--dry-run`, `-y/--yes`.

jk has **no** command for build history, artifacts, Pipeline `input`
approval, cancelling queue items, job/agent/credential/plugin configuration,
file parameters, or MCP over HTTP (stdio only). Replay sends the main
Jenkinsfile only. See [Tools → Not supported](https://phuquocchamp.github.io/jk/tools/#not-supported).

## Policy

```yaml
# ~/.config/qtools/jk.yaml
profiles:
  default:
    url: https://jenkins.example.com
    user: jane
    policy:
      tier: write                       # read | write | danger (default read)
      allow: { jobs: ["team-a/**"] }    # empty = all jobs
      deny:  { jobs: ["prod/**"] }      # deny wins over allow
```

- `read` (default) allows the 9 read tools; `write` adds trigger and replay;
  `danger` adds stop.
- `--tier` can only **lower** the configured tier. In MCP, tools above the
  tier are not registered at all.
- `build stop` needs `--yes` (MCP: `confirm: true`); without it jk exits 7
  with the plan. Write operations accept `--dry-run`.
- **The tier is not a sandbox.** The real boundary is the Jenkins user's
  permissions. See [Policy](https://phuquocchamp.github.io/jk/policy/).

## Output And Exit Codes

Stdout is data only. Errors go to stderr as
`{"error":{"code","message","hint","retryable","details"}}`.

| exit | code |
|---|---|
| 0 | success |
| 1 | `UPSTREAM`, `INTERNAL` |
| 2 | `USAGE` |
| 3 | `NOT_FOUND` |
| 4 | `POLICY_DENIED`, `FORBIDDEN` |
| 5 | `CONFLICT` |
| 6 | `AUTH_REQUIRED` |
| 7 | `NEEDS_CONFIRMATION` |

Meanings, retry rules, log truncation and spill files:
[Output and errors](https://phuquocchamp.github.io/jk/errors/).

## Reference

Use `--help` on any command to explore flags and examples:

```sh
jk build --help
jk log search --help
```

- [CLI reference](https://phuquocchamp.github.io/jk/reference/jk/)
- [Getting started](https://phuquocchamp.github.io/jk/getting-started/)
- [Tools](https://phuquocchamp.github.io/jk/tools/)
- [jk docs](https://phuquocchamp.github.io/jk/)

## Security

- The token is never printed or logged. Requests are logged (at most) as
  method, path and status on stderr.
- Use a token for a Jenkins user with only the permissions the agent needs.
- Keep the config file private (`chmod 600`); jk warns otherwise.

## Developing

```sh
make lint-lite   # gofmt check, go vet, go mod tidy -diff, kit import rule
make test        # go test ./...  (httptest only; never calls a real Jenkins)
make build       # bin/jk
make docs-serve  # regenerate the CLI reference and preview the docs site
```

| Path | Purpose |
|---|---|
| `cmd/jk` | Binary entry point |
| `internal/cli` | Cobra commands |
| `internal/core` | The 12 operations, shared by CLI and MCP |
| `internal/client` | Jenkins HTTP client |
| `internal/mcpserver` | `jk mcp` tool registration |
| `internal/kit` | Shared building blocks: auth (keyring), config, policy, errors, output |
| `skills/jenkins` | Claude Code skill for the CLI |
| `docs` | MkDocs site |

Releasing: [docs/release.md](docs/release.md).

## Contributing

Focused pull requests with a clear problem and tests that match the
user-facing behaviour are welcome. Run `make lint-lite` and `make test` before
opening one. PR titles use conventional commits, for example:

```text
fix(log): keep stage filter when the log is truncated
```

## License

jk is released under the MIT license. See [LICENSE](LICENSE).

<sub>Jenkins® is a registered trademark of LF Charities Inc. jk is an
independent project, not affiliated with or endorsed by the Jenkins project.</sub>

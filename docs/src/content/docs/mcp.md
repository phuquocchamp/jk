---
title: "MCP server"
---

`jk mcp` serves the [tools](/jk/tools/) over stdio. Only tools allowed by the
effective [tier](/jk/policy/) are registered.

## Claude Code

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

`jk mcp` finds the token the same way the CLI does: `JENKINS_API_TOKEN`
first, then the keyring. So after `jk auth login` the MCP config needs no
token at all.

:::caution[Don't pass the token with `-e`]
Other Jenkins MCP servers are set up with
`claude mcp add ... -e JENKINS_API_TOKEN=...`. That works with jk too, since the
environment wins over the keyring, but Claude Code then saves the token in
plain text in its config file (`~/.claude.json` or `.mcp.json`). Use
`jk auth login` instead and leave the token out of the MCP config.
:::

To use a different profile per project, pin it in the repository with
`git config jk.profile <name>` (see
[Per-repository profile](/jk/configuration/#per-repository-profile)). Claude
Code starts the server in the project directory, so `jk mcp` picks it up;
`--profile` in the MCP command still wins.

The tools show up as `mcp__jk__jenkins_*` (for example
`mcp__jk__jenkins_build_log`). To pre-approve them, add rules such as
`"mcp__jk__jenkins_build_log"` to `permissions.allow` in `settings.json`, or
`"mcp__jk__*"` to allow every tool the tier permits.

## Desktop apps

Claude Desktop, Cursor and similar apps do not read your shell's `PATH`. Use
the absolute path as `command`, for example `"command": "/Users/jane/go/bin/jk"`
(`which jk` prints it).

## Startup

- The server needs a URL and a token at start. Otherwise it exits (code 2 or
  6) with the error on stderr.
- It logs its profile, how the profile was picked, config path, tier and
  where the token and user came from to stderr, for example
  `jk mcp: profile=default profile_source=default config=/home/jane/.config/qtools/jk.yaml tier=read token_source=keyring user_source=file`.
  The token itself is never logged.
- When the token and the user come from different places (one from the
  environment, the other from the config file or keyring), it also logs a
  `jk mcp: warning:` line. See [Authentication](/jk/authentication/#check-which-token-is-used).
- Stdout carries only the MCP protocol.

## Claude Code skill

For agents that run shell commands instead of MCP, there is a skill for the
CLI in [`skills/jenkins/`](https://github.com/phuquocchamp/jk/tree/main/skills/jenkins).
Copy it to `~/.claude/skills/` or a project's `.claude/skills/`. Release
archives include it too.

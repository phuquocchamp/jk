# MCP server

`jk mcp` serves the [tools](tools.md) over stdio. Only tools allowed by the
effective [tier](policy.md) are registered.

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

Keep the token in the keyring (`jk auth login`) or in your shell environment,
not in `.mcp.json`.

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
- It logs its profile, config path and tier to stderr, for example
  `jk mcp: profile=default config=/home/jane/.config/qtools/jk.yaml tier=read`.
- Stdout carries only the MCP protocol.

## Claude Code skill

For agents that run shell commands instead of MCP, there is a skill for the
CLI in [`skills/jenkins/`](https://github.com/phuquocchamp/jk/tree/main/skills/jenkins).
Copy it to `~/.claude/skills/` or a project's `.claude/skills/`. Release
archives include it too.

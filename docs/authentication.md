# Authentication

jk signs in to Jenkins with a user's **API token** over HTTP basic auth. Where
the URL and user come from, and the setups for your machine, CI and MCP, are on
[Configuration](configuration.md). This page is about the token.

## Create a token

In Jenkins: your user, then **Security**, then **API Token**, then
**Add new token**. Copy it right away; Jenkins shows it only once.

Use a Jenkins user with only the permissions the agent needs. The jk
[policy](policy.md) is a second line of defence, not a replacement for Jenkins
permissions.

## Give it to jk

There are two ways. Use one of them.

| | OS keyring | `JENKINS_API_TOKEN` |
|---|---|---|
| How | `jk auth login` | `export JENKINS_API_TOKEN=...` |
| Stored | encrypted by the OS (macOS Keychain, Windows Credential Manager, Linux Secret Service) | plain text in your shell rc, `.env` file or CI secret |
| Readable by | jk, when it asks the keyring | every process started from that shell |
| Best for | your own machine, MCP servers | CI, containers, one-off runs |

!!! note "Exporting the token does not use the keyring"
    The two are separate. When `JENKINS_API_TOKEN` is set, jk uses it and
    never reads or writes the keyring. To move an exported token into the
    keyring:

    ```sh
    printf %s "$JENKINS_API_TOKEN" | jk auth login --with-token
    # then delete the export line from ~/.zshrc (or ~/.bashrc) and open a new shell
    jk auth status    # token_source: keyring
    ```

### Keyring commands

```sh
jk auth login                                     # hidden prompt (terminal only)
printf %s "$TOKEN" | jk auth login --with-token   # from stdin
jk auth login --profile legacy                    # token for another profile
jk auth logout                                    # remove the profile's token
```

The entry is stored under service `jk`, account `<profile>`. One token per
profile; see [several controllers](configuration.md#pick-a-setup).

On Linux the keyring needs a running Secret Service (for example
gnome-keyring). WSL and headless servers often have none; use
`JENKINS_API_TOKEN` there.

## Check which token is used

```sh
jk auth status --json
```

`token_source` is `env:JENKINS_API_TOKEN` or `keyring`. If both are set, the
environment variable wins.

## What jk does with the token

- It is never printed or logged, including in `auth status`, errors and debug
  logs. Requests are logged (at most) as method, path and status on stderr.
- POSTs use the API token with basic auth. If Jenkins answers 403, jk fetches a
  CSRF crumb once and retries.
- Never put the token in `jk.yaml` or `.mcp.json`.

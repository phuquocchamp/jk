---
title: "Authentication"
---

jk signs in to Jenkins with a user's **API token** over HTTP basic auth. Where
the URL and user come from, and the setups for your machine, CI and MCP, are on
[Configuration](/jk/configuration/). This page is about the token.

## Create a token

In Jenkins: your user, then **Security**, then **API Token**, then
**Add new token**. Copy it right away; Jenkins shows it only once.

Use a Jenkins user with only the permissions the agent needs. The jk
[policy](/jk/policy/) is a second line of defence, not a replacement for Jenkins
permissions.

## Give it to jk

There are two ways. Use one of them.

| | OS keyring | `JENKINS_API_TOKEN` |
|---|---|---|
| How | `jk auth login` | `export JENKINS_API_TOKEN=...` |
| Stored | encrypted by the OS (macOS Keychain, Windows Credential Manager, Linux Secret Service) | plain text in your shell rc, `.env` file or CI secret |
| Readable by | jk, when it asks the keyring | every process started from that shell |
| Best for | your own machine, MCP servers | CI, containers, one-off runs |

:::note[Exporting the token does not use the keyring]
The two are separate. When `JENKINS_API_TOKEN` is set, jk uses it and
never reads or writes the keyring. To move an exported token into the
keyring:

```sh
printf %s "$JENKINS_API_TOKEN" | jk auth login --with-token
# then delete the export line from ~/.zshrc (or ~/.bashrc) and open a new shell
jk auth status    # token_source: keyring
```
:::

### Keyring commands

```sh
jk auth login                                     # hidden prompt (terminal only)
printf %s "$TOKEN" | jk auth login --with-token   # from stdin
jk auth login --profile legacy                    # token for another profile
jk auth login --url https://ci.example.com --user jane   # URL and user on the command line
jk auth login --no-verify                         # store without asking Jenkins
jk auth logout                                    # remove the profile's token
```

The entry is stored under service `jk`, account `<profile>`. One token per
profile; see [several controllers](/jk/configuration/#pick-a-setup).

### What `jk auth login` does

1. Picks the profile (`--profile`, `git config jk.profile`, `default_profile`,
   then `default`) and its URL and user: `--url` / `--user`, then
   `JENKINS_URL` / `JENKINS_USER`, then the profile in the config file. When
   one is still missing and stdin is a terminal, it asks.
2. Reads the token from the hidden prompt or stdin.
3. Calls whoami on that Jenkins as that user with the new token.
4. Stores the token in the keyring only if Jenkins accepts it. A rejected
   token exits 6 (`AUTH_REQUIRED`) and nothing is stored; an unreachable
   Jenkins exits 1 (`UPSTREAM`).
5. If the profile is not in the config file yet, writes it there with the
   URL, the user and policy tier `read`, creating `jk.yaml` with mode `0600`
   when needed. An existing profile is never changed.

The result says `"verified": true`, the Jenkins user `id` the token belongs
to, and `profile_saved` / `config_created` / `config_path` for step 5.
`--no-verify` skips steps 3 and 5, for example when Jenkins is not reachable
from this machine; the result then says `"verified": false` and the config
file is left alone.

If `JENKINS_API_TOKEN` is exported, login still stores the token but warns that
the variable takes precedence over the keyring.

### Linux, WSL and headless machines

On Linux the keyring is the Secret Service on D-Bus (for example
gnome-keyring). WSL, containers and headless servers often have none, and jk
then fails with a hint like this:

```sh
# Debian/Ubuntu/WSL
sudo apt install -y gnome-keyring libsecret-1-0
# WSL or a headless session, in each shell (or in ~/.bashrc):
eval "$(gnome-keyring-daemon --start --components=secrets)"
```

Or skip the keyring there and use `JENKINS_API_TOKEN`.

## Check which token is used

```sh
jk auth status --json
```

`token_source` is `env:JENKINS_API_TOKEN` or `keyring`. If both are set, the
environment variable wins. `user_source` is `env:JENKINS_USER` or `file`.

jk reads the user and the token separately. When one comes from the
environment and the other from the config file or keyring (for example an
exported token with the user from `jk.yaml`), `auth status` adds a
`warnings` entry: the token may belong to a different Jenkins user, which
Jenkins answers with 401. Keep both in the environment, or both in the config
file and keyring.

## What jk does with the token

- It is never printed or logged, including in `auth status`, errors and debug
  logs. Requests are logged (at most) as method, path and status on stderr.
- POSTs use the API token with basic auth. If Jenkins answers 403, jk fetches a
  CSRF crumb once and retries.
- Never put the token in `jk.yaml` or `.mcp.json`.

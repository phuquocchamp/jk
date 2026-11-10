# Configuration

jk needs three things: the Jenkins **URL**, your Jenkins **user**, and an
**API token**. It reads them from three places:

| Setting | Config file | Environment variable | OS keyring |
|---|---|---|---|
| URL | `url` | `JENKINS_URL` (wins over the file) | no |
| User | `user` | `JENKINS_USER` (wins over the file) | no |
| Token | **never** | `JENKINS_API_TOKEN` (wins over the keyring) | yes, via `jk auth login` |
| Policy (tier, allow/deny, log cap) | `policy` | no | no |

In short:

- The **config file** holds everything that is not secret.
- The **keyring** holds the token, encrypted by the OS.
- **Environment variables** override both, which is what CI and one-off runs use.

## Pick a setup

=== "My machine (recommended)"

    The URL and user go in the config file, and the token goes in the
    keyring. Nothing needs to be exported in your shell.

    1. Create `~/.config/qtools/jk.yaml`:

        ```yaml
        profiles:
          default:
            url: https://jenkins.example.com
            user: jane
        ```

        ```sh
        chmod 600 ~/.config/qtools/jk.yaml
        ```

    2. Store the token:

        ```sh
        jk auth login                 # hidden prompt
        # or: printf %s "$TOKEN" | jk auth login --with-token
        ```

    3. Check:

        ```sh
        jk auth status    # token_source: keyring
        ```

    If your shell rc still has `export JENKINS_API_TOKEN=...` or
    `export JENKINS_URL=...` from an earlier setup, remove those lines. Env
    values win, so the keyring and the file would be ignored.

=== "CI / container"

    Use environment variables only, with no config file and no keyring.
    Inject the token from the CI's secret store:

    ```sh
    export JENKINS_URL=https://jenkins.example.com
    export JENKINS_USER=ci-bot
    export JENKINS_API_TOKEN="$JENKINS_TOKEN"   # injected by your CI's secret store
    jk auth status    # token_source: env:JENKINS_API_TOKEN
    ```

    Without a config file the tier is `read`. To allow triggers in CI, add a
    config file with `policy.tier: write` (and point `XDG_CONFIG_HOME` at its
    directory if it is not in the default place).

=== "Claude Code / MCP"

    Set up jk as in **My machine** first. Then:

    ```sh
    claude mcp add jk -- jk mcp
    ```

    The server reads the same config file and keyring, so no token goes in
    `.mcp.json`. Desktop apps need the absolute path to `jk` (`which jk`). See
    [MCP server](mcp.md).

=== "Several controllers"

    Use one profile per controller, and one keyring entry per profile:

    ```yaml
    profiles:
      default:
        url: https://jenkins.example.com
        user: jane
      legacy:
        url: https://old-ci.example.org
        user: jane.doe
    default_profile: default
    ```

    ```sh
    jk auth login                     # token for "default"
    jk auth login --profile legacy    # token for "legacy"
    jk job list --profile legacy
    ```

=== "Inside a Jenkins build"

    Jenkins sets `JENKINS_URL` for every build, pointing at the controller
    that runs it, and that **overrides your profile's `url`**. Pass the URL and
    token explicitly, from Jenkins credentials:

    ```groovy
    withCredentials([string(credentialsId: 'jk-token', variable: 'JENKINS_API_TOKEN')]) {
      sh 'JENKINS_USER=ci-bot jk build get team-a/api/main lastFailed'
    }
    ```

    To reach a *different* controller from a build, export its URL as
    `JENKINS_URL` in the same step.

## How jk decides

For each run:

1. **Profile:** `--profile`, then `default_profile` in the file, then
   `default`.
2. **URL and user:** `JENKINS_URL` / `JENKINS_USER` if set, otherwise the
   profile's `url` / `user`.
3. **Token:** `JENKINS_API_TOKEN` if set, otherwise the keyring entry for the
   **profile name** (service `jk`, account `<profile>`).
4. **Tier:** the profile's `policy.tier` (default `read`), lowered by
   `--tier` if given. Nothing can raise it.

`jk auth status` shows the result: `profile`, `url`, `user`, `tier`,
`config_path` and `token_source`. It never shows the token.

!!! warning "Each value is looked up on its own"
    jk picks the URL, the user and the token separately. If one comes from the
    environment and another from the file or keyring, they can belong to
    different accounts or controllers:

    - A keyring token is stored per **profile**, not per URL. If `JENKINS_URL`
      is set, the profile's keyring token is sent to that URL.
    - `JENKINS_USER` set for one account plus a keyring token from another
      gives `AUTH_REQUIRED` (401).

    On your own machine, do not export `JENKINS_*`. In CI, set all three.

## Config file reference

Location: `$XDG_CONFIG_HOME/qtools/jk.yaml`, or `~/.config/qtools/jk.yaml`
when `XDG_CONFIG_HOME` is not set. The file is optional.

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
default_profile: default
```

| Key | Default | Meaning |
|---|---|---|
| `profiles.<name>.url` | none | Jenkins base URL |
| `profiles.<name>.user` | none | Jenkins user the token belongs to |
| `profiles.<name>.policy.tier` | `read` | `read`, `write` or `danger`; see [Policy](policy.md) |
| `profiles.<name>.policy.allow.jobs` | all jobs | job path globs that are allowed |
| `profiles.<name>.policy.deny.jobs` | none | job path globs that are blocked; deny wins |
| `profiles.<name>.policy.max_log_lines` | `2000` | cap for `log tail` and total `log search` output |
| `default_profile` | `default` | profile used when `--profile` is not given |

Rules:

- **Strict parsing.** An unknown key (for example a typo like `dney:`) is an
  error, not a silently ignored rule.
- **Keep it private.** jk warns on stderr if the file is readable by group or
  others; run `chmod 600`. There is no `token` key: tokens never go in this
  file.
- **Missing profile.** A named or `default_profile` profile that does not exist
  is a usage error, unless `JENKINS_URL` is set.

## Common problems

| Symptom | Likely cause | Fix |
|---|---|---|
| `token_source` is `env:JENKINS_API_TOKEN` but you ran `jk auth login` | `JENKINS_API_TOKEN` is exported somewhere | Remove the export and open a new shell |
| `no Jenkins URL configured` | No `url` in the profile and no `JENKINS_URL` | Add `url` to the profile |
| `AUTH_REQUIRED` with a token set | `user` and token belong to different accounts, or the token was revoked | Check `jk auth status`; run `jk auth login` again |
| jk talks to the wrong controller | `JENKINS_URL` is set (always true inside a Jenkins build) | Unset it, or set it on purpose |
| `USAGE` error naming a key | Typo in `jk.yaml` | Fix the key; see the table above |
| Warning about file permissions | `jk.yaml` is readable by others | `chmod 600 ~/.config/qtools/jk.yaml` |
| Keyring error on Linux or WSL | No Secret Service daemon running | Use `JENKINS_API_TOKEN`, or start a keyring daemon |

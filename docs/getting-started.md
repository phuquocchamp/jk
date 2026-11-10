# Getting started

## Install

=== "Go"

    ```sh
    go install github.com/phuquocchamp/jk/cmd/jk@latest
    ```

=== "Release archive"

    Download the archive for your OS and CPU (macOS, Linux or Windows; amd64
    or arm64) from the
    [GitHub releases page](https://github.com/phuquocchamp/jk/releases),
    check it against `checksums.txt`, and put `jk` on your `PATH`.
    Binaries are static (`CGO_ENABLED=0`).

    ```sh
    sha256sum -c checksums.txt --ignore-missing
    tar -xzf jk_*_linux_amd64.tar.gz jk && sudo mv jk /usr/local/bin/
    ```

=== "From source"

    ```sh
    git clone https://github.com/phuquocchamp/jk && cd jk
    make build          # writes bin/jk
    bin/jk version
    ```

## Connect to Jenkins

First create an API token in Jenkins: your user, then **Security**, then
**API Token**. Then pick one setup:

=== "Config file + keyring (recommended)"

    Nothing to export, and the token is encrypted by the OS.

    ```sh
    mkdir -p ~/.config/qtools
    cat > ~/.config/qtools/jk.yaml <<'YAML'
    profiles:
      default:
        url: https://jenkins.example.com
        user: jane
    YAML
    chmod 600 ~/.config/qtools/jk.yaml

    jk auth login        # paste the token at the hidden prompt
    ```

=== "Environment variables only"

    Quickest to try, and what CI uses. The token sits in plain text in your
    shell environment.

    ```sh
    export JENKINS_URL=https://jenkins.example.com
    export JENKINS_USER=jane
    export JENKINS_API_TOKEN=...      # never commit this
    ```

Don't mix the two: an exported `JENKINS_*` value always wins over the config
file and the keyring.

Check that it works:

```sh
jk auth status
```

`token_source` shows where the token came from (`keyring` or
`env:JENKINS_API_TOKEN`); the token itself is never shown. Other setups (MCP,
several controllers, inside a Jenkins build) and what to do when something is
off: [Configuration](configuration.md).

## First commands

```sh
jk job list                                   # jobs at the root
jk job list team-a                            # inside a folder
jk build get team-a/api/main lastFailed       # what happened
jk build stages team-a/api/main lastFailed    # which stage failed
jk log tail team-a/api/main lastFailed --stage Test -n 200
jk log search team-a/api/main 57 -e '(?i)error' -C 3
jk test failed team-a/api/main lastFailed
```

Job arguments are a path like `folder/sub/job`, a multibranch branch like
`project/branch`, or a pasted Jenkins URL. A branch name containing `/` is
written with `%2F`: `project/feature%2Flogin`.

`[ref]` is a build number or `last` (default), `lastFailed`, `lastSuccessful`,
`lastCompleted`. A build number in a pasted URL is used when no ref is given.

## Shell completion

```sh
source <(jk completion zsh)    # or bash / fish / powershell
```

## Next

- Let an agent use it: [MCP server](mcp.md), or the Claude Code skill in
  [`skills/jk/`](https://github.com/phuquocchamp/jk/tree/main/skills/jk).
- Allow builds to be triggered: [Policy](policy.md).

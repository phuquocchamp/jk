## jk completion zsh

Generate the autocompletion script for zsh

### Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(jk completion zsh)

To load completions for every new session, execute once:

#### Linux:

	jk completion zsh > "${fpath[1]}/_jk"

#### macOS:

	jk completion zsh > $(brew --prefix)/share/zsh/site-functions/_jk

You will need to start a new shell for this setup to take effect.


```
jk completion zsh [flags]
```

### Options

```
  -h, --help              help for zsh
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
      --dry-run          show the plan for write operations without executing
      --fields strings   comma-separated top-level fields to keep (per item for lists)
      --jq string        jq expression applied to JSON output
      --json             JSON output (default when stdout is not a TTY)
      --profile string   config profile to use
      --tier string      lower the effective tier: read|write|danger
  -y, --yes              confirm dangerous operations
```

### SEE ALSO

* [jk completion](jk_completion.md)	 - Generate the autocompletion script for the specified shell


---
title: "jk completion fish"
description: "Generate the autocompletion script for fish"
---

Generate the autocompletion script for fish

### Synopsis

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	jk completion fish | source

To load completions for every new session, execute once:

	jk completion fish > ~/.config/fish/completions/jk.fish

You will need to start a new shell for this setup to take effect.


```
jk completion fish [flags]
```

### Options

```
  -h, --help              help for fish
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

* [jk completion](/jk/reference/jk_completion/)	 - Generate the autocompletion script for the specified shell


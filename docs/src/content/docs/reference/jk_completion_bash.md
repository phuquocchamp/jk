---
title: "jk completion bash"
description: "Generate the autocompletion script for bash"
---

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source &lt;(jk completion bash)

To load completions for every new session, execute once:

#### Linux:

	jk completion bash > /etc/bash_completion.d/jk

#### macOS:

	jk completion bash > $(brew --prefix)/etc/bash_completion.d/jk

You will need to start a new shell for this setup to take effect.


```
jk completion bash
```

### Options

```
  -h, --help              help for bash
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


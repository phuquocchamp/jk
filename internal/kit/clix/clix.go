// Package clix holds cobra helpers: global flags, TTY detection, and error ->
// stderr JSON + exit code mapping.
package clix

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/output"
)

// Globals are the persistent flags shared by all commands.
type Globals struct {
	Profile string   // --profile
	JSON    bool     // --json
	Fields  []string // --fields a,b
	JQ      string   // --jq EXPR
	Yes     bool     // --yes (confirm danger ops)
	DryRun  bool     // --dry-run (write/danger ops return a plan)
	Tier    string   // --tier (may only lower the configured tier)
}

// BindGlobals registers the global flags as persistent flags on cmd.
func BindGlobals(cmd *cobra.Command, g *Globals) {
	f := cmd.PersistentFlags()
	f.StringVar(&g.Profile, "profile", "", "config profile to use")
	f.BoolVar(&g.JSON, "json", false, "JSON output (default when stdout is not a TTY)")
	f.StringSliceVar(&g.Fields, "fields", nil, "comma-separated top-level fields to keep (per item for lists)")
	f.StringVar(&g.JQ, "jq", "", "jq expression applied to JSON output")
	f.BoolVarP(&g.Yes, "yes", "y", false, "confirm dangerous operations")
	f.BoolVar(&g.DryRun, "dry-run", false, "show the plan for write operations without executing")
	f.StringVar(&g.Tier, "tier", "", "lower the effective tier: read|write|danger")
}

// JSONMode reports whether output should be JSON for stdout w.
func (g *Globals) JSONMode(w io.Writer) bool {
	return g.JSON || g.JQ != "" || !IsTerminal(w)
}

// OutputOpts builds output.Opts for stdout w.
func (g *Globals) OutputOpts(w io.Writer) output.Opts {
	return output.Opts{JSON: g.JSONMode(w), Fields: g.Fields, JQ: g.JQ}
}

// IsTerminal reports whether v is an *os.File attached to a terminal.
func IsTerminal(v any) bool {
	f, ok := v.(*os.File)
	return ok && f != nil && term.IsTerminal(int(f.Fd()))
}

// CanPrompt reports whether both stdin and stdout are terminals.
func CanPrompt(stdin, stdout any) bool { return IsTerminal(stdin) && IsTerminal(stdout) }

// ReadSecret prints prompt to stderr and reads a line without echo. It returns
// USAGE when stdin is not a terminal (never prompt without a TTY).
func ReadSecret(stdin *os.File, stderr io.Writer, prompt string) (string, error) {
	if !IsTerminal(stdin) {
		return "", errs.Usage("stdin is not a terminal").WithHint("pipe the token and use --with-token")
	}
	fmt.Fprint(stderr, prompt)
	b, err := term.ReadPassword(int(stdin.Fd()))
	fmt.Fprintln(stderr)
	if err != nil {
		return "", errs.Wrap(errs.CodeInternal, err, "read secret")
	}
	return string(b), nil
}

// HandleError writes err to stderr (JSON envelope when jsonMode, else
// "error: CODE: message" plus hint) and returns the exit code. nil -> 0.
func HandleError(stderr io.Writer, err error, jsonMode bool) int {
	if err == nil {
		return 0
	}
	e := errs.From(err)
	if jsonMode {
		_ = json.NewEncoder(stderr).Encode(errs.Envelope{Error: e})
	} else {
		fmt.Fprintf(stderr, "error: %s: %s\n", e.Code, e.Message)
		if e.Hint != "" {
			fmt.Fprintf(stderr, "hint: %s\n", e.Hint)
		}
		if e.Details != nil {
			if b, err := json.MarshalIndent(e.Details, "", "  "); err == nil {
				fmt.Fprintf(stderr, "details: %s\n", b)
			}
		}
	}
	return errs.ExitCode(e.Code)
}

// Execute runs root, routes errors through HandleError and returns the exit code.
// Cobra flag/arg errors and unknown commands are mapped to USAGE. Callers should
// set root.SilenceErrors and root.SilenceUsage.
func Execute(root *cobra.Command, g *Globals, stdout, stderr io.Writer) int {
	PrepareUsageErrors(root)
	err := root.Execute()
	return HandleError(stderr, usageFromCobra(err), g.JSONMode(stdout))
}

// PrepareUsageErrors makes flag parsing and positional-argument errors of
// root and all its subcommands USAGE errors. Execute calls it; tests that run
// root.Execute directly may call it too. It is idempotent.
func PrepareUsageErrors(root *cobra.Command) {
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return withHelpHint(errs.Wrap(errs.CodeUsage, err, err.Error()), c)
	})
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Args != nil {
			c.Args = usageArgs(c.Args)
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

func usageArgs(fn cobra.PositionalArgs) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		err := fn(c, args)
		if err == nil {
			return nil
		}
		if _, ok := errs.As(err); ok {
			return err
		}
		return withHelpHint(errs.Wrap(errs.CodeUsage, err, err.Error()), c)
	}
}

// GroupRunE is the RunE for commands that only group subcommands: with no
// args it prints help, otherwise it reports an unknown subcommand as USAGE
// (cobra would print help and exit 0, which misleads agents).
func GroupRunE(c *cobra.Command, args []string) error {
	if len(args) == 0 {
		return c.Help()
	}
	return withHelpHint(errs.Usagef("unknown command %q for %q", args[0], c.CommandPath()), c)
}

func withHelpHint(e *errs.Error, c *cobra.Command) *errs.Error {
	if e.Hint == "" && c != nil {
		e.Hint = fmt.Sprintf("run '%s --help' for usage", c.CommandPath())
	}
	return e
}

// usageFromCobra maps cobra's plain errors (unknown command, arg validation
// done before our wrappers) to USAGE. Structured errors pass through.
func usageFromCobra(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.As(err); ok {
		return err
	}
	msg := err.Error()
	for _, p := range []string{"unknown command", "unknown flag", "unknown shorthand flag", "invalid argument",
		"flag needs an argument", "accepts ", "requires at least", "requires at most", "required flag", "invalid args"} {
		if strings.Contains(msg, p) {
			return errs.Wrap(errs.CodeUsage, err, msg)
		}
	}
	return err
}

// Package cli defines the cobra command tree. Each catalog command calls exactly
// one core.Service method and renders the result with kit/output.
package cli

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/auth"
	"github.com/phuquocchamp/jk/internal/kit/clix"
	"github.com/phuquocchamp/jk/internal/kit/config"
	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/output"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

// Env is what a command needs after flags are parsed.
type Env struct {
	Service *core.Service
	Profile config.Resolved
	Tier    policy.Tier // effective tier = min(config, --tier)
	// TokenSource says where the token came from ("env:<VAR>" or "keyring").
	TokenSource auth.Source
}

// Deps are injected by main (and by tests).
type Deps struct {
	Version string
	Globals *clix.Globals
	// Open loads config for g.Profile, computes the effective tier from g.Tier,
	// resolves the token and builds client + Service. Called lazily inside RunE,
	// so version/completion/auth login work without a token.
	Open func(ctx context.Context, g *clix.Globals) (*Env, error)
	// LoadProfile resolves a profile without needing a token (auth commands).
	LoadProfile func(profile string) (config.Resolved, error)
	Auth        *auth.Resolver
	// Verify checks a token against Jenkins before 'auth login' stores it, by
	// calling whoami as prof.User with token. nil skips the check.
	Verify func(ctx context.Context, prof config.Resolved, token string) (core.WhoamiOut, error)
	// SaveProfile adds a profile to the config file (creating the file if
	// needed) after 'auth login' verified it. nil never writes the config.
	SaveProfile func(name string, p config.Profile) (config.AddResult, error)
	Stdin       *os.File
	Stdout      io.Writer
	Stderr      io.Writer
	// ServeMCP runs the MCP server on stdio for env (wired by main to mcpserver).
	ServeMCP func(ctx context.Context, env *Env) error
	// CanAsk overrides the interactive-confirmation check (tests only); nil
	// means human mode with stdin and stdout on a terminal.
	CanAsk func() bool
}

// app carries Deps into the command constructors.
type app struct {
	d Deps
	g *clix.Globals
}

// NewRootCmd builds the root command with global flags bound to d.Globals.
func NewRootCmd(d Deps) *cobra.Command {
	if d.Globals == nil {
		d.Globals = &clix.Globals{}
	}
	if d.Stdin == nil {
		d.Stdin = os.Stdin
	}
	if d.Stdout == nil {
		d.Stdout = os.Stdout
	}
	if d.Stderr == nil {
		d.Stderr = os.Stderr
	}
	a := &app{d: d, g: d.Globals}

	root := &cobra.Command{
		Use:   "jk",
		Short: "Agent-first Jenkins CLI and MCP server",
		Long: `jk reads and drives Jenkins from the shell or as an MCP server.

Output is JSON when --json or --jq is set or when stdout is not a terminal.
Errors go to stderr ({"error":{...}} in JSON mode) with stable exit codes:
2 usage, 3 not found, 4 policy denied/forbidden, 5 conflict, 6 auth required,
7 needs confirmation, 1 upstream/internal.`,
		Version:       d.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetIn(d.Stdin)
	root.SetOut(d.Stdout)
	root.SetErr(d.Stderr)
	clix.BindGlobals(root, d.Globals)

	root.AddCommand(
		a.authCmd(),
		a.jobCmd(),
		a.buildCmd(),
		a.logCmd(),
		a.testCmd(),
		a.queueCmd(),
		a.mcpCmd(),
		a.versionCmd(),
	)
	clix.PrepareUsageErrors(root)
	return root
}

// open builds the Env lazily (after flag parsing).
func (a *app) open(cmd *cobra.Command) (*Env, error) {
	if a.d.Open == nil {
		return nil, errs.Internal("no backend configured")
	}
	env, err := a.d.Open(cmd.Context(), a.g)
	if err != nil {
		return nil, err
	}
	if env == nil || env.Service == nil {
		return nil, errs.Internal("no service configured")
	}
	return env, nil
}

// render writes v to stdout with the global output options.
func (a *app) render(v any) error {
	return output.Render(a.d.Stdout, v, a.g.OutputOpts(a.d.Stdout))
}

// group returns a command that only groups subcommands.
func group(use, short string, subs ...*cobra.Command) *cobra.Command {
	c := &cobra.Command{Use: use, Short: short, Args: cobra.ArbitraryArgs, RunE: clix.GroupRunE}
	c.AddCommand(subs...)
	return c
}

// opShort returns the catalog summary for an op (one source of truth with MCP).
func opShort(name string) string {
	if op, ok := core.OpByName(name); ok {
		return op.Summary
	}
	return ""
}

// optArg returns args[i] or "".
func optArg(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

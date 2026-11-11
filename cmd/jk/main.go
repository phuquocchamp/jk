// Command jk is an agent-first Jenkins CLI and MCP server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/phuquocchamp/jk/internal/cli"
	"github.com/phuquocchamp/jk/internal/client"
	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/auth"
	"github.com/phuquocchamp/jk/internal/kit/clix"
	"github.com/phuquocchamp/jk/internal/kit/config"
	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/policy"
	"github.com/phuquocchamp/jk/internal/mcpserver"
)

// version is set by goreleaser via -ldflags "-X main.version=...".
var version = "dev"

var cfgOpts = config.Options{
	Dir:         "qtools",
	App:         "jk",
	URLEnv:      []string{"JENKINS_URL"},
	UserEnv:     []string{"JENKINS_USER"},
	RepoProfile: gitProfile,
}

// configHeader opens a config file that 'jk auth login' creates.
const configHeader = `jk config, created by 'jk auth login'. Edit it freely.
The API token is in the OS keyring, not in this file.
policy.tier: read (default) lets agents only read; write adds trigger and replay;
danger adds stop. Docs: https://phuquocchamp.github.io/jk/configuration/`

// gitProfile returns 'git config jk.profile' for the current directory, so a
// repository can pin the profile for every jk command (and for 'jk mcp' when
// the client starts it in that repository). Outside a repository, without git
// or when unset it returns "". The lookup is bounded so a hung git never
// blocks a command.
func gitProfile() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "config", "--get", "jk.profile").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func main() {
	// Logs go to stderr only; stdout is the data / MCP protocol channel.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	g := &clix.Globals{}
	resolver := auth.NewResolver("jk", "JENKINS_API_TOKEN")

	loadProfile := func(name string) (config.Resolved, error) {
		f, _, err := config.Load(cfgOpts)
		if err != nil {
			return config.Resolved{}, err
		}
		return f.Resolve(name, cfgOpts)
	}

	open := func(ctx context.Context, g *clix.Globals) (*cli.Env, error) {
		prof, err := loadProfile(g.Profile)
		if err != nil {
			return nil, err
		}
		if prof.URL == "" {
			return nil, errs.Usage("no Jenkins URL configured").
				WithHint("set url in the config profile or export JENKINS_URL")
		}
		tier, err := policy.EffectiveTier(prof.Policy.Tier, g.Tier)
		if err != nil {
			return nil, err
		}
		pol := prof.Policy
		pol.Tier = tier
		token, src, err := resolver.Resolve(prof.Name)
		if err != nil {
			return nil, err
		}
		c := client.New(prof.URL, prof.User, token, client.WithUserAgent("jk/"+version))
		return &cli.Env{Service: core.NewService(c, policy.NewEngine(pol)), Profile: prof, Tier: tier, TokenSource: src}, nil
	}

	root := cli.NewRootCmd(cli.Deps{
		Version: version,
		Globals: g,
		Open:    open,
		// Lenient for 'auth login': a profile that is not in the file yet is
		// fine there, login creates it once the token checks out.
		LoadProfile: func(name string) (config.Resolved, error) {
			r, err := loadProfile(name)
			if errors.Is(err, config.ErrProfileNotFound) {
				return r, nil
			}
			return r, err
		},
		SaveProfile: func(name string, p config.Profile) (config.AddResult, error) {
			return config.AddProfile(cfgOpts, name, p, configHeader)
		},
		Auth: resolver,
		Verify: func(ctx context.Context, prof config.Resolved, token string) (core.WhoamiOut, error) {
			c := client.New(prof.URL, prof.User, token, client.WithUserAgent("jk/"+version))
			// whoami is a read op, allowed at the default read tier.
			return core.NewService(c, policy.NewEngine(policy.Policy{Tier: policy.TierRead})).Whoami(ctx, core.WhoamiIn{})
		},
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		ServeMCP: func(ctx context.Context, env *cli.Env) error {
			// stderr only: stdout carries the MCP protocol.
			// The token's source is printed, never the token.
			fmt.Fprintf(os.Stderr, "jk mcp: profile=%s profile_source=%s config=%s tier=%s token_source=%s user_source=%s\n",
				env.Profile.Name, env.Profile.ProfileSource, env.Profile.Path, env.Tier, env.TokenSource, env.Profile.UserSource)
			if w := cli.CredentialMixWarning(env.TokenSource, env.Profile.UserSource); w != "" {
				fmt.Fprintf(os.Stderr, "jk mcp: warning: %s\n", w)
			}
			return mcpserver.Run(ctx, mcpserver.New(env.Service, env.Tier, version))
		},
	})
	root.SetContext(ctx)
	code := clix.Execute(root, g, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

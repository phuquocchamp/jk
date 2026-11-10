// Command jk is an agent-first Jenkins CLI and MCP server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

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
	Dir:     "qtools",
	App:     "jk",
	URLEnv:  []string{"JENKINS_URL"},
	UserEnv: []string{"JENKINS_USER"},
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
		token, _, err := resolver.Resolve(prof.Name)
		if err != nil {
			return nil, err
		}
		c := client.New(prof.URL, prof.User, token, client.WithUserAgent("jk/"+version))
		return &cli.Env{Service: core.NewService(c, policy.NewEngine(pol)), Profile: prof, Tier: tier}, nil
	}

	root := cli.NewRootCmd(cli.Deps{
		Version:     version,
		Globals:     g,
		Open:        open,
		LoadProfile: loadProfile,
		Auth:        resolver,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		ServeMCP: func(ctx context.Context, env *cli.Env) error {
			// stderr only: stdout carries the MCP protocol.
			fmt.Fprintf(os.Stderr, "jk mcp: profile=%s config=%s tier=%s\n",
				env.Profile.Name, env.Profile.Path, env.Tier)
			return mcpserver.Run(ctx, mcpserver.New(env.Service, env.Tier, version))
		},
	})
	root.SetContext(ctx)
	code := clix.Execute(root, g, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

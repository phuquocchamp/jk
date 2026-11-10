package cli

import (
	"bufio"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/clix"
	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/output"
)

// AuthStatusOut is the result of 'auth status'. It never contains the token.
type AuthStatusOut struct {
	Profile     string `json:"profile"`
	URL         string `json:"url"`
	User        string `json:"user,omitempty"`
	ID          string `json:"id"`
	FullName    string `json:"full_name"`
	TokenSource string `json:"token_source"`
	Tier        string `json:"tier"`
	ConfigPath  string `json:"config_path,omitempty"`
}

// AuthLoginOut is the result of 'auth login'.
type AuthLoginOut struct {
	Profile string `json:"profile"`
	Stored  bool   `json:"stored"`
	Backend string `json:"backend"`
}

// AuthLogoutOut is the result of 'auth logout'.
type AuthLogoutOut struct {
	Profile string `json:"profile"`
	Removed bool   `json:"removed"`
}

// VersionOut is the result of 'version'.
type VersionOut struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (v VersionOut) PlainText() string { return v.Name + " " + v.Version }

func (a *app) authCmd() *cobra.Command {
	var withToken bool
	login := &cobra.Command{
		Use:   "login",
		Short: "Store an API token in the OS keyring for the profile",
		Long: `Store a Jenkins API token in the OS keyring (service "jk", account = profile).

With --with-token the token is read from stdin, e.g.
  printf %s "$TOKEN" | jk auth login --with-token
Without it, a hidden prompt is shown only when stdin is a terminal.
The environment variable JENKINS_API_TOKEN takes precedence over the keyring.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.d.Auth == nil {
				return errs.Internal("no token store configured")
			}
			profile, err := a.profileName()
			if err != nil {
				return err
			}
			var token string
			if withToken {
				token, err = readToken(a.d.Stdin)
			} else {
				token, err = clix.ReadSecret(a.d.Stdin, a.d.Stderr, "Jenkins API token for profile "+profile+": ")
			}
			if err != nil {
				return err
			}
			token = strings.TrimSpace(token)
			if token == "" {
				return errs.Usage("empty token")
			}
			if err := a.d.Auth.Store(profile, token); err != nil {
				return err
			}
			return a.render(AuthLoginOut{Profile: profile, Stored: true, Backend: "keyring"})
		},
	}
	login.Flags().BoolVar(&withToken, "with-token", false, "read the token from stdin")

	logout := &cobra.Command{
		Use:   "logout",
		Short: "Remove the profile's API token from the OS keyring",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.d.Auth == nil {
				return errs.Internal("no token store configured")
			}
			profile, err := a.profileName()
			if err != nil {
				return err
			}
			if err := a.d.Auth.Delete(profile); err != nil {
				return err
			}
			return a.render(AuthLogoutOut{Profile: profile, Removed: true})
		},
	}

	status := &cobra.Command{
		Use:   "status",
		Short: opShort(core.OpWhoami),
		Long: opShort(core.OpWhoami) + `

Also shows the profile, effective tier and where the token came from
(env:<VAR> or keyring). The token itself is never printed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			who, err := env.Service.Whoami(cmd.Context(), core.WhoamiIn{})
			if err != nil {
				return err
			}
			out := AuthStatusOut{
				Profile:    env.Profile.Name,
				URL:        who.URL,
				User:       env.Profile.User,
				ID:         who.ID,
				FullName:   who.FullName,
				Tier:       env.Tier.String(),
				ConfigPath: env.Profile.Path,
			}
			if out.URL == "" {
				out.URL = env.Profile.URL
			}
			if a.d.Auth != nil {
				if _, src, err := a.d.Auth.Resolve(env.Profile.Name); err == nil {
					out.TokenSource = string(src)
				}
			}
			return a.render(out)
		},
	}

	return group("auth", "Manage credentials and check who you are", login, logout, status)
}

// profileName resolves the active profile name without needing a token.
func (a *app) profileName() (string, error) {
	if a.d.LoadProfile != nil {
		p, err := a.d.LoadProfile(a.g.Profile)
		if err != nil {
			return "", err
		}
		if p.Name != "" {
			return p.Name, nil
		}
	}
	if a.g.Profile != "" {
		return a.g.Profile, nil
	}
	return "default", nil
}

// readToken reads the first line of r (bounded) as the token.
func readToken(r io.Reader) (string, error) {
	if r == nil {
		return "", errs.Usage("no stdin to read the token from")
	}
	line, err := bufio.NewReader(io.LimitReader(r, 64<<10)).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", errs.Wrap(errs.CodeUsage, err, "read token from stdin")
	}
	return strings.TrimSpace(line), nil
}

func (a *app) mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run the MCP server on stdio",
		Long: `Run jk as an MCP server over stdio. Tools above the effective tier
(policy.tier, optionally lowered with --tier) are not registered.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.d.ServeMCP == nil {
				return errs.Internal("MCP server not configured")
			}
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			return a.d.ServeMCP(cmd.Context(), env)
		},
	}
}

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the jk version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			v := a.d.Version
			if v == "" {
				v = "dev"
			}
			return a.render(VersionOut{Name: "jk", Version: v})
		},
	}
}

// renderTo renders v to w in human or JSON mode following the globals, but
// without --fields/--jq (used for plans shown on stderr).
func (a *app) renderTo(w io.Writer, v any) error {
	return output.Render(w, v, output.Opts{JSON: a.g.JSONMode(a.d.Stdout)})
}

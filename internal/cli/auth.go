package cli

import (
	"bufio"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/auth"
	"github.com/phuquocchamp/jk/internal/kit/clix"
	"github.com/phuquocchamp/jk/internal/kit/config"
	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/output"
)

// AuthStatusOut is the result of 'auth status'. It never contains the token.
type AuthStatusOut struct {
	Profile       string   `json:"profile"`
	ProfileSource string   `json:"profile_source,omitempty"`
	URL           string   `json:"url"`
	User          string   `json:"user,omitempty"`
	ID            string   `json:"id"`
	FullName      string   `json:"full_name"`
	TokenSource   string   `json:"token_source"`
	UserSource    string   `json:"user_source,omitempty"`
	Tier          string   `json:"tier"`
	ConfigPath    string   `json:"config_path,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

// AuthLoginOut is the result of 'auth login'.
type AuthLoginOut struct {
	Profile  string `json:"profile"`
	Stored   bool   `json:"stored"`
	Backend  string `json:"backend"`
	Verified bool   `json:"verified"`
	// ID is the Jenkins user the token authenticated as (when verified).
	ID string `json:"id,omitempty"`
	// ProfileSaved is true when login wrote the profile (url, user, read tier)
	// to the config file at ConfigPath; ConfigCreated when it created the file.
	ProfileSaved  bool   `json:"profile_saved"`
	ConfigCreated bool   `json:"config_created,omitempty"`
	ConfigPath    string `json:"config_path,omitempty"`
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
	var withToken, noVerify bool
	var urlFlag, userFlag string
	login := &cobra.Command{
		Use:   "login",
		Short: "Check an API token against Jenkins, store it in the OS keyring and save the profile",
		Long: `Store a Jenkins API token in the OS keyring (service "jk", account = profile).

The URL and user come from --url / --user, JENKINS_URL / JENKINS_USER, or the
profile in the config file. When they are missing and stdin is a terminal,
login asks for them.

Before storing, jk calls whoami on that URL as that user with the new token,
and stores nothing if Jenkins rejects it. --no-verify skips the check (e.g.
Jenkins is unreachable from here).

When the token checks out and the profile is not in the config file yet,
login writes it there (url, user and policy tier read), creating the file
with mode 0600 if needed. Existing profiles are never changed; raise the tier
by editing the file. With --no-verify nothing is written to the config.

With --with-token the token is read from stdin, e.g.
  printf %s "$TOKEN" | jk auth login --url https://jenkins.example.com --user jane --with-token
Without it, a hidden prompt is shown only when stdin is a terminal.
The environment variable JENKINS_API_TOKEN takes precedence over the keyring.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.d.Auth == nil {
				return errs.Internal("no token store configured")
			}
			prof, err := a.authProfile()
			if err != nil {
				return err
			}
			if v := strings.TrimSpace(urlFlag); v != "" {
				if prof.FromFile && prof.URL != "" && trimURL(v) != prof.URL {
					slog.Warn("--url differs from the url of profile " + prof.Name + " in " + prof.Path + "; the file is not changed")
				}
				prof.URL = v
			}
			if v := strings.TrimSpace(userFlag); v != "" {
				prof.User, prof.UserSource = v, "flag"
			}
			verify := !noVerify && a.d.Verify != nil
			if verify {
				// Ask (or fail) before the token prompt: without url and user there is nothing to check against.
				ask := !withToken && a.canPromptLogin()
				if prof.URL == "" && ask {
					if prof.URL, err = promptLine(a.d.Stdin, a.d.Stderr, "Jenkins URL (e.g. https://jenkins.example.com): "); err != nil {
						return err
					}
				}
				if prof.URL == "" {
					return errs.Usagef("profile %q has no Jenkins URL to verify the token against", prof.Name).
						WithHint("pass --url, export JENKINS_URL or set url in the config profile; or pass --no-verify")
				}
				if prof.User == "" && ask {
					if prof.User, err = promptLine(a.d.Stdin, a.d.Stderr, "Jenkins user: "); err != nil {
						return err
					}
				}
				if prof.User == "" {
					return errs.Usagef("profile %q has no user to verify the token with", prof.Name).
						WithHint("pass --user, export JENKINS_USER or set user in the config profile; or pass --no-verify")
				}
			}
			prof.URL = trimURL(prof.URL)
			if prof.URL != "" && !strings.HasPrefix(prof.URL, "https://") && !strings.HasPrefix(prof.URL, "http://") {
				return errs.Usagef("Jenkins URL %q must start with https:// or http://", prof.URL)
			}
			var token string
			if withToken {
				token, err = readToken(a.d.Stdin)
			} else {
				token, err = clix.ReadSecret(a.d.Stdin, a.d.Stderr, "Jenkins API token for profile "+prof.Name+": ")
			}
			if err != nil {
				return err
			}
			token = strings.TrimSpace(token)
			if token == "" {
				return errs.Usage("empty token")
			}
			out := AuthLoginOut{Profile: prof.Name, Backend: "keyring"}
			if verify {
				who, err := a.d.Verify(cmd.Context(), prof, token)
				if err != nil {
					e := errs.From(err)
					if e.Code == errs.CodeAuthRequired || e.Code == errs.CodeForbidden {
						return e.WithHint("Jenkins at " + prof.URL + " did not accept this token for user " +
							prof.User + "; nothing was stored. Check the token and the user, or pass --no-verify")
					}
					return e.WithHint("could not verify the token with Jenkins at " + prof.URL +
						"; nothing was stored. Check the URL and retry, or pass --no-verify to store it unchecked")
				}
				out.Verified, out.ID = true, who.ID
			}
			if err := a.d.Auth.Store(prof.Name, token); err != nil {
				return err
			}
			out.Stored = true
			// Only a verified profile goes into the config, so the file never
			// holds a URL or user that has not worked at least once.
			if out.Verified && !prof.FromFile && a.d.SaveProfile != nil {
				res, err := a.d.SaveProfile(prof.Name, config.Profile{
					URL: prof.URL, User: prof.User, Policy: config.PolicyConfig{Tier: "read"},
				})
				if err != nil {
					slog.Warn("token stored, but the profile was not saved to the config: " + err.Error())
				} else {
					out.ProfileSaved, out.ConfigCreated, out.ConfigPath = res.Added, res.Created, res.Path
				}
			}
			if name, ok := a.d.Auth.EnvToken(); ok {
				slog.Warn(name + " is set and takes precedence over the keyring; unset it to use the stored token")
			}
			return a.render(out)
		},
	}
	login.Flags().BoolVar(&withToken, "with-token", false, "read the token from stdin")
	login.Flags().BoolVar(&noVerify, "no-verify", false, "store the token without checking it against Jenkins (and without saving the profile)")
	login.Flags().StringVar(&urlFlag, "url", "", "Jenkins URL for this login (default: JENKINS_URL or the profile's url)")
	login.Flags().StringVar(&userFlag, "user", "", "Jenkins user for this login (default: JENKINS_USER or the profile's user)")

	logout := &cobra.Command{
		Use:   "logout",
		Short: "Remove the profile's API token from the OS keyring",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.d.Auth == nil {
				return errs.Internal("no token store configured")
			}
			prof, err := a.authProfile()
			if err != nil {
				return err
			}
			if err := a.d.Auth.Delete(prof.Name); err != nil {
				return err
			}
			return a.render(AuthLogoutOut{Profile: prof.Name, Removed: true})
		},
	}

	status := &cobra.Command{
		Use:   "status",
		Short: opShort(core.OpWhoami),
		Long: opShort(core.OpWhoami) + `

Also shows the profile, effective tier, where the token came from
(env:<VAR> or keyring) and where the user came from (env:<VAR> or file).
It warns when the two come from different places, since the token may then
belong to another Jenkins user. The token itself is never printed.`,
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
				Profile:       env.Profile.Name,
				ProfileSource: env.Profile.ProfileSource,
				URL:           who.URL,
				User:          env.Profile.User,
				ID:            who.ID,
				FullName:      who.FullName,
				UserSource:    env.Profile.UserSource,
				Tier:          env.Tier.String(),
				ConfigPath:    env.Profile.Path,
			}
			if out.URL == "" {
				out.URL = env.Profile.URL
			}
			src := env.TokenSource
			if src == "" && a.d.Auth != nil {
				if _, s, err := a.d.Auth.Resolve(env.Profile.Name); err == nil {
					src = s
				}
			}
			out.TokenSource = string(src)
			if w := CredentialMixWarning(src, env.Profile.UserSource); w != "" {
				out.Warnings = append(out.Warnings, w)
			}
			return a.render(out)
		},
	}

	return group("auth", "Manage credentials and check who you are", login, logout, status)
}

// authProfile resolves the active profile without needing a token. The name
// falls back to --profile, then "default".
func (a *app) authProfile() (config.Resolved, error) {
	var p config.Resolved
	if a.d.LoadProfile != nil {
		var err error
		if p, err = a.d.LoadProfile(a.g.Profile); err != nil {
			return config.Resolved{}, err
		}
	}
	if p.Name == "" {
		p.Name = a.g.Profile
	}
	if p.Name == "" {
		p.Name = "default"
	}
	return p, nil
}

// CredentialMixWarning returns a warning when the token and the user come from
// different places (one from the environment, the other from the keyring or the
// config file): an exported token then gets paired with a user it may not
// belong to, and Jenkins answers 401. Empty when they match or the user is unset.
func CredentialMixWarning(tokenSource auth.Source, userSource string) string {
	if tokenSource == "" || tokenSource == auth.SourceNone || userSource == "" {
		return ""
	}
	tokenEnv := strings.HasPrefix(string(tokenSource), "env:")
	userEnv := strings.HasPrefix(userSource, "env:")
	if tokenEnv == userEnv {
		return ""
	}
	return "token comes from " + string(tokenSource) + " but user from " + userSource +
		"; keep both in the environment or both in the config file and keyring, so the token belongs to that user"
}

// canPromptLogin reports whether login may ask for a missing URL or user:
// stdin is a terminal (tests override it through Deps.CanAsk).
func (a *app) canPromptLogin() bool {
	if a.d.CanAsk != nil {
		return a.d.CanAsk()
	}
	return clix.IsTerminal(a.d.Stdin)
}

// promptLine prints prompt to w and reads one line from r, one byte at a time
// so nothing after the newline is consumed (the token prompt reads next).
func promptLine(r *os.File, w io.Writer, prompt string) (string, error) {
	if r == nil {
		return "", errs.Usage("no stdin to read from")
	}
	_, _ = io.WriteString(w, prompt)
	var b strings.Builder
	buf := make([]byte, 1)
	for b.Len() < 4096 {
		n, err := r.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				break
			}
			b.WriteByte(buf[0])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errs.Wrap(errs.CodeUsage, err, "read input")
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func trimURL(u string) string { return strings.TrimRight(strings.TrimSpace(u), "/") }

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

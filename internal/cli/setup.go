package cli

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/policy"
	"github.com/phuquocchamp/jk/skills"
)

// System is the part of the OS that 'jk setup' touches; tests replace it.
type System struct {
	Executable func() (string, error)
	LookPath   func(file string) (string, error)
	// Run executes name with args and returns combined output.
	Run     func(ctx context.Context, name string, args ...string) ([]byte, error)
	HomeDir func() (string, error)
}

func (a *app) sys() System {
	s := System{}
	if a.d.System != nil {
		s = *a.d.System
	}
	if s.Executable == nil {
		s.Executable = os.Executable
	}
	if s.LookPath == nil {
		s.LookPath = exec.LookPath
	}
	if s.Run == nil {
		s.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		}
	}
	if s.HomeDir == nil {
		s.HomeDir = os.UserHomeDir
	}
	return s
}

// ---- outputs ----

type setupNextOut struct {
	Steps []string `json:"steps"`
}

func (v setupNextOut) PlainText() string { return strings.Join(v.Steps, "\n") + "\n" }

type skillFile struct {
	Path   string `json:"path"`
	Action string `json:"action" jsonschema:"created, updated, unchanged, or the planned action with --dry-run"`
}

type setupSkillOut struct {
	Dir    string      `json:"dir"`
	DryRun bool        `json:"dry_run"`
	Files  []skillFile `json:"files"`
}

func (v setupSkillOut) TableHeader() []string { return []string{"action", "path"} }
func (v setupSkillOut) TableRows() [][]string {
	rows := make([][]string, 0, len(v.Files))
	for _, f := range v.Files {
		rows = append(rows, []string{f.Action, f.Path})
	}
	return rows
}

type setupMCPOut struct {
	Client  string         `json:"client" jsonschema:"claude or print"`
	DryRun  bool           `json:"dry_run"`
	Command []string       `json:"command,omitempty" jsonschema:"the claude command run (or planned)"`
	Output  string         `json:"output,omitempty"`
	Config  map[string]any `json:"config,omitempty" jsonschema:"MCP server entry for clients configured with JSON"`
}

// ---- commands ----

func (a *app) setupCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "setup",
		Short: "Install the agent skill and register jk as an MCP server",
		Long: `Without a subcommand, prints the steps to set jk up for an agent.

  jk setup skill   writes the jenkins skill to ~/.claude/skills/jenkins
  jk setup mcp     registers 'jk mcp' with Claude Code, or prints the JSON entry`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.render(setupNextOut{Steps: []string{
				"1. jk auth login                  # store the API token in the keyring",
				"2. jk setup skill                 # skill for agents that run jk in a shell",
				"3. jk setup mcp --tier read       # MCP server for Claude Code (or --client print)",
				"Docs: https://phuquocchamp.github.io/jk/getting-started/",
			}})
		},
	}
	c.AddCommand(a.setupSkillCmd(), a.setupMCPCmd())
	return c
}

func (a *app) setupSkillCmd() *cobra.Command {
	var (
		dir   string
		force bool
	)
	c := &cobra.Command{
		Use:   "skill [--dir DIR] [--force]",
		Short: "Write the jenkins agent skill (default ~/.claude/skills/jenkins)",
		Long: `Writes the jenkins skill bundled in this binary to DIR/jenkins (default
~/.claude/skills). A file that differs from the bundled copy is not replaced
without --force (exit 5, CONFLICT). --dry-run lists what would be written.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := a.sys()
			if dir == "" {
				home, err := s.HomeDir()
				if err != nil {
					return errs.Wrap(errs.CodeUsage, err, "cannot find the home directory").WithHint("pass --dir")
				}
				dir = filepath.Join(home, ".claude", "skills")
			}
			out, err := installSkill(dir, force, a.g.DryRun)
			if err != nil {
				return err
			}
			return a.render(out)
		},
	}
	c.Flags().StringVar(&dir, "dir", "", "skills directory (default ~/.claude/skills)")
	c.Flags().BoolVar(&force, "force", false, "replace files that differ from the bundled skill")
	return c
}

// installSkill copies skills.FS/jenkins to dir/jenkins. Conflicts are checked
// for every file before anything is written.
func installSkill(dir string, force, dryRun bool) (setupSkillOut, error) {
	out := setupSkillOut{Dir: filepath.Join(dir, "jenkins"), DryRun: dryRun, Files: []skillFile{}}
	type plan struct {
		dst    string
		data   []byte
		action string
	}
	var plans []plan
	var conflicts []string
	err := fs.WalkDir(skills.FS, "jenkins", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := skills.FS.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(p))
		action := "created"
		if old, err := os.ReadFile(dst); err == nil {
			switch {
			case bytes.Equal(old, data):
				action = "unchanged"
			case force:
				action = "updated"
			default:
				conflicts = append(conflicts, dst)
				action = "conflict"
			}
		}
		plans = append(plans, plan{dst, data, action})
		return nil
	})
	if err != nil {
		return out, errs.Wrap(errs.CodeInternal, err, "read the bundled skill")
	}
	if len(conflicts) > 0 {
		return out, errs.Newf(errs.CodeConflict, "%d skill file(s) differ from the bundled copy", len(conflicts)).
			WithHint("pass --force to replace them (local edits are lost)").
			WithDetails(map[string]any{"files": conflicts})
	}
	for _, pl := range plans {
		if !dryRun && pl.action != "unchanged" {
			if err := os.MkdirAll(filepath.Dir(pl.dst), 0o755); err != nil {
				return out, errs.Wrap(errs.CodeInternal, err, "create "+filepath.Dir(pl.dst))
			}
			if err := os.WriteFile(pl.dst, pl.data, 0o644); err != nil {
				return out, errs.Wrap(errs.CodeInternal, err, "write "+pl.dst)
			}
		}
		out.Files = append(out.Files, skillFile{Path: pl.dst, Action: pl.action})
	}
	return out, nil
}

func (a *app) setupMCPCmd() *cobra.Command {
	var client, scope, tier string
	c := &cobra.Command{
		Use:   "mcp [--client claude|print] [--scope local|project|user] [--tier read]",
		Short: "Register 'jk mcp' with Claude Code, or print the JSON entry for other clients",
		Long: `--client claude (the default when 'claude' is on PATH) runs
  claude mcp add --scope SCOPE jk -- /absolute/path/to/jk mcp [--tier TIER]
--client print outputs the server entry (with the absolute path to jk) for
Claude Desktop, Cursor and other clients configured with JSON.

The token is never passed to the client: 'jk mcp' reads it from the keyring
or JENKINS_API_TOKEN at start. --tier pins the server below the config tier.
--dry-run shows the command without running it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := a.sys()
			if tier != "" {
				if _, err := policy.ParseTier(tier); err != nil {
					return err
				}
			}
			switch scope {
			case "local", "project", "user":
			default:
				return errs.Usagef("invalid --scope %q", scope).WithHint("use local, project or user")
			}
			bin, err := s.Executable()
			if err != nil {
				return errs.Wrap(errs.CodeInternal, err, "find the jk executable")
			}
			if resolved, err := filepath.EvalSymlinks(bin); err == nil {
				bin = resolved
			}
			args := []string{"mcp"}
			if tier != "" {
				args = append(args, "--tier", strings.ToLower(tier))
			}
			if client == "" {
				client = "print"
				if _, err := s.LookPath("claude"); err == nil {
					client = "claude"
				}
			}
			switch client {
			case "print":
				return a.render(setupMCPOut{Client: "print", DryRun: a.g.DryRun, Config: map[string]any{
					"mcpServers": map[string]any{"jk": map[string]any{"command": bin, "args": args}},
				}})
			case "claude":
				command := append([]string{"claude", "mcp", "add", "--scope", scope, "jk", "--", bin}, args...)
				out := setupMCPOut{Client: "claude", DryRun: a.g.DryRun, Command: command}
				if a.g.DryRun {
					return a.render(out)
				}
				if _, err := s.LookPath("claude"); err != nil {
					return errs.New(errs.CodeNotFound, "claude is not on PATH").
						WithHint("install Claude Code, or use --client print and add the entry by hand")
				}
				b, err := s.Run(cmd.Context(), command[0], command[1:]...)
				out.Output = strings.TrimSpace(string(b))
				if err != nil {
					code := errs.CodeUpstream
					if strings.Contains(strings.ToLower(out.Output), "already exists") {
						code = errs.CodeConflict
					}
					return errs.Newf(code, "claude mcp add failed: %s", firstNonEmpty(out.Output, err.Error())).
						WithHint(fmt.Sprintf("remove the old entry with 'claude mcp remove jk --scope %s' and run again", scope)).
						WithDetails(out)
				}
				return a.render(out)
			default:
				return errs.Usagef("invalid --client %q", client).WithHint("use claude or print")
			}
		},
	}
	c.Flags().StringVar(&client, "client", "", "claude or print (default: claude when it is on PATH)")
	c.Flags().StringVar(&scope, "scope", "local", "Claude Code scope: local, project or user")
	c.Flags().StringVar(&tier, "tier", "", "pin the server's tier (read, write, danger)")
	return c
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

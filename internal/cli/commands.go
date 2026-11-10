package cli

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/clix"
	"github.com/phuquocchamp/jk/internal/kit/errs"
)

const jobArgHelp = `<job> is a job path like folder/sub/job (multibranch: project/branch, with
"/" in branch names written as %2F) or a full Jenkins job/build URL.
[ref] is a build number or last, lastFailed, lastSuccessful, lastCompleted (default last).`

// ---- job ----

func (a *app) jobCmd() *cobra.Command {
	var limit int
	list := &cobra.Command{
		Use:   "list [folder]",
		Short: opShort(core.OpJobList),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			out, err := env.Service.JobList(cmd.Context(), core.JobListIn{Folder: optArg(args, 0), Limit: limit})
			if err != nil {
				return err
			}
			return a.render(jobListView(out))
		},
	}
	list.Flags().IntVar(&limit, "limit", core.DefaultListLimit, "max entries to return")

	get := &cobra.Command{
		Use:   "get <job>",
		Short: opShort(core.OpJobGet),
		Long:  opShort(core.OpJobGet) + "\n\n" + jobArgHelp,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			out, err := env.Service.JobGet(cmd.Context(), core.JobGetIn{Job: args[0]})
			if err != nil {
				return err
			}
			return a.render(out)
		},
	}
	return group("job", "Discover and inspect jobs", list, get)
}

// ---- build ----

func (a *app) buildCmd() *cobra.Command {
	get := &cobra.Command{
		Use:   "get <job> [ref]",
		Short: opShort(core.OpBuildGet),
		Long:  opShort(core.OpBuildGet) + "\n\n" + jobArgHelp,
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			out, err := env.Service.BuildGet(cmd.Context(), core.BuildGetIn{Job: args[0], Ref: core.BuildRef(optArg(args, 1))})
			if err != nil {
				return err
			}
			return a.render(out)
		},
	}

	stages := &cobra.Command{
		Use:   "stages <job> [ref]",
		Short: opShort(core.OpBuildStages),
		Long:  opShort(core.OpBuildStages) + "\n\n" + jobArgHelp,
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			out, err := env.Service.BuildStages(cmd.Context(), core.BuildStagesIn{Job: args[0], Ref: core.BuildRef(optArg(args, 1))})
			if err != nil {
				return err
			}
			return a.render(stagesView(out))
		},
	}

	return group("build", "Inspect, trigger, replay and stop builds",
		get, stages, a.buildTriggerCmd(), a.buildReplayCmd(), a.buildStopCmd())
}

func (a *app) buildTriggerCmd() *cobra.Command {
	var (
		params  []string
		wait    bool
		timeout string
	)
	c := &cobra.Command{
		Use:   "trigger <job> [-p k=v ...] [--wait [--timeout 10m]]",
		Short: opShort(core.OpBuildTrigger),
		Long: opShort(core.OpBuildTrigger) + `

Parameters are passed with -p name=value (repeatable). Jobs with parameter
definitions are triggered through buildWithParameters. --wait polls the queue
item and then the build until it finishes or --timeout (seconds or a Go
duration such as 10m; default 600s) expires. --dry-run prints the plan.

` + jobArgHelp,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pm, err := parseParams(params)
			if err != nil {
				return err
			}
			secs := 0
			if cmd.Flags().Changed("timeout") {
				if !wait {
					return errs.Usage("--timeout requires --wait")
				}
				if secs, err = parseTimeout(timeout); err != nil {
					return err
				}
			}
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			out, err := env.Service.BuildTrigger(cmd.Context(), core.BuildTriggerIn{
				Job: args[0], Params: pm, Wait: wait, TimeoutSeconds: secs, DryRun: a.g.DryRun,
			})
			if err != nil {
				return err
			}
			return a.render(out)
		},
	}
	c.Flags().StringArrayVarP(&params, "param", "p", nil, "build parameter name=value (repeatable)")
	c.Flags().BoolVar(&wait, "wait", false, "wait for the build to start and finish")
	c.Flags().StringVar(&timeout, "timeout", "", "wait timeout: seconds or a duration like 10m (default 600s)")
	return c
}

func (a *app) buildReplayCmd() *cobra.Command {
	var scriptFile string
	c := &cobra.Command{
		Use:   "replay <job> <ref> --script-file F",
		Short: opShort(core.OpBuildReplay),
		Long: opShort(core.OpBuildReplay) + `

--script-file is the Jenkinsfile to run instead of the original main script;
use "-" to read it from stdin. Loaded libraries/secondary scripts are not
replaced. --dry-run prints the plan.

` + jobArgHelp,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			script, err := a.readScript(scriptFile)
			if err != nil {
				return err
			}
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			out, err := env.Service.BuildReplay(cmd.Context(), core.BuildReplayIn{
				Job: args[0], Ref: core.BuildRef(args[1]), Script: script, DryRun: a.g.DryRun,
			})
			if err != nil {
				return err
			}
			return a.render(out)
		},
	}
	c.Flags().StringVar(&scriptFile, "script-file", "", `Jenkinsfile to replay with ("-" for stdin)`)
	return c
}

func (a *app) buildStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <job> <ref>",
		Short: opShort(core.OpBuildStop),
		Long: opShort(core.OpBuildStop) + `

Pass --yes to confirm. Without it the command exits 7 (NEEDS_CONFIRMATION)
with the plan in the error details; on an interactive terminal it asks first.

` + jobArgHelp,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			in := core.BuildStopIn{Job: args[0], Ref: core.BuildRef(args[1]), Confirm: a.g.Yes, DryRun: a.g.DryRun}
			out, err := env.Service.BuildStop(cmd.Context(), in)
			if errs.Is(err, errs.CodeNeedsConfirmation) && a.canAsk() {
				ok, perr := a.confirm(err, "Stop this build?")
				if perr != nil {
					return perr
				}
				if !ok {
					return err
				}
				// Pin the build shown in the plan: re-resolving a permalink
				// like "last" could hit a build started while the prompt was open.
				e, _ := errs.As(err)
				plan, _ := e.Details.(*core.Plan)
				if plan == nil || plan.Number <= 0 {
					return errs.Internal("confirmation plan has no build number")
				}
				in.Ref = core.BuildRef(strconv.Itoa(plan.Number))
				in.Confirm = true
				out, err = env.Service.BuildStop(cmd.Context(), in)
			}
			if err != nil {
				return err
			}
			return a.render(out)
		},
	}
}

// canAsk reports whether an interactive confirmation is allowed: human mode
// with both stdin and stdout on a terminal.
func (a *app) canAsk() bool {
	if a.d.CanAsk != nil {
		return a.d.CanAsk()
	}
	return !a.g.JSONMode(a.d.Stdout) && clix.CanPrompt(a.d.Stdin, a.d.Stdout)
}

// confirm shows the plan from a NEEDS_CONFIRMATION error and asks y/N on stderr.
func (a *app) confirm(needs error, question string) (bool, error) {
	if e, ok := errs.As(needs); ok && e.Details != nil {
		fmt.Fprintln(a.d.Stderr, "plan:")
		if err := a.renderTo(a.d.Stderr, e.Details); err != nil {
			return false, err
		}
	}
	fmt.Fprintf(a.d.Stderr, "%s [y/N] ", question)
	line, err := bufio.NewReader(a.d.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, errs.Wrap(errs.CodeInternal, err, "read confirmation")
	}
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes", nil
}

func (a *app) readScript(path string) (string, error) {
	if path == "" {
		return "", errs.Usage("--script-file is required").WithHint(`pass a Jenkinsfile path or "-" for stdin`)
	}
	var (
		b   []byte
		err error
	)
	if path == "-" {
		b, err = io.ReadAll(io.LimitReader(a.d.Stdin, 8<<20))
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return "", errs.Wrap(errs.CodeUsage, err, "read --script-file: "+err.Error())
	}
	if strings.TrimSpace(string(b)) == "" {
		return "", errs.Usage("--script-file is empty")
	}
	return string(b), nil
}

// parseParams turns repeated name=value flags into a map; the first "="
// separates name and value, so values may contain "=" and ",".
func parseParams(in []string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for _, p := range in {
		k, v, ok := strings.Cut(p, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, errs.Usagef("invalid parameter %q", p).WithHint("use -p name=value")
		}
		out[k] = v
	}
	return out, nil
}

// parseTimeout accepts whole seconds ("600") or a Go duration ("10m").
func parseTimeout(s string) (int, error) {
	s = strings.TrimSpace(s)
	if n, err := strconv.Atoi(s); err == nil {
		if n <= 0 {
			return 0, errs.Usagef("invalid --timeout %q", s)
		}
		return n, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, errs.Usagef("invalid --timeout %q", s).WithHint("use seconds (600) or a duration (10m)")
	}
	return int(math.Ceil(d.Seconds())), nil
}

// ---- log ----

func (a *app) logCmd() *cobra.Command {
	var (
		lines int
		stage string
	)
	tail := &cobra.Command{
		Use:   "tail <job> [ref]",
		Short: opShort(core.OpBuildLog),
		Long: opShort(core.OpBuildLog) + `

--lines defaults to and is capped by the policy max_log_lines. --stage limits
the output to one pipeline stage (name or id, see 'build stages').

` + jobArgHelp,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			out, err := env.Service.BuildLog(cmd.Context(), core.BuildLogIn{
				Job: args[0], Ref: core.BuildRef(optArg(args, 1)), Lines: lines, Stage: stage,
			})
			if err != nil {
				return err
			}
			return a.render(logView(out))
		},
	}
	tail.Flags().IntVarP(&lines, "lines", "n", 0, "number of trailing lines (default: policy max_log_lines)")
	tail.Flags().StringVar(&stage, "stage", "", "only this stage's log (name or id)")

	var (
		pattern    string
		ctxLines   int
		maxMatches int
	)
	search := &cobra.Command{
		Use:   "search <job> [ref] --pattern RE",
		Short: opShort(core.OpLogSearch),
		Long: opShort(core.OpLogSearch) + `

--pattern is a Go RE2 regular expression matched per line.

` + jobArgHelp,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if pattern == "" {
				return errs.Usage("--pattern is required")
			}
			if ctxLines < 0 || maxMatches < 0 {
				return errs.Usage("--context and --max-matches must not be negative")
			}
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			out, err := env.Service.LogSearch(cmd.Context(), core.LogSearchIn{
				Job: args[0], Ref: core.BuildRef(optArg(args, 1)), Pattern: pattern, Context: ctxLines, MaxMatches: maxMatches,
			})
			if err != nil {
				return err
			}
			return a.render(searchView(out))
		},
	}
	search.Flags().StringVarP(&pattern, "pattern", "e", "", "regular expression (required)")
	search.Flags().IntVarP(&ctxLines, "context", "C", 0, "lines of context around each match")
	search.Flags().IntVar(&maxMatches, "max-matches", core.DefaultMaxMatches, "stop after this many matches")

	return group("log", "Read and search build logs", tail, search)
}

// ---- test ----

func (a *app) testCmd() *cobra.Command {
	var limit int
	failed := &cobra.Command{
		Use:   "failed <job> [ref]",
		Short: opShort(core.OpTestFailed),
		Long:  opShort(core.OpTestFailed) + "\n\n" + jobArgHelp,
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			out, err := env.Service.TestFailed(cmd.Context(), core.TestFailedIn{
				Job: args[0], Ref: core.BuildRef(optArg(args, 1)), Limit: limit,
			})
			if err != nil {
				return err
			}
			return a.render(testView(out))
		},
	}
	failed.Flags().IntVar(&limit, "limit", core.DefaultListLimit, "max failed cases to return")
	return group("test", "Inspect JUnit test results", failed)
}

// ---- queue ----

func (a *app) queueCmd() *cobra.Command {
	get := &cobra.Command{
		Use:   "get <id>",
		Short: opShort(core.OpQueueGet),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if err != nil || id <= 0 {
				return errs.Usagef("invalid queue id %q", args[0]).WithHint("use the queue_id returned by 'build trigger'")
			}
			env, err := a.open(cmd)
			if err != nil {
				return err
			}
			out, err := env.Service.QueueGet(cmd.Context(), core.QueueGetIn{ID: id})
			if err != nil {
				return err
			}
			return a.render(out)
		},
	}
	return group("queue", "Follow queued builds", get)
}

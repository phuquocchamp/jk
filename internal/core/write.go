package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/interact"
	"github.com/phuquocchamp/jk/internal/kit/output"
)

// BuildTrigger starts a build. Parameterized jobs (non-empty parameter
// definitions) use buildWithParameters. With Wait it polls the queue item and
// then the build until it finishes or the timeout passes (TimedOut=true, nil error).
func (s *Service) BuildTrigger(ctx context.Context, in BuildTriggerIn) (BuildTriggerOut, error) {
	path, _, err := s.target(OpBuildTrigger, in.Job)
	if err != nil {
		return BuildTriggerOut{}, err
	}
	if err := nonNegative("timeout_seconds", in.TimeoutSeconds); err != nil {
		return BuildTriggerOut{}, err
	}
	for k := range in.Params {
		if strings.TrimSpace(k) == "" {
			return BuildTriggerOut{}, errs.Usage("parameter name is empty")
		}
	}
	job, err := s.backend.GetJob(ctx, path)
	if err != nil {
		return BuildTriggerOut{}, err
	}
	parameterized := len(job.Parameters) > 0
	if !parameterized && len(in.Params) > 0 {
		// Jenkins would silently ignore them on /build.
		return BuildTriggerOut{}, errs.Usagef("job %q defines no parameters, but %d were given", path, len(in.Params)).
			WithHint("drop the parameters; for a multibranch branch whose Jenkinsfile declares parameters, run one build first so Jenkins learns them")
	}
	out := BuildTriggerOut{Job: path}
	if unknown := unknownParams(job.Parameters, in.Params); len(unknown) > 0 {
		known := paramNames(job.Parameters)
		if !in.AllowUnknown {
			// Jenkins drops unknown names without an error, so a typo would
			// run the build with the default value.
			return BuildTriggerOut{}, errs.Usagef("unknown parameter %s for job %q", strings.Join(unknown, ", "), path).
				WithHint("known parameters: " + strings.Join(known, ", ") + "; pass allow_unknown=true (CLI: --allow-unknown) to send them anyway").
				WithDetails(map[string]any{"unknown": unknown, "known": known})
		}
		out.Warnings = append(out.Warnings, "parameters not defined by the job (Jenkins ignores them): "+strings.Join(unknown, ", "))
	}
	if err := checkChoices(job.Parameters, in.Params); err != nil {
		return BuildTriggerOut{}, err
	}

	plan := &Plan{Op: OpBuildTrigger, Method: "POST", Path: JobURLPath(path) + "build"}
	if parameterized {
		plan.Path = JobURLPath(path) + "buildWithParameters"
		if len(in.Params) > 0 {
			plan.Params = maps.Clone(in.Params)
		}
	}
	if in.DryRun {
		out.DryRun, out.Plan = true, plan
		return out, nil
	}

	qid, err := s.backend.TriggerBuild(ctx, path, in.Params, parameterized)
	if err != nil {
		return BuildTriggerOut{}, err
	}
	out.QueueID = qid
	if !in.Wait {
		return out, nil
	}
	if qid <= 0 {
		return out, errs.New(errs.CodeUpstream, "build was triggered but Jenkins returned no queue id; cannot wait").
			WithHint("do not re-trigger; find the build with build_get " + path).
			WithDetails(out)
	}
	out, err = s.waitBuild(ctx, path, out, s.waitTimeout(in.TimeoutSeconds))
	if err != nil {
		return out, afterTrigger(err, out)
	}
	if out.TimedOut {
		out.Next = waitNext(path, out.QueueID, out.Number)
	}
	return out, nil
}

// waitTimeout returns the requested timeout (default DefaultTriggerTimeout),
// capped by maxWait when set.
func (s *Service) waitTimeout(secs int) time.Duration {
	timeout := DefaultTriggerTimeout
	if secs > 0 {
		timeout = time.Duration(secs) * time.Second
	}
	if s.maxWait > 0 && timeout > s.maxWait {
		timeout = s.maxWait
	}
	return timeout
}

// waitNext tells the caller how to keep waiting: by build number once known,
// else by queue id.
func waitNext(path string, qid int64, number *int) *Next {
	n := &Next{Tool: MCPPrefix + OpBuildWait, Args: map[string]any{"job": path}}
	if number != nil {
		n.Args["ref"] = *number
		n.CLI = fmt.Sprintf("jk build wait %s %d", path, *number)
	} else {
		n.Args["queue_id"] = qid
		n.CLI = fmt.Sprintf("jk build wait %s --queue-id %d", path, qid)
	}
	return n
}

// BuildWait waits for a queued (queue_id) or started (ref) build to finish,
// bounded by the timeout. Running out of time is not an error: timed_out is
// set with the next call to make.
func (s *Service) BuildWait(ctx context.Context, in BuildWaitIn) (BuildWaitOut, error) {
	path, urlRef, err := s.target(OpBuildWait, in.Job)
	if err != nil {
		return BuildWaitOut{}, err
	}
	if err := nonNegative("timeout_seconds", in.TimeoutSeconds); err != nil {
		return BuildWaitOut{}, err
	}
	if in.QueueID < 0 {
		return BuildWaitOut{}, errs.Usagef("invalid queue id %d", in.QueueID)
	}
	st := BuildTriggerOut{Job: path, QueueID: in.QueueID}
	if in.QueueID == 0 || in.Ref != "" || urlRef != "" {
		ref, err := pickRef(in.Ref, urlRef, in.QueueID == 0)
		if err != nil {
			return BuildWaitOut{}, errs.From(err).WithHint("pass the build number (ref) or the queue_id from build_trigger")
		}
		n, err := s.number(ctx, path, ref)
		if err != nil {
			return BuildWaitOut{}, err
		}
		st.Number = &n
	}
	st, err = s.waitBuild(ctx, path, st, s.waitTimeout(in.TimeoutSeconds))
	out := BuildWaitOut{Job: path, QueueID: st.QueueID, Number: st.Number, Result: st.Result,
		Building: st.Building, URL: st.URL, TimedOut: st.TimedOut}
	if err != nil {
		return out, err
	}
	if out.TimedOut {
		out.Next = waitNext(path, out.QueueID, out.Number)
	}
	return out, nil
}

func paramNames(defs []ParamDef) []string {
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return names
}

// checkChoices rejects a value outside a plain choice parameter's choices:
// Jenkins' buildWithParameters would run the build with it anyway. Only the
// core ChoiceParameterDefinition is checked; plugin types (Extended Choice,
// Active Choices) may expose static or stale choice lists.
func checkChoices(defs []ParamDef, given map[string]string) error {
	for _, d := range defs {
		v, ok := given[d.Name]
		if !ok || len(d.Choices) == 0 || d.Type != "ChoiceParameterDefinition" {
			continue
		}
		if !slices.Contains(d.Choices, v) {
			return errs.Usagef("invalid value %q for choice parameter %s", v, d.Name).
				WithHint("choices: " + strings.Join(d.Choices, ", ")).
				WithDetails(map[string]any{"parameter": d.Name, "choices": d.Choices})
		}
	}
	return nil
}

// afterTrigger marks an error raised after the build was already queued: it is
// never retryable (a retry would queue a second build) and carries the partial
// result so the caller can follow the queue item.
func afterTrigger(err error, out BuildTriggerOut) error {
	src := errs.From(err)
	e := *src
	e.Retryable = false
	e.Details = out
	e.Hint = fmt.Sprintf("build already queued as queue item %d; follow it with queue_get, do not re-trigger", out.QueueID)
	if e.Err == nil && src != err {
		e.Err = err
	}
	return &e
}

// waitBuild polls queue item then build. The deadline is checked against
// s.now (testable) and also enforced with a context timeout for real runs.
func (s *Service) waitBuild(ctx context.Context, path string, out BuildTriggerOut, timeout time.Duration) (BuildTriggerOut, error) {
	deadline := s.now().Add(timeout)
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// timedOut reports whether err (or the clock) means our own deadline,
	// as opposed to a failure or a cancelled parent context.
	timedOut := func(err error) bool {
		if ctx.Err() != nil {
			return false
		}
		if err != nil {
			return wctx.Err() != nil
		}
		return !s.now().Before(deadline)
	}
	pause := func() (bool, error) {
		if timedOut(nil) {
			return true, nil
		}
		if err := s.sleep(wctx, s.pollInterval); err != nil {
			if timedOut(err) {
				return true, nil
			}
			return false, errs.From(err)
		}
		return timedOut(nil), nil
	}

	start := s.now()
	report := func(msg string) {
		interact.Report(ctx, s.now().Sub(start).Seconds(), timeout.Seconds(), msg)
	}

	// Queue -> build number.
	for out.Number == nil {
		q, err := s.backend.GetQueueItem(wctx, out.QueueID)
		if err != nil {
			if timedOut(err) {
				out.TimedOut = true
				return out, nil
			}
			return out, err
		}
		if q.Cancelled {
			return out, errs.Newf(errs.CodeConflict, "queue item %d was cancelled", out.QueueID)
		}
		if q.BuildNumber != nil {
			out.Number, out.URL = q.BuildNumber, q.BuildURL
			out.Building = true
			break
		}
		why := q.Why
		if why == "" {
			why = "waiting in the queue"
		}
		report(fmt.Sprintf("queue item %d: %s", out.QueueID, why))
		if to, err := pause(); err != nil || to {
			out.TimedOut = to
			return out, err
		}
	}

	// Build -> result.
	ref := BuildRef(strconv.Itoa(*out.Number))
	for {
		b, err := s.backend.GetBuild(wctx, path, ref)
		switch {
		case err == nil:
			if b.URL != "" {
				out.URL = b.URL
			}
			out.Building, out.Result = b.Building, b.Result
			if !b.Building {
				return out, nil
			}
			report(fmt.Sprintf("build #%d running", *out.Number))
		case errs.Is(err, errs.CodeNotFound):
			// The build can 404 for a moment right after it leaves the queue.
		case timedOut(err):
			out.TimedOut = true
			return out, nil
		default:
			return out, err
		}
		if to, err := pause(); err != nil || to {
			out.TimedOut = to
			return out, err
		}
	}
}

func unknownParams(defs []ParamDef, given map[string]string) []string {
	known := make(map[string]bool, len(defs))
	for _, d := range defs {
		known[d.Name] = true
	}
	var unknown []string
	for k := range given {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	slices.Sort(unknown)
	return unknown
}

// BuildReplay re-runs a pipeline build with a replacement main script. The
// plan carries a diff against the build's original script; replay runs
// arbitrary pipeline code, so it needs confirm (like build_stop).
func (s *Service) BuildReplay(ctx context.Context, in BuildReplayIn) (BuildReplayOut, error) {
	path, ref, err := s.prepare(OpBuildReplay, in.Job, in.Ref, true)
	if err != nil {
		return BuildReplayOut{}, err
	}
	if strings.TrimSpace(in.Script) == "" {
		return BuildReplayOut{}, errs.Usage("script is empty").WithHint("pass the full Jenkinsfile to run; build_replay_script returns the original")
	}
	n, err := s.number(ctx, path, ref)
	if err != nil {
		return BuildReplayOut{}, err
	}
	plan := &Plan{
		Op: OpBuildReplay, Method: "POST",
		Path:   fmt.Sprintf("%s%d/replay/run", JobURLPath(path), n),
		Number: n,
		Params: map[string]string{"mainScript": fmt.Sprintf("<%d bytes>", len(in.Script))},
		Note:   "only the main script is replaced; loaded library/scripts stay as in the original build",
	}
	orig, err := s.backend.GetReplayScript(ctx, path, n)
	switch {
	case err == nil:
		d := unifiedDiff(orig.Main, in.Script, fmt.Sprintf("original #%d", n), "replay", 3, maxDiffLines)
		plan.Diff, plan.DiffStats = d.text, &DiffStats{Added: d.added, Removed: d.removed, Truncated: d.truncated}
		if d.added == 0 && d.removed == 0 {
			plan.Note = "the script is identical to the original; replay would re-run the same pipeline"
		}
	case errs.Is(err, errs.CodeNotFound):
		// The replay form is missing, so replay/run would fail as well.
		return BuildReplayOut{}, err
	default:
		plan.Note += "; original script unavailable for diff: " + errs.From(err).Message
	}
	out := BuildReplayOut{Job: path, FromNumber: n}
	if in.DryRun {
		out.DryRun, out.Plan = true, plan
		return out, nil
	}
	needs := errs.Newf(errs.CodeNeedsConfirmation, "replaying build %d of %q with a new script needs confirmation", n, path).
		WithHint(fmt.Sprintf("show the diff to the user; re-run with confirm=true (CLI: --yes) and ref=%q", strconv.Itoa(n))).
		WithDetails(plan)
	if err := approve(ctx, in.Confirm, plan, fmt.Sprintf("Replay build #%d of %s with a modified Jenkinsfile?", n, path), needs); err != nil {
		return BuildReplayOut{}, err
	}
	qid, err := s.backend.ReplayBuild(ctx, path, n, in.Script)
	if err != nil {
		return BuildReplayOut{}, err
	}
	out.QueueID = qid
	return out, nil
}

// BuildReplayScript returns the main script (and loaded script names) a build
// ran, read from its replay form.
func (s *Service) BuildReplayScript(ctx context.Context, in BuildReplayScriptIn) (BuildReplayScriptOut, error) {
	path, ref, err := s.prepare(OpReplayScript, in.Job, in.Ref, false)
	if err != nil {
		return BuildReplayScriptOut{}, err
	}
	n, err := s.number(ctx, path, ref)
	if err != nil {
		return BuildReplayScriptOut{}, err
	}
	rs, err := s.backend.GetReplayScript(ctx, path, n)
	if err != nil {
		return BuildReplayScriptOut{}, err
	}
	out := BuildReplayScriptOut{Job: path, Number: n, MainScript: rs.Main, Bytes: len(rs.Main), Lines: countLines(rs.Main)}
	for _, name := range slices.Sorted(maps.Keys(rs.Loaded)) {
		out.LoadedScripts = append(out.LoadedScripts, LoadedScript{Name: name, Bytes: len(rs.Loaded[name])})
	}
	if len(rs.Main) > SpillThreshold {
		p, err := s.spill(rs.Main, spillPrefix(OpReplayScript, n))
		if err != nil {
			return BuildReplayScriptOut{}, err
		}
		out.SpilledTo, out.SpilledURI, out.MainScript = p, output.SpillURI(p), ""
	}
	return out, nil
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(s, "\n"), "\n") + 1
}

// approveDiffLines caps the diff shown in a confirmation prompt.
const approveDiffLines = 80

// approve settles the confirmation of a danger op. When the caller can reach
// the human (MCP elicitation), the human is always asked, even if the caller
// passed confirm=true: an agent must not approve its own action. If the
// prompt cannot be shown, or there is no way to ask, confirm must be true;
// otherwise needs (NEEDS_CONFIRMATION with the plan) is returned.
func approve(ctx context.Context, confirm bool, plan *Plan, question string, needs *errs.Error) error {
	if c, ok := interact.ConfirmerFrom(ctx); ok {
		yes, err := c.Confirm(ctx, confirmMessage(question, plan))
		switch {
		case errors.Is(err, interact.ErrPending):
			return err // the transport asks the human and calls again
		case err == nil && yes:
			return nil
		case err == nil:
			return errs.Newf(errs.CodePolicyDenied, "the user declined: %s %s", plan.Method, plan.Path).
				WithHint("do not retry; ask the user what to change")
		}
		// No answer (e.g. the client failed to render the prompt): fall through
		// to the confirm flag, as for clients without elicitation.
	}
	if !confirm {
		return needs
	}
	return nil
}

// confirmMessage renders the question and the plan for a human.
func confirmMessage(question string, plan *Plan) string {
	var b strings.Builder
	b.WriteString(question)
	fmt.Fprintf(&b, "\n\n%s %s", plan.Method, plan.Path)
	if plan.Diff != "" {
		lines := strings.Split(plan.Diff, "\n")
		if len(lines) > approveDiffLines {
			lines = append(lines[:approveDiffLines], fmt.Sprintf("... (%d more lines)", len(lines)-approveDiffLines))
		}
		b.WriteString("\n\n" + strings.Join(lines, "\n"))
	}
	if plan.Note != "" {
		b.WriteString("\n\nNote: " + plan.Note)
	}
	return b.String()
}

// BuildStop aborts a build. The ref is resolved to a number first so the plan
// and the confirmation hint name a fixed build.
func (s *Service) BuildStop(ctx context.Context, in BuildStopIn) (BuildStopOut, error) {
	path, ref, err := s.prepare(OpBuildStop, in.Job, in.Ref, true)
	if err != nil {
		return BuildStopOut{}, err
	}
	n, err := s.number(ctx, path, ref)
	if err != nil {
		return BuildStopOut{}, err
	}
	plan := &Plan{Op: OpBuildStop, Method: "POST", Path: fmt.Sprintf("%s%d/stop", JobURLPath(path), n), Number: n}
	out := BuildStopOut{Job: path, Number: n}
	if in.DryRun {
		out.DryRun, out.Plan = true, plan
		return out, nil
	}
	needs := errs.Newf(errs.CodeNeedsConfirmation, "stopping build %d of %q needs confirmation", n, path).
		WithHint(fmt.Sprintf("re-run with confirm=true (CLI: --yes) and ref=%q", strconv.Itoa(n))).
		WithDetails(plan)
	if err := approve(ctx, in.Confirm, plan, fmt.Sprintf("Stop build #%d of %s?", n, path), needs); err != nil {
		return BuildStopOut{}, err
	}
	if err := s.backend.StopBuild(ctx, path, n); err != nil {
		return BuildStopOut{}, err
	}
	out.Stopped = true
	return out, nil
}

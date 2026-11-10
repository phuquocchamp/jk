package core

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/phuquocchamp/jk/internal/kit/errs"
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

	plan := &Plan{Op: OpBuildTrigger, Method: "POST", Path: JobURLPath(path) + "build"}
	if parameterized {
		plan.Path = JobURLPath(path) + "buildWithParameters"
		if len(in.Params) > 0 {
			plan.Params = maps.Clone(in.Params)
		}
		if unknown := unknownParams(job.Parameters, in.Params); len(unknown) > 0 {
			plan.Note = "parameters not defined by the job (Jenkins ignores them): " + strings.Join(unknown, ", ")
		}
	}
	out := BuildTriggerOut{Job: path}
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
	timeout := DefaultTriggerTimeout
	if in.TimeoutSeconds > 0 {
		timeout = time.Duration(in.TimeoutSeconds) * time.Second
	}
	out, err = s.waitBuild(ctx, path, out, timeout)
	if err != nil {
		return out, afterTrigger(err, out)
	}
	return out, nil
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

// BuildReplay re-runs a pipeline build with a replacement main script.
func (s *Service) BuildReplay(ctx context.Context, in BuildReplayIn) (BuildReplayOut, error) {
	path, ref, err := s.prepare(OpBuildReplay, in.Job, in.Ref, true)
	if err != nil {
		return BuildReplayOut{}, err
	}
	if strings.TrimSpace(in.Script) == "" {
		return BuildReplayOut{}, errs.Usage("script is empty").WithHint("pass the full Jenkinsfile to run")
	}
	n, err := s.number(ctx, path, ref)
	if err != nil {
		return BuildReplayOut{}, err
	}
	out := BuildReplayOut{Job: path, FromNumber: n}
	if in.DryRun {
		out.DryRun = true
		out.Plan = &Plan{
			Op: OpBuildReplay, Method: "POST",
			Path:   fmt.Sprintf("%s%d/replay/run", JobURLPath(path), n),
			Number: n,
			Params: map[string]string{"mainScript": fmt.Sprintf("<%d bytes>", len(in.Script))},
			Note:   "only the main script is replaced; loaded library/scripts stay as in the original build",
		}
		return out, nil
	}
	qid, err := s.backend.ReplayBuild(ctx, path, n, in.Script)
	if err != nil {
		return BuildReplayOut{}, err
	}
	out.QueueID = qid
	return out, nil
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
	if !in.Confirm {
		return BuildStopOut{}, errs.Newf(errs.CodeNeedsConfirmation, "stopping build %d of %q needs confirmation", n, path).
			WithHint(fmt.Sprintf("re-run with confirm=true (CLI: --yes) and ref=%q", strconv.Itoa(n))).
			WithDetails(plan)
	}
	if err := s.backend.StopBuild(ctx, path, n); err != nil {
		return BuildStopOut{}, err
	}
	out.Stopped = true
	return out, nil
}

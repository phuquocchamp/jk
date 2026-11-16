package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

// Stage sources reported in BuildStagesOut.Source.
const (
	SourcePGV   = "pipeline-graph-view"
	SourceWfapi = "wfapi"
)

// maxWfapiLookups bounds the stage node calls of the wfapi fallback.
const maxWfapiLookups = 20

// stageView is the stage model of one build, with the steps when known.
type stageView struct {
	out   BuildStagesOut
	steps []GraphStep // nil with source=wfapi
}

// loadStages builds the stage list of build n. It prefers the Pipeline Graph
// View tree, whose states tell failed stages from stages skipped after a
// failure (wfapi reports both as FAILED) and which nests parallel branches.
// Without the plugin it falls back to wfapi.
func (s *Service) loadStages(ctx context.Context, path string, n int, stepErrors bool) (stageView, error) {
	// The tree and the step list are independent requests: fetch them together.
	var (
		steps    []GraphStep
		stepsErr error
		wg       sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		steps, stepsErr = s.backend.GetSteps(ctx, path, n)
	}()
	tree, err := s.backend.GetStageTree(ctx, path, n)
	wg.Wait()
	switch {
	case err == nil:
		if stepsErr != nil {
			steps = nil // optional: stages still carry the right status
		}
		return s.stagesFromGraph(ctx, path, n, tree, steps, stepErrors), nil
	case errs.Is(err, errs.CodeNotFound), errs.Is(err, errs.CodeForbidden):
		return s.stagesFromWfapi(ctx, path, n)
	default:
		return stageView{}, err
	}
}

func (s *Service) stagesFromGraph(ctx context.Context, path string, n int, tree []GraphStage, steps []GraphStep, stepErrors bool) stageView {
	out := BuildStagesOut{Job: path, Number: n, Source: SourcePGV, Stages: []Stage{}}
	var walk func(gs []GraphStage, parent string)
	walk = func(gs []GraphStage, parent string) {
		for _, g := range gs {
			out.Stages = append(out.Stages, Stage{
				ID: g.ID, Name: g.Name, Type: strings.ToLower(g.Type), ParentID: parent,
				Status: graphStatus(g.State), StartTime: g.StartTime, DurationMs: g.DurationMs,
			})
			walk(g.Children, g.ID)
		}
	}
	walk(tree, "")

	failedStep := map[string]GraphStep{}
	for _, st := range steps {
		if graphStatus(st.State) == "FAILED" {
			if _, seen := failedStep[st.StageID]; !seen {
				failedStep[st.StageID] = st
			}
		}
	}
	for i := range out.Stages {
		st := &out.Stages[i]
		if fs, ok := failedStep[st.ID]; ok && st.Status == "FAILED" {
			st.FailedStep = &FailedStep{ID: fs.ID, Name: fs.Name}
		}
	}
	markInherited(out.Stages)
	out.FirstFailed = firstFailed(out.Stages)
	// The step error is nearly always "script returned exit code N", so only
	// the first failure gets one (one request instead of one per failed stage).
	if stepErrors && out.FirstFailed != "" {
		i := slices.IndexFunc(out.Stages, func(st Stage) bool { return st.ID == out.FirstFailed })
		if fs := out.Stages[i].FailedStep; fs != nil {
			if msg, err := s.backend.GetStepError(ctx, path, n, fs.ID); err == nil && strings.TrimSpace(msg) != "" {
				e := &StageError{Message: firstLine(msg)}
				fs.Error, out.Stages[i].Error = e, e
			}
		}
	}
	out.Status, out.DurationMs = overallStatus(out.Stages)
	return stageView{out: out, steps: steps}
}

// markInherited flags FAILED stages that have no failing step of their own but
// contain a failed stage: they failed only because a nested stage did.
func markInherited(stages []Stage) {
	hasFailedChild := map[string]bool{}
	for _, st := range stages {
		if st.Status == "FAILED" && st.ParentID != "" {
			hasFailedChild[st.ParentID] = true
		}
	}
	for i := range stages {
		st := &stages[i]
		if st.Status == "FAILED" && st.FailedStep == nil && hasFailedChild[st.ID] {
			st.Inherited = true
		}
	}
}

func (s *Service) stagesFromWfapi(ctx context.Context, path string, n int) (stageView, error) {
	run, err := s.backend.GetRun(ctx, path, n)
	if err != nil {
		return stageView{}, err
	}
	stages := run.Stages
	if stages == nil {
		stages = []Stage{}
	}
	out := BuildStagesOut{Job: path, Number: n, Status: run.Status, DurationMs: run.DurationMs, Source: SourceWfapi, Stages: stages}
	// wfapi marks stages skipped after a failure as FAILED too. A real failure
	// has a FAILED flow node; a skipped stage has none (only bookkeeping steps).
	// One call per FAILED stage, in start order, bounded by maxWfapiLookups.
	failed := make([]int, 0)
	for i, st := range out.Stages {
		if st.Status == "FAILED" {
			failed = append(failed, i)
		}
	}
	slices.SortStableFunc(failed, func(a, b int) int { return out.Stages[a].StartTime.Compare(out.Stages[b].StartTime) })
	sawReal := false
	for k, i := range failed {
		if k == maxWfapiLookups {
			break
		}
		st := &out.Stages[i]
		nodes, err := s.backend.GetStageNodes(ctx, path, n, st.ID)
		if err != nil {
			continue
		}
		var step *FailedStep
		allOK := len(nodes) > 0
		for _, nd := range nodes {
			allOK = allOK && nd.Status == "SUCCESS"
		}
		for _, nd := range nodes {
			if nd.Status != "FAILED" {
				continue
			}
			name := nd.Command
			if name == "" {
				name = nd.Name
			}
			step = &FailedStep{ID: nd.ID, Name: name, Error: nd.Error}
			break
		}
		switch {
		case step != nil:
			st.FailedStep, sawReal = step, true
		case sawReal && allOK:
			// Only after a real failure, and only when every node succeeded
			// (the bookkeeping steps of a skipped stage). A lone FAILED stage,
			// or one with an ABORTED/UNSTABLE node, keeps the reported status.
			st.Status, st.SkippedAfterFailure = "SKIPPED", true
		}
	}
	out.FirstFailed = firstFailed(out.Stages)
	return stageView{out: out}, nil
}

// firstFailed picks the stage where the build first failed: the earliest
// started FAILED stage that has a failing step, else the earliest FAILED stage
// that has no FAILED child (a parent only inherits its child's failure).
func firstFailed(stages []Stage) string {
	pick := func(ok func(Stage) bool) string {
		best := -1
		for i, st := range stages {
			if st.Status != "FAILED" || !ok(st) {
				continue
			}
			if best < 0 || st.StartTime.Before(stages[best].StartTime) {
				best = i
			}
		}
		if best < 0 {
			return ""
		}
		return stages[best].ID
	}
	if id := pick(func(st Stage) bool { return st.FailedStep != nil }); id != "" {
		return id
	}
	failedParent := map[string]bool{}
	for _, st := range stages {
		if st.Status == "FAILED" && st.ParentID != "" {
			failedParent[st.ParentID] = true
		}
	}
	return pick(func(st Stage) bool { return !failedParent[st.ID] })
}

// overallStatus derives a run status from top-level stages (the graph tree
// has no run-level status) and sums their wall-clock span.
func overallStatus(stages []Stage) (string, int64) {
	rank := map[string]int{"IN_PROGRESS": 6, "FAILED": 5, "ABORTED": 4, "UNSTABLE": 3, "PAUSED_PENDING_INPUT": 2, "SUCCESS": 1}
	status, best := "", 0
	var start, end int64
	for _, st := range stages {
		if st.ParentID != "" {
			continue
		}
		if r := rank[st.Status]; r > best {
			status, best = st.Status, r
		}
		if t := st.StartTime.UnixMilli(); !st.StartTime.IsZero() {
			if start == 0 || t < start {
				start = t
			}
			if e := t + st.DurationMs; e > end {
				end = e
			}
		}
	}
	if status == "" && len(stages) > 0 {
		status = "NOT_EXECUTED"
	}
	return status, max(0, end-start)
}

// graphStatus maps a Pipeline Graph View state to the wfapi-style status.
func graphStatus(state string) string {
	switch strings.ToLower(state) {
	case "success":
		return "SUCCESS"
	case "failure":
		return "FAILED"
	case "unstable":
		return "UNSTABLE"
	case "aborted":
		return "ABORTED"
	case "skipped":
		return "SKIPPED"
	case "running":
		return "IN_PROGRESS"
	case "paused":
		return "PAUSED_PENDING_INPUT"
	case "not_built":
		return "NOT_EXECUTED"
	case "queued":
		return "QUEUED"
	}
	return strings.ToUpper(state)
}

// selectStage finds a stage by exact id, then by case-insensitive name. Several
// stages with that name (e.g. in matrix or parallel legs) is a USAGE error.
func selectStage(stages []Stage, sel string, n int) (Stage, error) {
	for _, st := range stages {
		if st.ID == sel {
			return st, nil
		}
	}
	var hits []Stage
	for _, st := range stages {
		if strings.EqualFold(st.Name, sel) {
			hits = append(hits, st)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		names := make([]string, 0, len(stages))
		for _, x := range stages {
			names = append(names, x.Name)
		}
		e := errs.NotFound(fmt.Sprintf("stage %q not found in build %d", sel, n))
		if len(names) > 0 {
			return Stage{}, e.WithHint("stages: " + strings.Join(names, ", "))
		}
		return Stage{}, e.WithHint("the build has no pipeline stages; omit stage to read the console log")
	}
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.ID)
	}
	return Stage{}, errs.Usagef("%d stages are named %q in build %d", len(hits), sel, n).
		WithHint("pass the stage id instead: " + strings.Join(ids, ", "))
}

// descendants returns id and the ids of all stages nested below it.
func descendants(stages []Stage, id string) map[string]bool {
	set := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for _, st := range stages {
			if !set[st.ID] && set[st.ParentID] {
				set[st.ID], changed = true, true
			}
		}
	}
	return set
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return trimText(s, ErrorDetailsMaxChars)
}

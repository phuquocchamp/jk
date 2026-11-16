package core

import (
	"context"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/interact"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

var t0 = time.Unix(1_700_000_000, 0).UTC()

func at(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }

// graphBuild mirrors a real Pipeline Graph View response: a parallel block
// where one leg fails and the stages after it are skipped (wfapi would
// report all of them FAILED).
func graphBuild() *fakeBackend {
	return &fakeBackend{
		stageTree: func(string, int) ([]GraphStage, error) {
			return []GraphStage{
				{ID: "11", Name: "Check", Type: "STAGE", State: "success", StartTime: at(0), DurationMs: 1000},
				{ID: "19", Name: "Parallel", Type: "STAGE", State: "failure", StartTime: at(1), DurationMs: 9000, Children: []GraphStage{
					{ID: "24", Name: "Linux", Type: "PARALLEL", State: "success", StartTime: at(1), Children: []GraphStage{
						{ID: "104", Name: "Linux: Build", Type: "STAGE", State: "success", StartTime: at(2)},
					}},
					{ID: "25", Name: "Mac", Type: "PARALLEL", State: "failure", StartTime: at(1), Children: []GraphStage{
						{ID: "134", Name: "Mac: Test", Type: "STAGE", State: "success", StartTime: at(2)},
						{ID: "407", Name: "Mac: Images", Type: "STAGE", State: "failure", StartTime: at(5)},
						{ID: "439", Name: "Mac: App", Type: "STAGE", State: "skipped", StartTime: at(6)},
					}},
				}},
				{ID: "583", Name: "Merge", Type: "STAGE", State: "skipped", StartTime: at(10)},
			}, nil
		},
		steps: func(string, int) ([]GraphStep, error) {
			return []GraphStep{
				{ID: "105", Name: "make linux", StageID: "104", State: "success"},
				{ID: "135", Name: "npm test", StageID: "134", State: "success"},
				{ID: "419", Name: "docker login", StageID: "407", State: "success"},
				{ID: "425", Name: "npm run images:build", StageID: "407", State: "failure"},
				{ID: "429", Name: "docker logout", StageID: "407", State: "success"},
			}, nil
		},
		stepError: func(_ string, _ int, id string) (string, error) {
			if id == "425" {
				return "script returned exit code 1\n\tat ...", nil
			}
			return "", nil
		},
		nodeLog: func(_ string, _ int, id string) (string, error) {
			switch id {
			case "419":
				return "[2026-11-15T17:14:50.000Z] Login Succeeded\n", nil
			case "425":
				return "[2026-11-15T17:14:55.100Z] \x1b[31mbuilding images\x1b[0m\r\n[2026-11-15T17:14:57.309Z] exited with 1: images:build\n", nil
			case "429":
				return "Removing login credentials\n", nil
			case "135":
				return "tests ok\n", nil
			}
			return "", errs.NotFound("no node " + id)
		},
	}
}

func TestBuildStagesFromGraph(t *testing.T) {
	fb := graphBuild()
	out, err := newSvc(fb, policy.Policy{}).BuildStages(context.Background(), BuildStagesIn{Job: "app", Ref: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != SourcePGV || out.FirstFailed != "407" || out.Status != "FAILED" {
		t.Fatalf("got source=%s first=%s status=%s", out.Source, out.FirstFailed, out.Status)
	}
	byID := map[string]Stage{}
	var order []string
	for _, st := range out.Stages {
		byID[st.ID], order = st, append(order, st.ID)
	}
	if want := []string{"11", "19", "24", "104", "25", "134", "407", "439", "583"}; !slices.Equal(order, want) {
		t.Fatalf("order %v", order)
	}
	if st := byID["439"]; st.Status != "SKIPPED" || st.ParentID != "25" || st.FailedStep != nil {
		t.Fatalf("skipped stage %+v", st)
	}
	if st := byID["25"]; st.Type != "parallel" || st.ParentID != "19" || !st.Inherited {
		t.Fatalf("parallel leg %+v", st)
	}
	if !byID["19"].Inherited || byID["407"].Inherited {
		t.Fatalf("inherited: 19=%v 407=%v", byID["19"].Inherited, byID["407"].Inherited)
	}
	fs := byID["407"].FailedStep
	if fs == nil || fs.ID != "425" || fs.Name != "npm run images:build" || fs.Error == nil || fs.Error.Message != "script returned exit code 1" {
		t.Fatalf("failed step %+v", fs)
	}
	if fb.called("GetRun") {
		t.Fatal("wfapi called although the graph was available")
	}
}

func TestBuildStagesWfapiFallback(t *testing.T) {
	fb := &fakeBackend{
		getRun: func(string, int) (Run, error) {
			return Run{Status: "FAILED", Stages: []Stage{
				{ID: "6", Name: "Build", Status: "SUCCESS", StartTime: at(0)},
				{ID: "17", Name: "Test", Status: "FAILED", StartTime: at(5), Error: &StageError{Message: "script returned exit code 1"}},
				{ID: "30", Name: "Deploy", Status: "FAILED", StartTime: at(9)},
			}}, nil
		},
		getStageNodes: func(_ string, _ int, id string) ([]StageNode, error) {
			if id == "30" {
				// Skipped after the failure: only a bookkeeping step.
				return []StageNode{{ID: "31", Name: "Get contextual object from internal APIs", Status: "SUCCESS"}}, nil
			}
			return []StageNode{
				{ID: "18", Name: "Shell Script", Status: "SUCCESS", Command: "make"},
				{ID: "20", Name: "Shell Script", Status: "FAILED", Command: "make test", Error: &StageError{Type: "hudson.AbortException", Message: "script returned exit code 2"}},
			}, nil
		},
	}
	out, err := newSvc(fb, policy.Policy{}).BuildStages(context.Background(), BuildStagesIn{Job: "app", Ref: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != SourceWfapi || out.FirstFailed != "17" {
		t.Fatalf("got %+v", out)
	}
	fs := out.Stages[1].FailedStep
	if fs == nil || fs.Name != "make test" || fs.Error.Message != "script returned exit code 2" || out.Stages[2].FailedStep != nil {
		t.Fatalf("failed step %+v", out.Stages)
	}
	if d := out.Stages[2]; d.Status != "SKIPPED" || !d.SkippedAfterFailure {
		t.Fatalf("deploy %+v", d)
	}
}

func TestBuildStagesWfapiLoneFailureKeepsStatus(t *testing.T) {
	fb := &fakeBackend{
		getRun: func(string, int) (Run, error) {
			return Run{Stages: []Stage{{ID: "9", Name: "Gate", Status: "FAILED", StartTime: at(0)}}}, nil
		},
		getStageNodes: func(string, int, string) ([]StageNode, error) { return nil, nil },
	}
	out, err := newSvc(fb, policy.Policy{}).BuildStages(context.Background(), BuildStagesIn{Job: "app", Ref: "1"})
	if err != nil || out.Stages[0].Status != "FAILED" || out.Stages[0].SkippedAfterFailure || out.FirstFailed != "9" {
		t.Fatalf("got %+v, %v", out, err)
	}

	// After a real failure, a stage with an ABORTED node (e.g. a timeout) is not relabelled.
	fb = &fakeBackend{
		getRun: func(string, int) (Run, error) {
			return Run{Stages: []Stage{
				{ID: "5", Name: "Test", Status: "FAILED", StartTime: at(0)},
				{ID: "8", Name: "Slow", Status: "FAILED", StartTime: at(3)},
			}}, nil
		},
		getStageNodes: func(_ string, _ int, id string) ([]StageNode, error) {
			if id == "5" {
				return []StageNode{{ID: "6", Status: "FAILED", Command: "make test"}}, nil
			}
			return []StageNode{{ID: "9", Status: "ABORTED", Command: "sleep 3600"}}, nil
		},
	}
	out, err = newSvc(fb, policy.Policy{}).BuildStages(context.Background(), BuildStagesIn{Job: "app", Ref: "1"})
	if err != nil || out.Stages[1].Status != "FAILED" || out.Stages[1].SkippedAfterFailure {
		t.Fatalf("aborted node: %+v, %v", out.Stages, err)
	}
}

func TestBuildLogStageFromGraphSteps(t *testing.T) {
	fb := graphBuild()
	s := newSvc(fb, policy.Policy{})
	ctx := context.Background()

	out, err := s.BuildLog(ctx, BuildLogIn{Job: "app", Ref: "1", Stage: "mac: images"})
	if err != nil {
		t.Fatal(err)
	}
	want := "Login Succeeded\nbuilding images\nexited with 1: images:build\nRemoving login credentials"
	if out.Text != want || out.Stage != "Mac: Images" || out.Source != "steps" || out.TotalLines != 4 {
		t.Fatalf("got %+v", out)
	}
	if !slices.Equal(out.Stripped, []string{"ansi", "timestamps"}) {
		t.Fatalf("stripped %v", out.Stripped)
	}
	if fb.called("GetStepError") || fb.called("GetStageNodes") {
		t.Fatal("log read made stage-error or wfapi calls")
	}

	// A parallel leg includes the steps of its nested stages.
	out, err = s.BuildLog(ctx, BuildLogIn{Job: "app", Ref: "1", Stage: "25", Lines: 2})
	if err != nil || out.Text != "exited with 1: images:build\nRemoving login credentials" || out.TotalLines != 5 || !out.Truncated {
		t.Fatalf("leg: %+v, %v", out, err)
	}

	// raw keeps ANSI codes and timestamps.
	out, err = s.BuildLog(ctx, BuildLogIn{Job: "app", Ref: "1", Stage: "407", Raw: true, Lines: 2})
	if err != nil || !strings.HasPrefix(out.Text, "[2026-11-15T17:14:57.309Z] ") || out.Stripped != nil {
		t.Fatalf("raw: %+v, %v", out, err)
	}
}

func TestSelectStageDuplicateNames(t *testing.T) {
	stages := []Stage{{ID: "5", Name: "Test"}, {ID: "9", Name: "test"}}
	_, err := selectStage(stages, "Test", 3)
	e := wantCode(t, err, errs.CodeUsage)
	if !strings.Contains(e.Hint, "5, 9") {
		t.Fatalf("hint %q", e.Hint)
	}
	if st, err := selectStage(stages, "9", 3); err != nil || st.ID != "9" {
		t.Fatalf("by id: %+v %v", st, err)
	}
}

// rawConsole has console notes, ANSI and timestamps like progressiveText.
func rawConsole(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("[2026-11-15T17:00:00Z] \x1b[8mha:////4AB0YgQ==\x1b[0mline ")
		b.WriteString(strings.Repeat("x", 40))
		b.WriteString(" ")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("\n")
	}
	return b.String()
}

func TestBuildLogTailFromOffset(t *testing.T) {
	log := rawConsole(20000) // ~1.6MB
	var starts []int64
	fb := &fakeBackend{
		consoleSize: func(string, int) (int64, error) { return int64(len(log)), nil },
		consoleFrom: func(_ string, _ int, start int64) (io.ReadCloser, error) {
			starts = append(starts, start)
			return io.NopCloser(strings.NewReader(log[start:])), nil
		},
	}
	out, err := newSvc(fb, policy.Policy{}).BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "1", Lines: 3})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out.Text, "\n")
	if len(lines) != 3 || strings.Contains(out.Text, "ha:") || strings.Contains(out.Text, "\x1b") || strings.Contains(out.Text, "[2026") {
		t.Fatalf("text %q", out.Text)
	}
	if !strings.HasPrefix(lines[0], "line ") {
		t.Fatalf("first line not clean: %q", lines[0])
	}
	if out.Source != "progressiveText" || out.TotalLines != 0 || out.TotalBytes != int64(len(log)) || out.ReadBytes == 0 || out.ReadBytes >= out.TotalBytes || !out.Truncated {
		t.Fatalf("got %+v", out)
	}
	if fb.called("StreamConsole ") {
		t.Fatal("full console streamed")
	}
	if len(starts) != 1 || starts[0] <= 0 {
		t.Fatalf("starts %v", starts)
	}
}

func TestBuildLogTailWidensWindow(t *testing.T) {
	// Few, very long lines: the first window holds fewer lines than asked.
	log := strings.Repeat(strings.Repeat("y", 60_000)+"\n", 6)
	var starts []int64
	fb := &fakeBackend{
		consoleSize: func(string, int) (int64, error) { return int64(len(log)), nil },
		consoleFrom: func(_ string, _ int, start int64) (io.ReadCloser, error) {
			starts = append(starts, start)
			return io.NopCloser(strings.NewReader(log[start:])), nil
		},
	}
	out, err := newSvc(fb, policy.Policy{}).BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "1", Lines: 4})
	if err != nil || out.Lines != 4 {
		t.Fatalf("got %+v, %v", out.Lines, err)
	}
	if len(starts) < 2 || starts[1] >= starts[0] {
		t.Fatalf("window did not widen: %v", starts)
	}
	// The window reached the start: the whole log was read, so the count is total.
	if starts[len(starts)-1] == 0 && out.TotalLines != 6 {
		t.Fatalf("total %d", out.TotalLines)
	}
}

func TestBuildLogOffsetFallsBackToConsoleText(t *testing.T) {
	fb := &fakeBackend{
		consoleSize: func(string, int) (int64, error) { return 100, nil },
		consoleFrom: func(string, int, int64) (io.ReadCloser, error) { return nil, errs.NotFound("old Jenkins") },
		console:     consoleOf("a\nb\nc\n"),
	}
	out, err := newSvc(fb, policy.Policy{}).BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "1", Lines: 2})
	if err != nil || out.Text != "b\nc" || out.Source != "consoleText" || out.TotalLines != 3 {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestLogSearchStripsByDefault(t *testing.T) {
	fb := &fakeBackend{console: consoleOf("[2026-11-15T17:00:00.1Z] \x1b[1mERROR\x1b[0m boom\nok\n")}
	s := newSvc(fb, policy.Policy{})
	out, err := s.LogSearch(context.Background(), LogSearchIn{Job: "app", Ref: "1", Pattern: "^ERROR boom$"})
	if err != nil || len(out.Matches) != 1 || out.Matches[0].Text != "ERROR boom" {
		t.Fatalf("got %+v, %v", out, err)
	}
	out, err = s.LogSearch(context.Background(), LogSearchIn{Job: "app", Ref: "1", Pattern: "^ERROR boom$", Raw: true})
	if err != nil || len(out.Matches) != 0 {
		t.Fatalf("raw: %+v, %v", out, err)
	}
}

func choiceJob(string) (Job, error) {
	return Job{Parameters: []ParamDef{
		{Name: "ENV", Type: "ChoiceParameterDefinition", Choices: []string{"dev", "staging"}},
		{Name: "SKIP_TESTS", Type: "BooleanParameterDefinition"},
	}}, nil
}

func TestBuildTriggerRejectsUnknownParams(t *testing.T) {
	fb := &fakeBackend{getJob: choiceJob}
	s := newSvc(fb, policy.Policy{Tier: policy.TierWrite})
	_, err := s.BuildTrigger(context.Background(), BuildTriggerIn{Job: "app", Params: map[string]string{"ENVV": "staging"}})
	e := wantCode(t, err, errs.CodeUsage)
	d, _ := e.Details.(map[string]any)
	if !strings.Contains(e.Hint, "ENV, SKIP_TESTS") || !slices.Equal(d["unknown"].([]string), []string{"ENVV"}) {
		t.Fatalf("got %+v", e)
	}
	if fb.called("TriggerBuild") {
		t.Fatal("triggered with a typo")
	}

	// allow_unknown sends them and warns.
	fb.trigger = func(string, map[string]string, bool) (int64, error) { return 9, nil }
	out, err := s.BuildTrigger(context.Background(), BuildTriggerIn{Job: "app", Params: map[string]string{"ENVV": "x"}, AllowUnknown: true})
	if err != nil || out.QueueID != 9 || len(out.Warnings) != 1 {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestBuildTriggerRejectsBadChoice(t *testing.T) {
	fb := &fakeBackend{getJob: choiceJob}
	_, err := newSvc(fb, policy.Policy{Tier: policy.TierWrite}).BuildTrigger(context.Background(),
		BuildTriggerIn{Job: "app", Params: map[string]string{"ENV": "prod"}, DryRun: true})
	e := wantCode(t, err, errs.CodeUsage)
	if !strings.Contains(e.Hint, "dev, staging") {
		t.Fatalf("hint %q", e.Hint)
	}

	// Plugin choice types are not validated: their lists may be dynamic.
	fb = &fakeBackend{getJob: func(string) (Job, error) {
		return Job{Parameters: []ParamDef{{Name: "BRANCH", Type: "CascadeChoiceParameter", Choices: []string{"main"}}}}, nil
	}}
	if _, err := newSvc(fb, policy.Policy{Tier: policy.TierWrite}).BuildTrigger(context.Background(),
		BuildTriggerIn{Job: "app", Params: map[string]string{"BRANCH": "feature/x"}, DryRun: true}); err != nil {
		t.Fatalf("plugin choice rejected: %v", err)
	}
}

func TestBuildTriggerWaitCappedForMCP(t *testing.T) {
	fb := &fakeBackend{
		getJob:    func(string) (Job, error) { return Job{}, nil },
		trigger:   func(string, map[string]string, bool) (int64, error) { return 5, nil },
		queueItem: func(id int64) (QueueItem, error) { return QueueItem{ID: id, BuildNumber: intp(3)}, nil },
		getBuild:  func(string, BuildRef) (Build, error) { return Build{Number: 3, Building: true}, nil },
	}
	clk := &fakeClock{now: t0}
	s := newSvc(fb, policy.Policy{Tier: policy.TierWrite}, WithClock(clk.Now), WithSleep(clk.Sleep), WithPollInterval(5*time.Second))
	s.SetMaxWait(DefaultMCPMaxWait)
	out, err := s.BuildTrigger(context.Background(), BuildTriggerIn{Job: "team/app", Wait: true, TimeoutSeconds: 600})
	if err != nil || !out.TimedOut {
		t.Fatalf("got %+v, %v", out, err)
	}
	if waited := clk.Now().Sub(t0); waited > DefaultMCPMaxWait+5*time.Second {
		t.Fatalf("waited %s", waited)
	}
	if out.Next == nil || out.Next.Tool != "jenkins_build_wait" || out.Next.Args["ref"] != 3 || out.Next.CLI != "jk build wait team/app 3" {
		t.Fatalf("next %+v", out.Next)
	}
}

func TestBuildWait(t *testing.T) {
	polls := 0
	fb := &fakeBackend{
		queueItem: func(id int64) (QueueItem, error) {
			polls++
			if polls < 2 {
				return QueueItem{ID: id}, nil
			}
			return QueueItem{ID: id, BuildNumber: intp(7)}, nil
		},
		getBuild: func(_ string, ref BuildRef) (Build, error) {
			return Build{Number: 7, Result: "FAILURE", URL: "u"}, nil
		},
	}
	clk := &fakeClock{now: t0}
	s := newSvc(fb, policy.Policy{}, WithClock(clk.Now), WithSleep(clk.Sleep))
	ctx := context.Background()

	out, err := s.BuildWait(ctx, BuildWaitIn{Job: "app", QueueID: 44})
	if err != nil || out.Number == nil || *out.Number != 7 || out.Result != "FAILURE" || out.TimedOut || out.QueueID != 44 {
		t.Fatalf("by queue: %+v, %v", out, err)
	}
	out, err = s.BuildWait(ctx, BuildWaitIn{Job: "app", Ref: "7"})
	if err != nil || out.Result != "FAILURE" || fb.called("ResolveBuild") {
		t.Fatalf("by ref: %+v, %v", out, err)
	}
	_, err = s.BuildWait(ctx, BuildWaitIn{Job: "app"})
	wantCode(t, err, errs.CodeUsage)
}

func TestBuildReplayNeedsConfirmWithDiff(t *testing.T) {
	orig := "pipeline {\n  stages {\n    stage('Test') {\n      sh 'npm test'\n    }\n  }\n}\n"
	edited := strings.Replace(orig, "npm test", "npm test -- order", 1)
	fb := &fakeBackend{
		replayScript: func(string, int) (ReplayScript, error) { return ReplayScript{Main: orig}, nil },
		replay:       func(string, int, string) (int64, error) { return 3, nil },
	}
	s := newSvc(fb, policy.Policy{Tier: policy.TierDanger})
	ctx := context.Background()

	_, err := s.BuildReplay(ctx, BuildReplayIn{Job: "app", Ref: "57", Script: edited})
	e := wantCode(t, err, errs.CodeNeedsConfirmation)
	plan := e.Details.(*Plan)
	if plan.Number != 57 || plan.DiffStats == nil || plan.DiffStats.Added != 1 || plan.DiffStats.Removed != 1 {
		t.Fatalf("plan %+v", plan)
	}
	if !strings.Contains(plan.Diff, "-      sh 'npm test'\n+      sh 'npm test -- order'") {
		t.Fatalf("diff %q", plan.Diff)
	}
	if fb.called("ReplayBuild") {
		t.Fatal("replayed without confirm")
	}
	out, err := s.BuildReplay(ctx, BuildReplayIn{Job: "app", Ref: "57", Script: edited, Confirm: true})
	if err != nil || out.QueueID != 3 {
		t.Fatalf("got %+v, %v", out, err)
	}

	// Write tier no longer allows replay.
	_, err = newSvc(fb, policy.Policy{Tier: policy.TierWrite}).BuildReplay(ctx, BuildReplayIn{Job: "app", Ref: "57", Script: edited, DryRun: true})
	wantCode(t, err, errs.CodePolicyDenied)
}

func TestBuildReplayUnavailable(t *testing.T) {
	fb := &fakeBackend{replayScript: func(string, int) (ReplayScript, error) {
		return ReplayScript{}, errs.NotFound("replay: HTTP 404").WithHint("replay is not available for this build")
	}}
	_, err := newSvc(fb, policy.Policy{Tier: policy.TierDanger}).BuildReplay(context.Background(),
		BuildReplayIn{Job: "app", Ref: "1", Script: "x", DryRun: true})
	wantCode(t, err, errs.CodeNotFound)
}

func TestBuildReplayScript(t *testing.T) {
	fb := &fakeBackend{
		resolveBuild: func(string, BuildRef) (int, error) { return 12, nil },
		replayScript: func(string, int) (ReplayScript, error) {
			return ReplayScript{Main: "a\nb\n", Loaded: map[string]string{"Script2": "yy", "Script1": "x"}}, nil
		},
	}
	out, err := newSvc(fb, policy.Policy{}).BuildReplayScript(context.Background(), BuildReplayScriptIn{Job: "app"})
	if err != nil || out.Number != 12 || out.MainScript != "a\nb\n" || out.Lines != 2 || out.Bytes != 4 {
		t.Fatalf("got %+v, %v", out, err)
	}
	if len(out.LoadedScripts) != 2 || out.LoadedScripts[0].Name != "Script1" || out.LoadedScripts[1].Bytes != 2 {
		t.Fatalf("loaded %+v", out.LoadedScripts)
	}
}

func TestUnifiedDiff(t *testing.T) {
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n"
	b := "1\n2\n3\nfour\n5\n6\n7\n8\n9\n10\n11\n12\n13\n"
	d := unifiedDiff(a, b, "a", "b", 2, 100)
	want := "--- a\n+++ b\n@@ -2,5 +2,5 @@\n 2\n 3\n-4\n+four\n 5\n 6\n@@ -11,2 +11,3 @@\n 11\n 12\n+13"
	if d.text != want || d.added != 2 || d.removed != 1 {
		t.Fatalf("diff:\n%s\n(+%d -%d)", d.text, d.added, d.removed)
	}
	if d := unifiedDiff(a, a, "a", "b", 3, 100); d.text != "" || d.added+d.removed != 0 {
		t.Fatalf("identical: %+v", d)
	}
	if d := unifiedDiff("", "x\n", "a", "b", 3, 100); d.text != "--- a\n+++ b\n@@ -0,0 +1 @@\n+x" {
		t.Fatalf("from empty: %q", d.text)
	}
	if d := unifiedDiff(a, b, "a", "b", 2, 4); !d.truncated || strings.Count(d.text, "\n") != 3 {
		t.Fatalf("truncate: %+v", d)
	}
}

func TestBuildDiagnosePipeline(t *testing.T) {
	fb := graphBuild()
	fb.getBuild = func(_ string, ref BuildRef) (Build, error) {
		if ref != RefLastFailed {
			t.Errorf("default ref %q", ref)
		}
		return Build{Number: 57, Result: "FAILURE", URL: "u", Changes: []Change{
			{CommitID: "9f2c1e0aa", Message: "order: return 201\n\nlong body", Author: "Jane"},
		}}, nil
	}
	fb.testReport = func(string, int) (TestReport, error) {
		return TestReport{HasReport: true, FailCount: 3, PassCount: 10, Failed: []TestCase{
			{Name: "a", Status: "FAILED"}, {Name: "b", Status: "REGRESSION"}, {Name: "c", Status: "FAILED"},
		}}, nil
	}
	out, err := newSvc(fb, policy.Policy{}).BuildDiagnose(context.Background(), BuildDiagnoseIn{Job: "app", MaxTests: 2})
	if err != nil {
		t.Fatal(err)
	}
	f := out.Failure
	if f == nil || f.StageID != "407" || f.Stage != "Mac: Images" || f.Step == nil || f.Step.Name != "npm run images:build" {
		t.Fatalf("failure %+v", f)
	}
	if f.LogSource != "step" || f.Log != "building images\nexited with 1: images:build" || f.LogLines != 2 {
		t.Fatalf("log %+v", f)
	}
	if out.StageSource != SourcePGV || !slices.Equal(out.FailedStages, []string{"Mac: Images"}) {
		t.Fatalf("stages %v %v", out.StageSource, out.FailedStages)
	}
	if out.Tests == nil || len(out.Tests.Failed) != 2 || !out.Tests.Truncated {
		t.Fatalf("tests %+v", out.Tests)
	}
	if len(out.Changes) != 1 || out.Changes[0].Message != "order: return 201" {
		t.Fatalf("changes %+v", out.Changes)
	}
	if len(out.Next) != 3 || !strings.Contains(out.Next[0], "stage=407") || len(out.Warnings) != 0 {
		t.Fatalf("next %v warnings %v", out.Next, out.Warnings)
	}
}

func TestBuildDiagnoseFreestyleAndSuccess(t *testing.T) {
	fb := &fakeBackend{
		getBuild:   func(string, BuildRef) (Build, error) { return Build{Number: 4, Result: "FAILURE"}, nil },
		getRun:     func(string, int) (Run, error) { return Run{}, errs.NotFound("no wfapi") },
		console:    consoleOf("compiling\nerror: boom\n"),
		testReport: func(string, int) (TestReport, error) { return TestReport{}, nil },
	}
	out, err := newSvc(fb, policy.Policy{}).BuildDiagnose(context.Background(), BuildDiagnoseIn{Job: "app", Ref: "4"})
	if err != nil || out.Failure == nil || out.Failure.LogSource != "console" || out.Failure.Log != "compiling\nerror: boom" || out.Tests != nil {
		t.Fatalf("freestyle: %+v, %v", out, err)
	}

	ok := &fakeBackend{getBuild: func(string, BuildRef) (Build, error) { return Build{Number: 5, Result: "SUCCESS"}, nil }}
	out, err = newSvc(ok, policy.Policy{}).BuildDiagnose(context.Background(), BuildDiagnoseIn{Job: "app", Ref: "5"})
	if err != nil || out.Failure != nil || ok.called("GetStageTree") {
		t.Fatalf("success: %+v, %v", out, err)
	}
}

func TestBuildDiagnoseKeepsPartialResults(t *testing.T) {
	fb := graphBuild()
	fb.getBuild = func(string, BuildRef) (Build, error) { return Build{Number: 9, Result: "FAILURE"}, nil }
	fb.testReport = func(string, int) (TestReport, error) { return TestReport{}, errs.New(errs.CodeUpstream, "HTTP 500") }
	out, err := newSvc(fb, policy.Policy{}).BuildDiagnose(context.Background(), BuildDiagnoseIn{Job: "app", Ref: "9"})
	if err != nil || out.Failure == nil || len(out.Warnings) != 1 || !strings.HasPrefix(out.Warnings[0], "tests:") {
		t.Fatalf("got %+v, %v", out, err)
	}
}

type fakeConfirmer struct {
	yes   bool
	err   error
	asked []string
}

func (c *fakeConfirmer) Confirm(_ context.Context, m string) (bool, error) {
	c.asked = append(c.asked, m)
	return c.yes, c.err
}

func TestDangerOpsAskTheHuman(t *testing.T) {
	stopped := 0
	fb := &fakeBackend{stop: func(string, int) error { stopped++; return nil }}
	s := newSvc(fb, policy.Policy{Tier: policy.TierDanger})

	// Declined: no stop, even with the agent's confirm=true.
	c := &fakeConfirmer{}
	_, err := s.BuildStop(interact.WithConfirmer(context.Background(), c), BuildStopIn{Job: "app", Ref: "9", Confirm: true})
	wantCode(t, err, errs.CodePolicyDenied)
	if stopped != 0 || len(c.asked) != 1 || !strings.Contains(c.asked[0], "POST /job/app/9/stop") {
		t.Fatalf("stopped=%d asked=%q", stopped, c.asked)
	}
	// Approved without the flag.
	c = &fakeConfirmer{yes: true}
	if _, err := s.BuildStop(interact.WithConfirmer(context.Background(), c), BuildStopIn{Job: "app", Ref: "9"}); err != nil || stopped != 1 {
		t.Fatalf("approved: %v stopped=%d", err, stopped)
	}
	// Pending: passed through unchanged for the transport.
	c = &fakeConfirmer{err: interact.ErrPending}
	if _, err := s.BuildStop(interact.WithConfirmer(context.Background(), c), BuildStopIn{Job: "app", Ref: "9"}); !errors.Is(err, interact.ErrPending) {
		t.Fatalf("pending: %v", err)
	}
	// No answer at all: fall back to the confirm flag.
	c = &fakeConfirmer{err: errors.New("client failed")}
	_, err = s.BuildStop(interact.WithConfirmer(context.Background(), c), BuildStopIn{Job: "app", Ref: "9"})
	wantCode(t, err, errs.CodeNeedsConfirmation)
	if stopped != 1 {
		t.Fatal("stopped without an answer")
	}
	// Dry run never asks.
	c = &fakeConfirmer{}
	if _, err := s.BuildStop(interact.WithConfirmer(context.Background(), c), BuildStopIn{Job: "app", Ref: "9", DryRun: true}); err != nil || len(c.asked) != 0 {
		t.Fatalf("dry run asked: %v %q", err, c.asked)
	}
}

type progressRec struct {
	done []float64
	msgs []string
}

func (p *progressRec) Report(_ context.Context, done, _ float64, m string) {
	p.done, p.msgs = append(p.done, done), append(p.msgs, m)
}

func TestWaitReportsProgress(t *testing.T) {
	polls := 0
	fb := &fakeBackend{
		queueItem: func(id int64) (QueueItem, error) {
			polls++
			if polls < 3 {
				return QueueItem{ID: id, Why: "Waiting for next available executor"}, nil
			}
			return QueueItem{ID: id, BuildNumber: intp(4)}, nil
		},
		getBuild: func(string, BuildRef) (Build, error) {
			if polls < 5 {
				polls++
				return Build{Number: 4, Building: true}, nil
			}
			return Build{Number: 4, Result: "SUCCESS"}, nil
		},
	}
	clk := &fakeClock{now: t0}
	s := newSvc(fb, policy.Policy{}, WithClock(clk.Now), WithSleep(clk.Sleep), WithPollInterval(time.Second))
	p := &progressRec{}
	out, err := s.BuildWait(interact.WithProgress(context.Background(), p), BuildWaitIn{Job: "app", QueueID: 3})
	if err != nil || out.Result != "SUCCESS" {
		t.Fatalf("got %+v, %v", out, err)
	}
	if len(p.msgs) < 3 || p.msgs[0] != "queue item 3: Waiting for next available executor" || !strings.HasPrefix(p.msgs[len(p.msgs)-1], "build #4 running") {
		t.Fatalf("messages %q", p.msgs)
	}
	if !slices.IsSorted(p.done) {
		t.Fatalf("progress not increasing: %v", p.done)
	}
}

func TestSpillCarriesResourceURI(t *testing.T) {
	fb := &fakeBackend{console: consoleOf(strings.Repeat(strings.Repeat("z", 99)+"\n", 1000))}
	s := newSvc(fb, policy.Policy{}, WithSpill(func(string, string) (string, error) { return "/tmp/jk/build_log-3-77.txt", nil }))
	out, err := s.BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "3"})
	if err != nil || out.SpilledURI != "jk://spill/build_log-3-77.txt" {
		t.Fatalf("got %+v, %v", out.SpilledURI, err)
	}
}

func countCalls(fb *fakeBackend, prefix string) int {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	n := 0
	for _, c := range fb.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func TestStepErrorOnlyForFirstFailure(t *testing.T) {
	fb := graphBuild()
	// A second real failure in another leg.
	tree := fb.stageTree
	fb.stageTree = func(j string, n int) ([]GraphStage, error) {
		gs, _ := tree(j, n)
		gs[1].Children[0].State = "failure"
		gs[1].Children[0].Children[0].State = "failure"
		return gs, nil
	}
	steps := fb.steps
	fb.steps = func(j string, n int) ([]GraphStep, error) {
		ss, _ := steps(j, n)
		ss[0].State = "failure"
		return ss, nil
	}
	out, err := newSvc(fb, policy.Policy{}).BuildStages(context.Background(), BuildStagesIn{Job: "app", Ref: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := countCalls(fb, "GetStepError"); got != 1 {
		t.Fatalf("GetStepError calls = %d, want 1", got)
	}
	if out.FirstFailed != "104" {
		t.Fatalf("first failed %s", out.FirstFailed)
	}
}

// manySteps is a stage of n steps, each logging 10 lines "s<i> line <j>".
func manySteps(n int) *fakeBackend {
	var steps []GraphStep
	for i := 1; i <= n; i++ {
		steps = append(steps, GraphStep{ID: strconv.Itoa(100 + i), StageID: "7", State: "success"})
	}
	steps[n-1].State = "failure"
	return &fakeBackend{
		stageTree: func(string, int) ([]GraphStage, error) {
			return []GraphStage{{ID: "7", Name: "Test", Type: "STAGE", State: "failure"}}, nil
		},
		steps:     func(string, int) ([]GraphStep, error) { return steps, nil },
		stepError: func(string, int, string) (string, error) { return "script returned exit code 1", nil },
		nodeLog: func(_ string, _ int, id string) (string, error) {
			var b strings.Builder
			for j := 1; j <= 10; j++ {
				b.WriteString("s" + id + " line " + strconv.Itoa(j) + "\n")
			}
			return b.String(), nil
		},
	}
}

func TestStageTailReadsLastStepsFirst(t *testing.T) {
	fb := manySteps(12)
	out, err := newSvc(fb, policy.Policy{}).BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "1", Stage: "Test", Lines: 15})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out.Text, "\n")
	if len(lines) != 15 || lines[0] != "s111 line 6" || lines[14] != "s112 line 10" {
		t.Fatalf("text %q", out.Text)
	}
	// One batch of 4 holds 40 >= 15 lines: the 8 earlier steps are not read.
	if got := countCalls(fb, "StreamNodeLog"); got != 4 || out.StepsRead != 4 || out.StepsTotal != 12 {
		t.Fatalf("calls=%d read=%d total=%d", got, out.StepsRead, out.StepsTotal)
	}
	if out.TotalLines != 0 || !out.Truncated {
		t.Fatalf("partial read must omit total_lines: %+v", out)
	}

	// Asking for more than the stage has reads every step, in order.
	fb = manySteps(6)
	out, err = newSvc(fb, policy.Policy{}).BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "1", Stage: "Test"})
	if err != nil || out.TotalLines != 60 || out.StepsRead != 6 || out.Truncated || !strings.HasPrefix(out.Text, "s101 line 1\n") {
		t.Fatalf("full: %+v, %v", out, err)
	}
}

func TestBuildLogFailedStep(t *testing.T) {
	fb := graphBuild()
	s := newSvc(fb, policy.Policy{})
	ctx := context.Background()

	// No stage: the failing step of the first failed stage.
	out, err := s.BuildLog(ctx, BuildLogIn{Job: "app", Ref: "1", Step: "failed"})
	if err != nil || out.Source != "step" || out.StepID != "425" || out.Step != "npm run images:build" || out.Stage != "Mac: Images" {
		t.Fatalf("got %+v, %v", out, err)
	}
	if out.Text != "building images\nexited with 1: images:build" || out.TotalLines != 2 {
		t.Fatalf("text %q", out.Text)
	}
	// A parallel leg resolves to its failing nested stage.
	out, err = s.BuildLog(ctx, BuildLogIn{Job: "app", Ref: "1", Stage: "Mac", Step: "failed"})
	if err != nil || out.StepID != "425" {
		t.Fatalf("leg: %+v, %v", out, err)
	}
	// A stage without a failure.
	_, err = s.BuildLog(ctx, BuildLogIn{Job: "app", Ref: "1", Stage: "Check", Step: "failed"})
	wantCode(t, err, errs.CodeNotFound)
}

func TestDiagnoseHighlightsAndTrim(t *testing.T) {
	fb := graphBuild()
	fb.getBuild = func(string, BuildRef) (Build, error) { return Build{Number: 3, Result: "FAILURE"}, nil }
	fb.testReport = func(string, int) (TestReport, error) { return TestReport{}, nil }
	fb.nodeLog = func(_ string, _ int, id string) (string, error) {
		return "\nprep\nERROR: image build failed\nnoise\nwarn x\n\n\n", nil
	}
	out, err := newSvc(fb, policy.Policy{}).BuildDiagnose(context.Background(), BuildDiagnoseIn{Job: "app", Ref: "3"})
	if err != nil {
		t.Fatal(err)
	}
	f := out.Failure
	if f.Log != "prep\nERROR: image build failed\nnoise\nwarn x" || f.LogLines != 4 {
		t.Fatalf("log %q", f.Log)
	}
	if len(f.Highlights) != 1 || f.Highlights[0].Line != 3 || f.Highlights[0].Text != "ERROR: image build failed" {
		t.Fatalf("highlights %+v", f.Highlights)
	}
}

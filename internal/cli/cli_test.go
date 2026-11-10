package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/auth"
	"github.com/phuquocchamp/jk/internal/kit/clix"
	"github.com/phuquocchamp/jk/internal/kit/config"
	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

// ---- fake backend ----

type fakeBackend struct {
	jobs        []core.JobSummary
	job         core.Job
	build       core.Build
	console     string
	listFolder  string
	triggered   map[string]string
	triggerPar  bool
	triggerHits int
	replayed    string
	stopped     int
	// lastSeq, when set, is returned by successive permalink resolutions
	// (simulates new builds starting between calls).
	lastSeq []int
}

var _ core.Backend = (*fakeBackend)(nil)

func (f *fakeBackend) BaseURL() string { return "https://jenkins.example.com" }
func (f *fakeBackend) Whoami(context.Context) (core.User, error) {
	return core.User{ID: "jane", FullName: "Jane Doe"}, nil
}
func (f *fakeBackend) ListJobs(_ context.Context, folder string) ([]core.JobSummary, error) {
	f.listFolder = folder
	return f.jobs, nil
}
func (f *fakeBackend) GetJob(_ context.Context, job string) (core.Job, error) {
	j := f.job
	j.Path = job
	return j, nil
}
func (f *fakeBackend) ResolveBuild(_ context.Context, _ string, ref core.BuildRef) (int, error) {
	if n, ok := ref.Number(); ok {
		return n, nil
	}
	if len(f.lastSeq) > 0 {
		n := f.lastSeq[0]
		if len(f.lastSeq) > 1 {
			f.lastSeq = f.lastSeq[1:]
		}
		return n, nil
	}
	return 42, nil
}
func (f *fakeBackend) GetBuild(_ context.Context, job string, ref core.BuildRef) (core.Build, error) {
	b := f.build
	b.Job = job
	if n, ok := ref.Number(); ok {
		b.Number = n
	}
	return b, nil
}
func (f *fakeBackend) GetRun(context.Context, string, int) (core.Run, error) {
	return core.Run{ID: "42", Status: "FAILED", DurationMs: 90000, Stages: []core.Stage{
		{ID: "6", Name: "Build", Status: "SUCCESS", DurationMs: 60000, StartTime: time.Unix(1700000000, 0).UTC()},
		{ID: "17", Name: "Test", Status: "FAILED", DurationMs: 30000, StartTime: time.Unix(1700000060, 0).UTC()},
	}}, nil
}
func (f *fakeBackend) GetStageNodes(context.Context, string, int, string) ([]core.StageNode, error) {
	return nil, nil
}
func (f *fakeBackend) GetNodeLog(context.Context, string, int, string) (core.NodeLog, error) {
	return core.NodeLog{}, nil
}
func (f *fakeBackend) StreamConsole(context.Context, string, int) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.console)), nil
}
func (f *fakeBackend) GetTestReport(context.Context, string, int) (core.TestReport, error) {
	return core.TestReport{HasReport: true, FailCount: 1, PassCount: 9, Failed: []core.TestCase{
		{ClassName: "com.example.FooTest", Name: "bar", Status: "FAILED", ErrorDetails: "expected 1"},
	}}, nil
}
func (f *fakeBackend) GetQueueItem(_ context.Context, id int64) (core.QueueItem, error) {
	return core.QueueItem{ID: id, Why: "Waiting for next available executor"}, nil
}
func (f *fakeBackend) TriggerBuild(_ context.Context, _ string, params map[string]string, parameterized bool) (int64, error) {
	f.triggerHits++
	f.triggered, f.triggerPar = params, parameterized
	return 1234, nil
}
func (f *fakeBackend) ReplayBuild(_ context.Context, _ string, _ int, script string) (int64, error) {
	f.replayed = script
	return 99, nil
}
func (f *fakeBackend) StopBuild(_ context.Context, _ string, n int) error {
	f.stopped = n
	return nil
}

// ---- harness ----

type harness struct {
	t      *testing.T
	fb     *fakeBackend
	pol    policy.Policy
	stdin  *os.File
	auth   *auth.Resolver
	canAsk func() bool
}

func newHarness(t *testing.T, tier policy.Tier) *harness {
	t.Helper()
	keyring.MockInit()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fb := &fakeBackend{
		jobs: []core.JobSummary{
			{Name: "api", Path: "team-a/api", Kind: core.KindPipeline, Color: "blue", URL: "https://jenkins.example.com/job/team-a/job/api/"},
			{Name: "web", Path: "team-a/web", Kind: core.KindPipeline, Color: "red", URL: "https://jenkins.example.com/job/team-a/job/web/"},
		},
		job:     core.Job{Name: "api", Kind: core.KindPipeline, Parameters: []core.ParamDef{{Name: "ENV", Type: "StringParameterDefinition"}, {Name: "FLAGS", Type: "StringParameterDefinition"}}},
		build:   core.Build{Number: 42, Result: "FAILURE", DurationMs: 83000, URL: "https://jenkins.example.com/job/team-a/job/api/42/"},
		console: "line 1\nline 2\nERROR: boom\nline 4\n",
	}
	r := auth.NewResolver("jk", "JENKINS_API_TOKEN")
	r.Getenv = func(string) string { return "" } // never pick up a real token from the env
	return &harness{t: t, fb: fb, pol: policy.Policy{Tier: tier}, auth: r}
}

func (h *harness) run(args ...string) (int, string, string) {
	h.t.Helper()
	var stdout, stderr bytes.Buffer
	g := &clix.Globals{}
	open := func(ctx context.Context, g *clix.Globals) (*Env, error) {
		tier, err := policy.EffectiveTier(h.pol.Tier, g.Tier)
		if err != nil {
			return nil, err
		}
		pol := h.pol
		pol.Tier = tier
		svc := core.NewService(h.fb, policy.NewEngine(pol))
		return &Env{Service: svc, Tier: tier, Profile: config.Resolved{Name: "default", URL: h.fb.BaseURL(), User: "jane"}}, nil
	}
	root := NewRootCmd(Deps{
		Version: "1.2.3",
		Globals: g,
		Open:    open,
		LoadProfile: func(p string) (config.Resolved, error) {
			if p == "" {
				p = "default"
			}
			return config.Resolved{Name: p}, nil
		},
		Auth:   h.auth,
		Stdin:  h.stdin,
		Stdout: &stdout,
		Stderr: &stderr,
		CanAsk: h.canAsk,
	})
	root.SetArgs(args)
	code := clix.Execute(root, g, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func (h *harness) withStdin(content string) {
	h.t.Helper()
	p := filepath.Join(h.t.TempDir(), "stdin")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		h.t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { f.Close() })
	h.stdin = f
}

func asMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("not a JSON object: %q (%v)", s, err)
	}
	return m
}

func errCode(t *testing.T, stderr string) string {
	t.Helper()
	env := asMap(t, stderr)
	e, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error envelope: %q", stderr)
	}
	return e["code"].(string)
}

// ---- tests ----

func TestVersion(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	code, out, _ := h.run("version")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if m := asMap(t, out); m["version"] != "1.2.3" || m["name"] != "jk" {
		t.Fatalf("got %v", m)
	}
}

func TestUsageErrors(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	cases := [][]string{
		{"nope"},
		{"job", "nope"},
		{"job", "list", "--bogus"},
		{"build", "get"},
		{"build", "stop", "a"},
		{"queue", "get", "abc"},
		{"log", "search", "team-a/api"},
		{"build", "trigger", "team-a/api", "-p", "novalue"},
		{"build", "trigger", "team-a/api", "--timeout", "10"},
		{"build", "replay", "team-a/api", "42"},
		{"log", "tail", "team-a/api", "--lines", "abc"},
		{"build", "get", "team-a/api", "notaref"},
		{"job", "list", "--jq", ".jobs["},
	}
	for _, args := range cases {
		code, out, stderr := h.run(args...)
		if code != 2 {
			t.Errorf("%v: exit %d, want 2 (stderr %q)", args, code, stderr)
			continue
		}
		if out != "" {
			t.Errorf("%v: stdout should be empty, got %q", args, out)
		}
		if c := errCode(t, stderr); c != "USAGE" {
			t.Errorf("%v: code %s", args, c)
		}
	}
}

func TestJobListFieldsAndJQ(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	code, out, stderr := h.run("job", "list", "team-a", "--fields", "name,path")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if h.fb.listFolder != "team-a" {
		t.Fatalf("folder %q", h.fb.listFolder)
	}
	m := asMap(t, out)
	jobs := m["jobs"].([]any)
	if len(m) != 1 || len(jobs) != 2 {
		t.Fatalf("got %v", m)
	}
	if j := jobs[1].(map[string]any); len(j) != 2 || j["path"] != "team-a/web" {
		t.Fatalf("job %v", j)
	}

	code, out, _ = h.run("job", "list", "team-a", "--jq", "[.jobs[].name]")
	if code != 0 || strings.Join(strings.Fields(out), "") != `["api","web"]` {
		t.Fatalf("jq: exit %d out %q", code, out)
	}
}

func TestBuildGetAndStages(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	code, out, stderr := h.run("build", "get", "https://jenkins.example.com/job/team-a/job/api/7/")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	m := asMap(t, out)
	if m["job"] != "team-a/api" || m["number"] != float64(7) || m["duration_ms"] != float64(83000) {
		t.Fatalf("got %v", m)
	}

	code, out, _ = h.run("build", "stages", "team-a/api", "lastFailed", "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if st := asMap(t, out)["stages"].([]any); len(st) != 2 {
		t.Fatalf("stages %v", st)
	}
}

func TestLogTailAndSearch(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	code, out, stderr := h.run("log", "tail", "team-a/api", "42", "-n", "2")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if m := asMap(t, out); m["text"] != "ERROR: boom\nline 4" && m["text"] != "ERROR: boom\nline 4\n" {
		t.Fatalf("text %q", m["text"])
	}

	code, out, stderr = h.run("log", "search", "team-a/api", "--pattern", "ERROR", "-C", "1")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	matches := asMap(t, out)["matches"].([]any)
	if len(matches) != 1 || matches[0].(map[string]any)["line"] != float64(3) {
		t.Fatalf("matches %v", matches)
	}
}

func TestTestFailedAndQueue(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	code, out, stderr := h.run("test", "failed", "team-a/api")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if m := asMap(t, out); m["fail_count"] != float64(1) || len(m["failed"].([]any)) != 1 {
		t.Fatalf("got %v", m)
	}
	code, out, _ = h.run("queue", "get", "1234")
	if code != 0 || asMap(t, out)["id"] != float64(1234) {
		t.Fatalf("queue: exit %d out %q", code, out)
	}
}

func TestTriggerParamsAndTier(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	code, _, stderr := h.run("build", "trigger", "team-a/api")
	if code != 4 || errCode(t, stderr) != "POLICY_DENIED" {
		t.Fatalf("read tier: exit %d stderr %q", code, stderr)
	}

	h = newHarness(t, policy.TierWrite)
	code, out, stderr := h.run("build", "trigger", "team-a/api", "-p", "ENV=qa", "-p", "FLAGS=a=b,c")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !h.fb.triggerPar || h.fb.triggered["ENV"] != "qa" || h.fb.triggered["FLAGS"] != "a=b,c" {
		t.Fatalf("params %v parameterized %v", h.fb.triggered, h.fb.triggerPar)
	}
	if asMap(t, out)["queue_id"] != float64(1234) {
		t.Fatalf("out %q", out)
	}

	// --dry-run returns the plan and does not call Jenkins.
	h.fb.triggerHits = 0
	code, out, _ = h.run("build", "trigger", "team-a/api", "-p", "ENV=qa", "--dry-run")
	if code != 0 || h.fb.triggerHits != 0 {
		t.Fatalf("dry-run: exit %d hits %d", code, h.fb.triggerHits)
	}
	plan := asMap(t, out)["plan"].(map[string]any)
	if plan["path"] != "/job/team-a/job/api/buildWithParameters" {
		t.Fatalf("plan %v", plan)
	}

	// --tier may only lower the tier.
	code, _, stderr = h.run("build", "trigger", "team-a/api", "--tier", "danger")
	if code != 4 || errCode(t, stderr) != "POLICY_DENIED" {
		t.Fatalf("--tier danger: exit %d %q", code, stderr)
	}
	code, _, _ = h.run("build", "trigger", "team-a/api", "--tier", "read")
	if code != 4 {
		t.Fatalf("--tier read: exit %d", code)
	}
}

func TestReplayFromStdin(t *testing.T) {
	h := newHarness(t, policy.TierWrite)
	h.withStdin("pipeline { agent any }\n")
	code, out, stderr := h.run("build", "replay", "team-a/api", "42", "--script-file", "-")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if h.fb.replayed != "pipeline { agent any }\n" {
		t.Fatalf("script %q", h.fb.replayed)
	}
	if m := asMap(t, out); m["queue_id"] != float64(99) || m["from_number"] != float64(42) {
		t.Fatalf("got %v", m)
	}
}

func TestStopNeedsConfirmation(t *testing.T) {
	h := newHarness(t, policy.TierDanger)
	code, out, stderr := h.run("build", "stop", "team-a/api", "42")
	if code != 7 || out != "" {
		t.Fatalf("exit %d out %q", code, out)
	}
	e := asMap(t, stderr)["error"].(map[string]any)
	if e["code"] != "NEEDS_CONFIRMATION" || e["details"].(map[string]any)["path"] != "/job/team-a/job/api/42/stop" {
		t.Fatalf("error %v", e)
	}
	if h.fb.stopped != 0 {
		t.Fatal("stopped without confirmation")
	}

	code, out, stderr = h.run("build", "stop", "team-a/api", "42", "--yes")
	if code != 0 || h.fb.stopped != 42 || asMap(t, out)["stopped"] != true {
		t.Fatalf("exit %d out %q err %q", code, out, stderr)
	}

	code, _, _ = h.run("build", "stop", "team-a/api", "42", "--tier", "write", "--yes")
	if code != 4 {
		t.Fatalf("--tier write: exit %d", code)
	}
}

func TestAuthLoginStatusLogout(t *testing.T) {
	const token = "fake-token-0123456789abcdef"
	h := newHarness(t, policy.TierRead)

	// Without --with-token and without a TTY, never prompt.
	h.withStdin(token + "\n")
	code, _, stderr := h.run("auth", "login")
	if code != 2 || strings.Contains(stderr, token) {
		t.Fatalf("no-TTY login: exit %d stderr %q", code, stderr)
	}

	h.withStdin(token + "\n")
	code, out, stderr := h.run("auth", "login", "--with-token", "--profile", "work")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.Contains(out+stderr, token) {
		t.Fatal("token leaked to output")
	}
	if got, err := keyring.Get("jk", "work"); err != nil || got != token {
		t.Fatalf("keyring %q %v", got, err)
	}
	if m := asMap(t, out); m["profile"] != "work" || m["stored"] != true {
		t.Fatalf("got %v", m)
	}

	if err := h.auth.Store("default", token); err != nil {
		t.Fatal(err)
	}
	code, out, stderr = h.run("auth", "status")
	if code != 0 {
		t.Fatalf("status exit %d: %s", code, stderr)
	}
	if strings.Contains(out+stderr, token) {
		t.Fatal("token leaked in status")
	}
	m := asMap(t, out)
	if m["id"] != "jane" || m["full_name"] != "Jane Doe" || m["token_source"] != "keyring" || m["tier"] != "read" {
		t.Fatalf("status %v", m)
	}

	code, out, _ = h.run("auth", "logout", "--profile", "work")
	if code != 0 || asMap(t, out)["removed"] != true {
		t.Fatalf("logout exit %d out %q", code, out)
	}
	if _, err := keyring.Get("jk", "work"); err != keyring.ErrNotFound {
		t.Fatalf("token still stored: %v", err)
	}
}

func TestOpenErrorMapsExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	g := &clix.Globals{}
	root := NewRootCmd(Deps{
		Globals: g,
		Open: func(context.Context, *clix.Globals) (*Env, error) {
			return nil, errs.New(errs.CodeAuthRequired, "no API token").WithHint("run auth login")
		},
		Stdout: &stdout, Stderr: &stderr,
	})
	root.SetArgs([]string{"job", "get", "team-a/api"})
	if code := clix.Execute(root, g, &stdout, &stderr); code != 6 {
		t.Fatalf("exit %d", code)
	}
	if errCode(t, stderr.String()) != "AUTH_REQUIRED" {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestHumanViews(t *testing.T) {
	jl := jobListView(core.JobListOut{Jobs: []core.JobSummary{{Name: "api", Kind: "pipeline", Path: "team-a/api"}}})
	if rows := jl.TableRows(); len(rows) != 1 || rows[0][3] != "team-a/api" {
		t.Fatalf("rows %v", rows)
	}
	sv := searchView(core.LogSearchOut{Matches: []core.LogMatch{{Line: 3, Text: "ERROR", Before: []string{"b"}, After: []string{"a"}}}})
	if got := sv.PlainText(); got != "2-b\n3:ERROR\n4-a\n" {
		t.Fatalf("search %q", got)
	}
	st := stagesView(core.BuildStagesOut{Stages: []core.Stage{{ID: "6", Name: "Build", Status: "SUCCESS", DurationMs: 61000}}})
	if r := st.TableRows()[0]; r[3] != "1m1s" {
		t.Fatalf("stage row %v", r)
	}
}

func TestStopInteractiveConfirmPinsBuild(t *testing.T) {
	h := newHarness(t, policy.TierDanger)
	h.canAsk = func() bool { return true }
	// "last" is 57 when the plan is shown, 58 once the user has answered.
	h.fb.lastSeq = []int{57, 58}
	h.withStdin("y\n")
	code, out, stderr := h.run("build", "stop", "team-a/api", "last")
	if code != 0 {
		t.Fatalf("exit %d out %q err %q", code, out, stderr)
	}
	if h.fb.stopped != 57 {
		t.Fatalf("stopped build %d, want the confirmed 57", h.fb.stopped)
	}
	if !strings.Contains(stderr, "57/stop") {
		t.Fatalf("plan not shown: %q", stderr)
	}

	h2 := newHarness(t, policy.TierDanger)
	h2.canAsk = func() bool { return true }
	h2.withStdin("n\n")
	if code, _, _ := h2.run("build", "stop", "team-a/api", "last"); code != 7 || h2.fb.stopped != 0 {
		t.Fatalf("declined: exit %d stopped %d", code, h2.fb.stopped)
	}
}

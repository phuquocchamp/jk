package core

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

// fakeBackend is a Backend driven by function fields; unset fields fail the
// call with INTERNAL so tests notice unexpected calls. Every call is recorded.
type fakeBackend struct {
	mu    sync.Mutex
	calls []string

	whoami        func() (User, error)
	listJobs      func(folder string) ([]JobSummary, error)
	getJob        func(job string) (Job, error)
	resolveBuild  func(job string, ref BuildRef) (int, error)
	getBuild      func(job string, ref BuildRef) (Build, error)
	getRun        func(job string, n int) (Run, error)
	getStageNodes func(job string, n int, id string) ([]StageNode, error)
	getNodeLog    func(job string, n int, id string) (NodeLog, error)
	console       func(job string, n int) (io.ReadCloser, error)
	testReport    func(job string, n int) (TestReport, error)
	queueItem     func(id int64) (QueueItem, error)
	trigger       func(job string, params map[string]string, parameterized bool) (int64, error)
	replay        func(job string, n int, script string) (int64, error)
	stop          func(job string, n int) error
}

func (f *fakeBackend) rec(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

func (f *fakeBackend) called(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

var errUnexpected = errs.Internal("unexpected backend call")

func (f *fakeBackend) BaseURL() string { return "https://jenkins.example.com" }
func (f *fakeBackend) Whoami(ctx context.Context) (User, error) {
	f.rec("Whoami")
	if f.whoami == nil {
		return User{}, errUnexpected
	}
	return f.whoami()
}
func (f *fakeBackend) ListJobs(ctx context.Context, folder string) ([]JobSummary, error) {
	f.rec("ListJobs %s", folder)
	if f.listJobs == nil {
		return nil, errUnexpected
	}
	return f.listJobs(folder)
}
func (f *fakeBackend) GetJob(ctx context.Context, job string) (Job, error) {
	f.rec("GetJob %s", job)
	if f.getJob == nil {
		return Job{}, errUnexpected
	}
	return f.getJob(job)
}
func (f *fakeBackend) ResolveBuild(ctx context.Context, job string, ref BuildRef) (int, error) {
	f.rec("ResolveBuild %s %s", job, ref)
	if f.resolveBuild == nil {
		return 0, errUnexpected
	}
	return f.resolveBuild(job, ref)
}
func (f *fakeBackend) GetBuild(ctx context.Context, job string, ref BuildRef) (Build, error) {
	f.rec("GetBuild %s %s", job, ref)
	if f.getBuild == nil {
		return Build{}, errUnexpected
	}
	return f.getBuild(job, ref)
}
func (f *fakeBackend) GetRun(ctx context.Context, job string, n int) (Run, error) {
	f.rec("GetRun %s %d", job, n)
	if f.getRun == nil {
		return Run{}, errUnexpected
	}
	return f.getRun(job, n)
}
func (f *fakeBackend) GetStageNodes(ctx context.Context, job string, n int, id string) ([]StageNode, error) {
	f.rec("GetStageNodes %s %d %s", job, n, id)
	if f.getStageNodes == nil {
		return nil, errUnexpected
	}
	return f.getStageNodes(job, n, id)
}
func (f *fakeBackend) GetNodeLog(ctx context.Context, job string, n int, id string) (NodeLog, error) {
	f.rec("GetNodeLog %s %d %s", job, n, id)
	if f.getNodeLog == nil {
		return NodeLog{}, errUnexpected
	}
	return f.getNodeLog(job, n, id)
}
func (f *fakeBackend) StreamConsole(ctx context.Context, job string, n int) (io.ReadCloser, error) {
	f.rec("StreamConsole %s %d", job, n)
	if f.console == nil {
		return nil, errUnexpected
	}
	return f.console(job, n)
}
func (f *fakeBackend) GetTestReport(ctx context.Context, job string, n int) (TestReport, error) {
	f.rec("GetTestReport %s %d", job, n)
	if f.testReport == nil {
		return TestReport{}, errUnexpected
	}
	return f.testReport(job, n)
}
func (f *fakeBackend) GetQueueItem(ctx context.Context, id int64) (QueueItem, error) {
	f.rec("GetQueueItem %d", id)
	if f.queueItem == nil {
		return QueueItem{}, errUnexpected
	}
	return f.queueItem(id)
}
func (f *fakeBackend) TriggerBuild(ctx context.Context, job string, params map[string]string, parameterized bool) (int64, error) {
	f.rec("TriggerBuild %s %v", job, parameterized)
	if f.trigger == nil {
		return 0, errUnexpected
	}
	return f.trigger(job, params, parameterized)
}
func (f *fakeBackend) ReplayBuild(ctx context.Context, job string, n int, script string) (int64, error) {
	f.rec("ReplayBuild %s %d", job, n)
	if f.replay == nil {
		return 0, errUnexpected
	}
	return f.replay(job, n, script)
}
func (f *fakeBackend) StopBuild(ctx context.Context, job string, n int) error {
	f.rec("StopBuild %s %d", job, n)
	if f.stop == nil {
		return errUnexpected
	}
	return f.stop(job, n)
}

var _ Backend = (*fakeBackend)(nil)

func newSvc(b Backend, p policy.Policy, opts ...Option) *Service {
	return NewService(b, policy.NewEngine(p), opts...)
}

func wantCode(t *testing.T, err error, code errs.Code) *errs.Error {
	t.Helper()
	e, ok := errs.As(err)
	if !ok || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	return e
}

func intp(n int) *int { return &n }

// numberedLog returns "line 1\nline 2\n...line n\n".
func numberedLog(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// countingReader records how many bytes were consumed and whether it was closed.
type countingReader struct {
	r      io.Reader
	read   int
	closed bool
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += n
	return n, err
}
func (c *countingReader) Close() error { c.closed = true; return nil }

// ---- policy and parsing ----

func TestPolicyDeniedBeforeBackend(t *testing.T) {
	ctx := context.Background()
	fb := &fakeBackend{}
	read := newSvc(fb, policy.Policy{Tier: policy.TierRead})
	_, err := read.BuildTrigger(ctx, BuildTriggerIn{Job: "team-a/app"})
	wantCode(t, err, errs.CodePolicyDenied)
	_, err = read.BuildStop(ctx, BuildStopIn{Job: "team-a/app", Ref: "3", Confirm: true})
	wantCode(t, err, errs.CodePolicyDenied)

	globbed := newSvc(fb, policy.Policy{
		Tier:  policy.TierDanger,
		Allow: policy.Rules{"jobs": {"team-a/**"}},
		Deny:  policy.Rules{"jobs": {"team-a/secret/**"}},
	})
	_, err = globbed.JobGet(ctx, JobGetIn{Job: "team-b/app"})
	wantCode(t, err, errs.CodePolicyDenied)
	_, err = globbed.BuildStop(ctx, BuildStopIn{Job: "team-a/secret/app", Ref: "3", Confirm: true})
	wantCode(t, err, errs.CodePolicyDenied)
	// A pasted URL is checked on its parsed job path.
	_, err = globbed.BuildGet(ctx, BuildGetIn{Job: "https://jenkins.example.com/job/team-b/job/app/7/"})
	wantCode(t, err, errs.CodePolicyDenied)

	if len(fb.calls) != 0 {
		t.Fatalf("backend called despite denial: %v", fb.calls)
	}
}

func TestUsageErrors(t *testing.T) {
	ctx := context.Background()
	fb := &fakeBackend{}
	s := newSvc(fb, policy.Policy{Tier: policy.TierDanger})
	cases := map[string]error{}
	_, cases["empty job"] = s.JobGet(ctx, JobGetIn{Job: ""})
	_, cases["bad ref"] = s.BuildGet(ctx, BuildGetIn{Job: "a", Ref: "nope"})
	_, cases["neg limit"] = s.JobList(ctx, JobListIn{Limit: -1})
	_, cases["bad queue id"] = s.QueueGet(ctx, QueueGetIn{ID: 0})
	_, cases["stop without ref"] = s.BuildStop(ctx, BuildStopIn{Job: "a", Confirm: true})
	_, cases["replay without ref"] = s.BuildReplay(ctx, BuildReplayIn{Job: "a", Script: "x"})
	_, cases["neg timeout"] = s.BuildTrigger(ctx, BuildTriggerIn{Job: "a", TimeoutSeconds: -1})
	for name, err := range cases {
		t.Run(name, func(t *testing.T) { wantCode(t, err, errs.CodeUsage) })
	}
	if len(fb.calls) != 0 {
		t.Fatalf("backend called: %v", fb.calls)
	}
}

// ---- read ops ----

func TestWhoami(t *testing.T) {
	fb := &fakeBackend{whoami: func() (User, error) { return User{ID: "jane", FullName: "Jane Doe"}, nil }}
	out, err := newSvc(fb, policy.Policy{}).Whoami(context.Background(), WhoamiIn{})
	if err != nil || out.ID != "jane" || out.FullName != "Jane Doe" || out.URL != "https://jenkins.example.com" {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestJobListFiltersAndLimits(t *testing.T) {
	fb := &fakeBackend{listJobs: func(folder string) ([]JobSummary, error) {
		if folder != "" {
			t.Errorf("folder = %q", folder)
		}
		return []JobSummary{
			{Name: "team-a", Path: "team-a", Kind: KindFolder, Container: true},
			{Name: "team-b", Path: "team-b", Kind: KindFolder, Container: true},
			{Name: "prod", Path: "prod"},
			{Name: "tools", Path: "tools"},
		}, nil
	}}
	s := newSvc(fb, policy.Policy{Deny: policy.Rules{"jobs": {"prod", "prod/**"}}})
	out, err := s.JobList(context.Background(), JobListIn{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 3 || !out.Truncated || len(out.Jobs) != 2 || out.Jobs[0].Name != "team-a" {
		t.Fatalf("got %+v", out)
	}
}

func TestJobListFolderAndPathFill(t *testing.T) {
	fb := &fakeBackend{listJobs: func(folder string) ([]JobSummary, error) {
		return []JobSummary{{Name: "main"}, {Name: "feature%2Ffoo"}}, nil
	}}
	out, err := newSvc(fb, policy.Policy{}).JobList(context.Background(), JobListIn{Folder: "/proj/"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Folder != "proj" || out.Total != 2 || out.Truncated || out.Jobs[1].Path != "proj/feature%2Ffoo" {
		t.Fatalf("got %+v", out)
	}
}

func TestBuildGetRefFromURL(t *testing.T) {
	var got BuildRef
	fb := &fakeBackend{getBuild: func(job string, ref BuildRef) (Build, error) {
		got = ref
		return Build{Number: 42, Result: "FAILURE"}, nil
	}}
	s := newSvc(fb, policy.Policy{})
	ctx := context.Background()
	b, err := s.BuildGet(ctx, BuildGetIn{Job: "https://jenkins.example.com/job/proj/job/feature%252Ffoo/42/console"})
	if err != nil || got != "42" || b.Job != "proj/feature%2Ffoo" {
		t.Fatalf("ref=%q build=%+v err=%v", got, b, err)
	}
	// Explicit ref wins over the URL.
	if _, err := s.BuildGet(ctx, BuildGetIn{Job: "https://jenkins.example.com/job/proj/job/main/42/", Ref: "lastFailedBuild"}); err != nil || got != RefLastFailed {
		t.Fatalf("ref=%q err=%v", got, err)
	}
	// Default is last.
	if _, err := s.BuildGet(ctx, BuildGetIn{Job: "proj/main"}); err != nil || got != RefLast {
		t.Fatalf("ref=%q err=%v", got, err)
	}
}

func TestBuildStagesResolvesPermalink(t *testing.T) {
	fb := &fakeBackend{
		resolveBuild: func(job string, ref BuildRef) (int, error) { return 9, nil },
		getRun: func(job string, n int) (Run, error) {
			return Run{Status: "FAILED", DurationMs: 1200, Stages: []Stage{{ID: "6", Name: "Build", Status: "FAILED"}}}, nil
		},
	}
	out, err := newSvc(fb, policy.Policy{}).BuildStages(context.Background(), BuildStagesIn{Job: "app", Ref: "lastFailed"})
	if err != nil || out.Number != 9 || out.Status != "FAILED" || len(out.Stages) != 1 {
		t.Fatalf("got %+v, %v", out, err)
	}
	if !fb.called("ResolveBuild app lastFailed") {
		t.Fatalf("calls: %v", fb.calls)
	}
}

func TestTestFailed(t *testing.T) {
	long := strings.Repeat("x", 900)
	fb := &fakeBackend{testReport: func(job string, n int) (TestReport, error) {
		return TestReport{HasReport: true, FailCount: 3, PassCount: 10, Failed: []TestCase{
			{Name: "a", Status: "FAILED", ErrorDetails: long},
			{Name: "b", Status: "REGRESSION"},
			{Name: "c", Status: "PASSED"},
			{Name: "d", Status: "FAILED"},
		}}, nil
	}}
	out, err := newSvc(fb, policy.Policy{}).TestFailed(context.Background(), TestFailedIn{Job: "app", Ref: "5", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Failed) != 2 || !out.Truncated || out.Number != 5 || out.FailCount != 3 {
		t.Fatalf("got %+v", out)
	}
	if n := len([]rune(out.Failed[0].ErrorDetails)); n != ErrorDetailsMaxChars+3 {
		t.Fatalf("error details len %d", n)
	}
}

func TestTestFailedNoReport(t *testing.T) {
	fb := &fakeBackend{testReport: func(string, int) (TestReport, error) { return TestReport{}, nil }}
	out, err := newSvc(fb, policy.Policy{}).TestFailed(context.Background(), TestFailedIn{Job: "app", Ref: "5"})
	if err != nil || out.HasReport || out.Failed == nil || len(out.Failed) != 0 {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestQueueGet(t *testing.T) {
	fb := &fakeBackend{queueItem: func(id int64) (QueueItem, error) { return QueueItem{ID: id, Why: "Waiting"}, nil }}
	out, err := newSvc(fb, policy.Policy{}).QueueGet(context.Background(), QueueGetIn{ID: 17})
	if err != nil || out.ID != 17 || out.Why != "Waiting" {
		t.Fatalf("got %+v, %v", out, err)
	}
}

// ---- logs ----

func consoleOf(text string) func(string, int) (io.ReadCloser, error) {
	return func(string, int) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(text)), nil }
}

func TestBuildLogTail(t *testing.T) {
	fb := &fakeBackend{console: consoleOf(numberedLog(100))}
	s := newSvc(fb, policy.Policy{})
	out, err := s.BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "3", Lines: 5})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "line 96\nline 97\nline 98\nline 99\nline 100" || out.Lines != 5 || out.TotalLines != 100 || !out.Truncated {
		t.Fatalf("got %+v", out)
	}
	// Whole log fits: not truncated.
	out, err = s.BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "3", Lines: 500})
	if err != nil || out.Lines != 100 || out.Truncated || !strings.HasPrefix(out.Text, "line 1\n") {
		t.Fatalf("got lines=%d truncated=%v err=%v", out.Lines, out.Truncated, err)
	}
}

func TestBuildLogPolicyCap(t *testing.T) {
	fb := &fakeBackend{console: consoleOf(numberedLog(30))}
	s := newSvc(fb, policy.Policy{MaxLogLines: 10})
	out, err := s.BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "3", Lines: 50})
	if err != nil || out.Lines != 10 || !out.Truncated || out.TotalLines != 30 {
		t.Fatalf("got %+v, %v", out, err)
	}
	// Default (0) means the cap.
	out, err = s.BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "3"})
	if err != nil || out.Lines != 10 {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestBuildLogCRLFAndNoFinalNewline(t *testing.T) {
	fb := &fakeBackend{console: consoleOf("a\r\nb\r\nc")}
	out, err := newSvc(fb, policy.Policy{}).BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "1"})
	if err != nil || out.Text != "a\nb\nc" || out.TotalLines != 3 {
		t.Fatalf("got %q total=%d err=%v", out.Text, out.TotalLines, err)
	}
}

func TestBuildLogLongLineIsCut(t *testing.T) {
	long := strings.Repeat("y", maxLineBytes+5000)
	fb := &fakeBackend{console: consoleOf("first\n" + long + "\nlast\n")}
	var spilled string
	s := newSvc(fb, policy.Policy{}, WithSpill(func(c, p string) (string, error) { spilled = c; return "/tmp/x", nil }))
	out, err := s.BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "1"})
	if err != nil || out.TotalLines != 3 {
		t.Fatalf("got %+v, %v", out, err)
	}
	if len(spilled) != len("first\n")+maxLineBytes+len("\nlast") {
		t.Fatalf("spilled len %d", len(spilled))
	}
}

func TestBuildLogSpill(t *testing.T) {
	line := strings.Repeat("z", 99)
	fb := &fakeBackend{console: consoleOf(strings.Repeat(line+"\n", 1000))}
	var prefix, content string
	s := newSvc(fb, policy.Policy{}, WithSpill(func(c, p string) (string, error) {
		prefix, content = p, c
		return "/tmp/jk/build_log-3-1.txt", nil
	}))
	out, err := s.BuildLog(context.Background(), BuildLogIn{Job: "folder/app", Ref: "3"})
	if err != nil {
		t.Fatal(err)
	}
	if out.SpilledTo == "" || out.Text != "" || out.Lines != 1000 || len(content) != 1000*100-1 {
		t.Fatalf("got %+v (content %d)", out, len(content))
	}
	if strings.Contains(prefix, "/") {
		t.Fatalf("spill prefix contains a path separator: %q", prefix)
	}
}

func TestBuildLogRealSpill(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	fb := &fakeBackend{console: consoleOf(strings.Repeat(strings.Repeat("q", 200)+"\n", 500))}
	out, err := newSvc(fb, policy.Policy{}).BuildLog(context.Background(), BuildLogIn{Job: "a/b", Ref: "2"})
	if err != nil || out.SpilledTo == "" {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func stageBackend() *fakeBackend {
	return &fakeBackend{
		getRun: func(string, int) (Run, error) {
			return Run{Stages: []Stage{{ID: "6", Name: "Checkout"}, {ID: "12", Name: "Build"}, {ID: "30", Name: "Test"}}}, nil
		},
		getStageNodes: func(_ string, _ int, id string) ([]StageNode, error) {
			if id == "12" {
				return []StageNode{{ID: "13"}, {ID: "15"}}, nil
			}
			return nil, nil
		},
		getNodeLog: func(_ string, _ int, id string) (NodeLog, error) {
			switch id {
			case "13":
				return NodeLog{Text: "compile <ok>\nlink"}, nil
			case "15":
				return NodeLog{Text: "package\n", HasMore: true}, nil
			case "30":
				return NodeLog{Text: "test output\n"}, nil
			}
			return NodeLog{}, errs.NotFound("no node")
		},
	}
}

func TestBuildLogStage(t *testing.T) {
	fb := stageBackend()
	s := newSvc(fb, policy.Policy{})
	out, err := s.BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "4", Stage: "build"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Stage != "Build" || out.Text != "compile <ok>\nlink\npackage" || out.TotalLines != 3 || !out.Truncated {
		t.Fatalf("got %+v", out)
	}
	if fb.called("StreamConsole") {
		t.Fatal("console streamed for a stage log")
	}
	// Stage by id, falling back to the stage node itself.
	out, err = s.BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "4", Stage: "30"})
	if err != nil || out.Text != "test output" || out.Truncated {
		t.Fatalf("got %+v, %v", out, err)
	}
	_, err = s.BuildLog(context.Background(), BuildLogIn{Job: "app", Ref: "4", Stage: "Deploy"})
	e := wantCode(t, err, errs.CodeNotFound)
	if !strings.Contains(e.Hint, "Checkout, Build, Test") {
		t.Fatalf("hint %q", e.Hint)
	}
}

func TestLogSearchContext(t *testing.T) {
	log := "a\nb\nERROR one\nc\nd\ne\nERROR two\nERROR three\nf\n"
	fb := &fakeBackend{console: consoleOf(log)}
	out, err := newSvc(fb, policy.Policy{}).LogSearch(context.Background(), LogSearchIn{Job: "app", Ref: "1", Pattern: "ERROR", Context: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Matches) != 3 || out.Truncated || out.ScannedLines != 9 {
		t.Fatalf("got %+v", out)
	}
	m := out.Matches
	if m[0].Line != 3 || m[0].Before[0] != "b" || m[0].After[0] != "c" {
		t.Fatalf("m0 %+v", m[0])
	}
	if m[1].Line != 7 || len(m[1].Before) != 1 || m[1].Before[0] != "e" || m[1].After[0] != "ERROR three" {
		t.Fatalf("m1 %+v", m[1])
	}
	// The previous match line is not repeated as leading context.
	if m[2].Line != 8 || len(m[2].Before) != 0 || m[2].After[0] != "f" {
		t.Fatalf("m2 %+v", m[2])
	}
}

func TestLogSearchStopsEarly(t *testing.T) {
	cr := &countingReader{r: strings.NewReader("ERROR first\n" + numberedLog(200000))}
	fb := &fakeBackend{console: func(string, int) (io.ReadCloser, error) { return cr, nil }}
	out, err := newSvc(fb, policy.Policy{}).LogSearch(context.Background(), LogSearchIn{Job: "app", Ref: "1", Pattern: "^ERROR", MaxMatches: 1, Context: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Matches) != 1 || !out.Truncated || out.ScannedLines != 3 || len(out.Matches[0].After) != 2 {
		t.Fatalf("got %+v", out)
	}
	if cr.read > 128*1024 || !cr.closed {
		t.Fatalf("read %d bytes, closed=%v", cr.read, cr.closed)
	}
}

func TestLogSearchLineBudget(t *testing.T) {
	fb := &fakeBackend{console: consoleOf(strings.Repeat("hit\n", 100))}
	out, err := newSvc(fb, policy.Policy{MaxLogLines: 10}).LogSearch(context.Background(), LogSearchIn{Job: "app", Ref: "1", Pattern: "hit", MaxMatches: 1000})
	if err != nil || len(out.Matches) != 10 || !out.Truncated {
		t.Fatalf("got %d matches truncated=%v err=%v", len(out.Matches), out.Truncated, err)
	}
}

func TestLogSearchNoMatchesAndBadPattern(t *testing.T) {
	fb := &fakeBackend{console: consoleOf("a\nb\n")}
	s := newSvc(fb, policy.Policy{})
	out, err := s.LogSearch(context.Background(), LogSearchIn{Job: "app", Ref: "1", Pattern: "zzz"})
	if err != nil || out.Matches == nil || len(out.Matches) != 0 || out.ScannedLines != 2 || out.Truncated {
		t.Fatalf("got %+v, %v", out, err)
	}
	_, err = s.LogSearch(context.Background(), LogSearchIn{Job: "app", Ref: "1", Pattern: "("})
	wantCode(t, err, errs.CodeUsage)
	_, err = s.LogSearch(context.Background(), LogSearchIn{Job: "app", Ref: "1"})
	wantCode(t, err, errs.CodeUsage)
}

func TestLogSearchSpill(t *testing.T) {
	line := "ERR " + strings.Repeat("w", 500)
	fb := &fakeBackend{console: consoleOf(strings.Repeat(line+"\n", 300))}
	var content string
	s := newSvc(fb, policy.Policy{}, WithSpill(func(c, p string) (string, error) { content = c; return "/tmp/s", nil }))
	out, err := s.LogSearch(context.Background(), LogSearchIn{Job: "app", Ref: "1", Pattern: "ERR", MaxMatches: 200, Context: 1})
	if err != nil || out.SpilledTo != "/tmp/s" || len(out.Matches) != 200 {
		t.Fatalf("got spilled=%q matches=%d err=%v", out.SpilledTo, len(out.Matches), err)
	}
	if !strings.HasPrefix(content, "1:ERR ") || out.Matches[0].After != nil || len(out.Matches[0].Text) > 210 {
		t.Fatalf("bad spill content/matches: %q", content[:20])
	}
}

// ---- write ops ----

func TestBuildTriggerDryRun(t *testing.T) {
	fb := &fakeBackend{getJob: func(string) (Job, error) {
		return Job{Parameters: []ParamDef{{Name: "ENV"}}}, nil
	}}
	s := newSvc(fb, policy.Policy{Tier: policy.TierWrite})
	out, err := s.BuildTrigger(context.Background(), BuildTriggerIn{Job: "proj/feature%2Ffoo", Params: map[string]string{"ENV": "qa", "X": "1"}, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !out.DryRun || out.Plan == nil || out.Plan.Path != "/job/proj/job/feature%252Ffoo/buildWithParameters" || out.Plan.Params["ENV"] != "qa" || !strings.Contains(out.Plan.Note, "X") {
		t.Fatalf("got %+v plan %+v", out, out.Plan)
	}
	if fb.called("TriggerBuild") {
		t.Fatal("dry run triggered a build")
	}
}

func TestBuildTriggerParamsOnPlainJob(t *testing.T) {
	fb := &fakeBackend{getJob: func(string) (Job, error) { return Job{}, nil }}
	_, err := newSvc(fb, policy.Policy{Tier: policy.TierWrite}).BuildTrigger(context.Background(), BuildTriggerIn{Job: "app", Params: map[string]string{"A": "1"}})
	wantCode(t, err, errs.CodeUsage)
	if fb.called("TriggerBuild") {
		t.Fatal("triggered")
	}
}

func TestBuildTriggerNoWait(t *testing.T) {
	fb := &fakeBackend{
		getJob: func(string) (Job, error) { return Job{}, nil },
		trigger: func(job string, p map[string]string, parameterized bool) (int64, error) {
			if parameterized {
				t.Error("plain job triggered as parameterized")
			}
			return 77, nil
		},
	}
	out, err := newSvc(fb, policy.Policy{Tier: policy.TierWrite}).BuildTrigger(context.Background(), BuildTriggerIn{Job: "app"})
	if err != nil || out.QueueID != 77 || out.DryRun || out.Number != nil {
		t.Fatalf("got %+v, %v", out, err)
	}
}

// fakeClock advances by the requested duration on every sleep.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
	return nil
}

func TestBuildTriggerWait(t *testing.T) {
	polls := 0
	builds := 0
	fb := &fakeBackend{
		getJob:  func(string) (Job, error) { return Job{Parameters: []ParamDef{{Name: "ENV"}}}, nil },
		trigger: func(string, map[string]string, bool) (int64, error) { return 5, nil },
		queueItem: func(id int64) (QueueItem, error) {
			polls++
			if polls < 3 {
				return QueueItem{ID: id, Why: "Waiting for next available executor"}, nil
			}
			return QueueItem{ID: id, BuildNumber: intp(12), BuildURL: "https://jenkins.example.com/job/app/12/"}, nil
		},
		getBuild: func(job string, ref BuildRef) (Build, error) {
			builds++
			if ref != "12" {
				t.Errorf("ref %q", ref)
			}
			switch builds {
			case 1:
				return Build{}, errs.NotFound("not yet")
			case 2:
				return Build{Number: 12, Building: true}, nil
			}
			return Build{Number: 12, Result: "SUCCESS"}, nil
		},
	}
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	s := newSvc(fb, policy.Policy{Tier: policy.TierWrite}, WithClock(clk.Now), WithSleep(clk.Sleep), WithPollInterval(time.Second))
	out, err := s.BuildTrigger(context.Background(), BuildTriggerIn{Job: "app", Params: map[string]string{"ENV": "qa"}, Wait: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.QueueID != 5 || out.Number == nil || *out.Number != 12 || out.Result != "SUCCESS" || out.Building || out.TimedOut {
		t.Fatalf("got %+v", out)
	}
}

func TestBuildTriggerWaitTimeout(t *testing.T) {
	fb := &fakeBackend{
		getJob:    func(string) (Job, error) { return Job{}, nil },
		trigger:   func(string, map[string]string, bool) (int64, error) { return 5, nil },
		queueItem: func(id int64) (QueueItem, error) { return QueueItem{ID: id, BuildNumber: intp(3)}, nil },
		getBuild:  func(string, BuildRef) (Build, error) { return Build{Number: 3, Building: true}, nil },
	}
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	s := newSvc(fb, policy.Policy{Tier: policy.TierWrite}, WithClock(clk.Now), WithSleep(clk.Sleep), WithPollInterval(2*time.Second))
	out, err := s.BuildTrigger(context.Background(), BuildTriggerIn{Job: "app", Wait: true, TimeoutSeconds: 10})
	if err != nil || !out.TimedOut || !out.Building || *out.Number != 3 {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestBuildTriggerWaitCancelledQueue(t *testing.T) {
	fb := &fakeBackend{
		getJob:    func(string) (Job, error) { return Job{}, nil },
		trigger:   func(string, map[string]string, bool) (int64, error) { return 5, nil },
		queueItem: func(id int64) (QueueItem, error) { return QueueItem{ID: id, Cancelled: true}, nil },
	}
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	s := newSvc(fb, policy.Policy{Tier: policy.TierWrite}, WithClock(clk.Now), WithSleep(clk.Sleep))
	out, err := s.BuildTrigger(context.Background(), BuildTriggerIn{Job: "app", Wait: true})
	wantCode(t, err, errs.CodeConflict)
	if out.QueueID != 5 {
		t.Fatalf("queue id lost: %+v", out)
	}
}

func TestBuildTriggerWaitParentCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fb := &fakeBackend{
		getJob:  func(string) (Job, error) { return Job{}, nil },
		trigger: func(string, map[string]string, bool) (int64, error) { return 5, nil },
		queueItem: func(id int64) (QueueItem, error) {
			cancel()
			return QueueItem{ID: id}, nil
		},
	}
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	s := newSvc(fb, policy.Policy{Tier: policy.TierWrite}, WithClock(clk.Now), WithSleep(clk.Sleep))
	out, err := s.BuildTrigger(ctx, BuildTriggerIn{Job: "app", Wait: true})
	if err == nil || out.TimedOut {
		t.Fatalf("want error on parent cancel, got %+v", out)
	}
}

func TestBuildReplay(t *testing.T) {
	var script string
	fb := &fakeBackend{
		resolveBuild: func(string, BuildRef) (int, error) { return 8, nil },
		replay:       func(job string, n int, s string) (int64, error) { script = s; return 0, nil },
	}
	s := newSvc(fb, policy.Policy{Tier: policy.TierWrite})
	ctx := context.Background()
	out, err := s.BuildReplay(ctx, BuildReplayIn{Job: "app", Ref: "last", Script: "pipeline {}", DryRun: true})
	if err != nil || out.Plan == nil || out.Plan.Path != "/job/app/8/replay/run" || out.FromNumber != 8 || fb.called("ReplayBuild") {
		t.Fatalf("got %+v, %v", out, err)
	}
	if strings.Contains(out.Plan.Params["mainScript"], "pipeline") {
		t.Fatal("plan should not echo the script")
	}
	out, err = s.BuildReplay(ctx, BuildReplayIn{Job: "app", Ref: "8", Script: "pipeline {}"})
	if err != nil || script != "pipeline {}" || out.DryRun {
		t.Fatalf("got %+v, %v", out, err)
	}
	_, err = s.BuildReplay(ctx, BuildReplayIn{Job: "app", Ref: "8", Script: "  "})
	wantCode(t, err, errs.CodeUsage)
}

func TestBuildStop(t *testing.T) {
	stopped := 0
	fb := &fakeBackend{
		resolveBuild: func(string, BuildRef) (int, error) { return 57, nil },
		stop:         func(job string, n int) error { stopped = n; return nil },
	}
	s := newSvc(fb, policy.Policy{Tier: policy.TierDanger})
	ctx := context.Background()

	// No confirm: NEEDS_CONFIRMATION with the plan naming the resolved number.
	_, err := s.BuildStop(ctx, BuildStopIn{Job: "app", Ref: "last"})
	e := wantCode(t, err, errs.CodeNeedsConfirmation)
	plan, ok := e.Details.(*Plan)
	if !ok || plan.Path != "/job/app/57/stop" || !strings.Contains(e.Hint, `"57"`) {
		t.Fatalf("details %#v hint %q", e.Details, e.Hint)
	}
	// Dry run wins over confirm.
	out, err := s.BuildStop(ctx, BuildStopIn{Job: "app", Ref: "57", DryRun: true, Confirm: true})
	if err != nil || !out.DryRun || out.Plan == nil || stopped != 0 || fb.called("StopBuild") {
		t.Fatalf("got %+v, %v", out, err)
	}
	// Ref taken from a pasted build URL.
	out, err = s.BuildStop(ctx, BuildStopIn{Job: "https://jenkins.example.com/job/app/57/", Confirm: true})
	if err != nil || !out.Stopped || stopped != 57 {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestValidationBeforeResolve(t *testing.T) {
	ctx := context.Background()
	fb := &fakeBackend{}
	s := newSvc(fb, policy.Policy{Tier: policy.TierDanger})
	cases := map[string]error{}
	_, cases["log lines"] = s.BuildLog(ctx, BuildLogIn{Job: "app", Ref: "last", Lines: -1})
	_, cases["search pattern"] = s.LogSearch(ctx, LogSearchIn{Job: "app", Ref: "last", Pattern: "("})
	_, cases["search context"] = s.LogSearch(ctx, LogSearchIn{Job: "app", Ref: "last", Pattern: "x", Context: -1})
	_, cases["test limit"] = s.TestFailed(ctx, TestFailedIn{Job: "app", Ref: "last", Limit: -1})
	_, cases["replay script"] = s.BuildReplay(ctx, BuildReplayIn{Job: "app", Ref: "last"})
	for name, err := range cases {
		t.Run(name, func(t *testing.T) { wantCode(t, err, errs.CodeUsage) })
	}
	if len(fb.calls) != 0 {
		t.Fatalf("backend called before validation: %v", fb.calls)
	}
}

func TestBuildTriggerWaitErrorKeepsQueueID(t *testing.T) {
	fb := &fakeBackend{
		getJob:  func(string) (Job, error) { return Job{}, nil },
		trigger: func(string, map[string]string, bool) (int64, error) { return 5, nil },
		queueItem: func(int64) (QueueItem, error) {
			return QueueItem{}, errs.FromHTTPStatus(502, "")
		},
	}
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	s := newSvc(fb, policy.Policy{Tier: policy.TierWrite}, WithClock(clk.Now), WithSleep(clk.Sleep))
	_, err := s.BuildTrigger(context.Background(), BuildTriggerIn{Job: "app", Wait: true})
	e := wantCode(t, err, errs.CodeUpstream)
	d, ok := e.Details.(BuildTriggerOut)
	if e.Retryable || !ok || d.QueueID != 5 || !strings.Contains(e.Hint, "do not re-trigger") {
		t.Fatalf("got %+v", e)
	}
}

func TestJobListAncestorOfAllowGlob(t *testing.T) {
	fb := &fakeBackend{listJobs: func(folder string) ([]JobSummary, error) {
		switch folder {
		case "":
			return []JobSummary{
				{Name: "team-a", Path: "team-a", Kind: KindFolder, Container: true},
				{Name: "team-b", Path: "team-b", Kind: KindFolder, Container: true},
				{Name: "loose", Path: "loose"},
			}, nil
		case "team-a":
			return []JobSummary{
				{Name: "app", Path: "team-a/app", Kind: KindFolder, Container: true},
				{Name: "other", Path: "team-a/other", Kind: KindFolder, Container: true},
				{Name: "job", Path: "team-a/job"},
			}, nil
		}
		t.Fatalf("unexpected folder %q", folder)
		return nil, nil
	}}
	s := newSvc(fb, policy.Policy{Allow: policy.Rules{"jobs": {"team-a/app/**"}}})
	ctx := context.Background()
	root, err := s.JobList(ctx, JobListIn{})
	if err != nil || root.Total != 1 || root.Jobs[0].Path != "team-a" {
		t.Fatalf("root = %+v, %v", root, err)
	}
	sub, err := s.JobList(ctx, JobListIn{Folder: "team-a"})
	if err != nil || sub.Total != 1 || sub.Jobs[0].Path != "team-a/app" {
		t.Fatalf("team-a = %+v, %v", sub, err)
	}
	_, err = s.JobList(ctx, JobListIn{Folder: "team-b"})
	wantCode(t, err, errs.CodePolicyDenied)
	// Ancestor visibility never grants actions on the folder itself.
	_, err = s.JobGet(ctx, JobGetIn{Job: "team-a"})
	wantCode(t, err, errs.CodePolicyDenied)
}

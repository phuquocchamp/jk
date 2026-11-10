package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/errs"
)

const (
	testUser  = "jane"
	testToken = "tok-SYNTHETIC-not-a-secret-1234"
)

// seen is one request observed by the fake Jenkins.
type seen struct {
	Method  string
	Path    string // escaped path, exactly as sent
	Tree    string
	Form    map[string][]string
	AuthOK  bool
	Headers http.Header
	Cookies []*http.Cookie
}

// fakeJenkins routes on "METHOD escapedPath" (no http.ServeMux: it would clean
// and unescape paths, hiding escaping bugs).
type fakeJenkins struct {
	t      *testing.T
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	reqs   []seen
	srv    *httptest.Server
}

func newFake(t *testing.T) *fakeJenkins {
	t.Helper()
	f := &fakeJenkins{t: t, routes: map[string]http.HandlerFunc{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		s := seen{
			Method:  r.Method,
			Path:    r.URL.EscapedPath(),
			Tree:    r.URL.Query().Get("tree"),
			AuthOK:  ok && u == testUser && p == testToken,
			Headers: r.Header.Clone(),
			Cookies: r.Cookies(),
		}
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			s.Form = r.PostForm
		}
		f.mu.Lock()
		f.reqs = append(f.reqs, s)
		h := f.routes[r.Method+" "+s.Path]
		f.mu.Unlock()
		if h == nil {
			http.Error(w, "<html>no route</html>", http.StatusNotFound)
			return
		}
		h(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeJenkins) on(method, path string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[method+" "+path] = h
}

func (f *fakeJenkins) requests() []seen {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seen(nil), f.reqs...)
}

func (f *fakeJenkins) client(opts ...Option) *Client {
	return New(f.srv.URL+"/", testUser, testToken, append([]Option{WithHTTPClient(f.srv.Client())}, opts...)...)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func serveFile(t *testing.T, name string) http.HandlerFunc {
	b := fixture(t, name)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json;charset=utf-8")
		_, _ = w.Write(b)
	}
}

func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<html><body>Jenkins error page</body></html>", code)
	}
}

func wantCode(t *testing.T, err error, code errs.Code) *errs.Error {
	t.Helper()
	e, ok := errs.As(err)
	if !ok {
		t.Fatalf("want *errs.Error %s, got %v", code, err)
	}
	if e.Code != code {
		t.Fatalf("want code %s, got %s (%v)", code, e.Code, err)
	}
	return e
}

func one(t *testing.T, f *fakeJenkins) seen {
	t.Helper()
	rs := f.requests()
	if len(rs) != 1 {
		t.Fatalf("want 1 request, got %d: %+v", len(rs), rs)
	}
	if !rs[0].AuthOK {
		t.Fatalf("request without the expected basic auth")
	}
	return rs[0]
}

func intp(n int) *int { return &n }

var ctx = context.Background()

// ---------------------------------------------------------------------------

func TestWhoami(t *testing.T) {
	f := newFake(t)
	f.on("GET", "/me/api/json", serveFile(t, "whoami.json"))
	u, err := f.client().Whoami(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u != (core.User{ID: "jane", FullName: "Jane Doe"}) {
		t.Fatalf("got %+v", u)
	}
	r := one(t, f)
	if r.Tree != "id,fullName" {
		t.Fatalf("tree = %q", r.Tree)
	}
	if r.Headers.Get("User-Agent") != "jk" {
		t.Fatalf("user agent = %q", r.Headers.Get("User-Agent"))
	}
}

func TestBaseURLTrimmed(t *testing.T) {
	if got := New("https://jenkins.example.com/ci/", "", "").BaseURL(); got != "https://jenkins.example.com/ci" {
		t.Fatal(got)
	}
}

func TestContextPrefix(t *testing.T) {
	f := newFake(t)
	f.on("GET", "/ci/job/a/api/json", serveFile(t, "job.json"))
	c := New(f.srv.URL+"/ci", testUser, testToken, WithHTTPClient(f.srv.Client()))
	if _, err := c.GetJob(ctx, "a"); err != nil {
		t.Fatal(err)
	}
}

func TestListJobs(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/api/json", serveFile(t, "jobs_root.json"))
		jobs, err := f.client().ListJobs(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		want := []core.JobSummary{
			{Name: "team-a", Path: "team-a", Kind: core.KindFolder, Container: true, URL: "https://jenkins.example.com/job/team-a/"},
			{Name: "demo-app", Path: "demo-app", Kind: core.KindMultibranch, Container: true, URL: "https://jenkins.example.com/job/demo-app/"},
			{Name: "nightly", Path: "nightly", Kind: core.KindJob, Color: "red", URL: "https://jenkins.example.com/job/nightly/"},
			{Name: "odd", Path: "odd", Kind: core.KindOther, Color: "notbuilt", URL: "https://jenkins.example.com/job/odd/"},
		}
		if !reflect.DeepEqual(jobs, want) {
			t.Fatalf("got %+v", jobs)
		}
		if r := one(t, f); r.Tree != "jobs[name,_class,color,url]" {
			t.Fatalf("tree = %q", r.Tree)
		}
	})
	t.Run("multibranch", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/job/org/job/demo-app/api/json", serveFile(t, "jobs_multibranch.json"))
		jobs, err := f.client().ListJobs(ctx, "org/demo-app")
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 2 || jobs[1].Path != "org/demo-app/feature%2Ffoo" || jobs[1].Kind != core.KindPipeline || jobs[1].Container {
			t.Fatalf("got %+v", jobs)
		}
	})
	t.Run("not a folder", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/job/nightly/api/json", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"_class":"hudson.model.FreeStyleProject"}`)
		})
		_, err := f.client().ListJobs(ctx, "nightly")
		wantCode(t, err, errs.CodeUsage)
	})
	t.Run("missing folder", func(t *testing.T) {
		f := newFake(t)
		_, err := f.client().ListJobs(ctx, "nope")
		e := wantCode(t, err, errs.CodeNotFound)
		if !strings.Contains(e.Hint, "%2F") {
			t.Fatalf("hint = %q", e.Hint)
		}
	})
}

func TestGetJobEscapesBranch(t *testing.T) {
	f := newFake(t)
	f.on("GET", "/job/demo-app/job/feature%252Ffoo/api/json", serveFile(t, "job.json"))
	j, err := f.client().GetJob(ctx, "demo-app/feature%2Ffoo")
	if err != nil {
		t.Fatal(err)
	}
	r := one(t, f)
	if r.Path != "/job/demo-app/job/feature%252Ffoo/api/json" {
		t.Fatalf("path = %q", r.Path)
	}
	if !strings.HasPrefix(r.Tree, "name,url,_class,color,buildable,lastBuild[number]") ||
		!strings.Contains(r.Tree, "property[parameterDefinitions[name,type,description,defaultParameterValue[value],choices]]") {
		t.Fatalf("tree = %q", r.Tree)
	}
	want := core.Job{
		Name: "feature%2Ffoo", Path: "demo-app/feature%2Ffoo",
		URL:  "https://jenkins.example.com/job/demo-app/job/feature%252Ffoo/",
		Kind: core.KindPipeline, Class: "org.jenkinsci.plugins.workflow.job.WorkflowJob",
		Color: "red", Buildable: true,
		LastBuild: intp(7), LastSuccessfulBuild: intp(5), LastCompletedBuild: intp(6),
		Parameters: []core.ParamDef{
			{Name: "ENV", Type: "StringParameterDefinition", Description: "target env", Default: "staging"},
			{Name: "DRY", Type: "BooleanParameterDefinition", Default: true},
			{Name: "PICK", Type: "ChoiceParameterDefinition", Default: "a", Choices: []string{"a", "b"}},
			{Name: "SECRET", Type: "PasswordParameterDefinition"},
		},
	}
	if !reflect.DeepEqual(j, want) {
		t.Fatalf("got  %+v\nwant %+v", j, want)
	}
}

// A pasted job URL parsed by core.ParseJobInput round-trips to the same request path.
func TestPastedURLRoundTrip(t *testing.T) {
	f := newFake(t)
	f.on("GET", "/job/demo-app/job/feature%252Ffoo/api/json", serveFile(t, "job.json"))
	p, ref, err := core.ParseJobInput("https://jenkins.example.com/job/demo-app/job/feature%252Ffoo/7/console")
	if err != nil {
		t.Fatal(err)
	}
	if ref != "7" {
		t.Fatalf("ref = %q", ref)
	}
	if _, err := f.client().GetJob(ctx, p); err != nil {
		t.Fatal(err)
	}
}

func TestResolveBuild(t *testing.T) {
	f := newFake(t)
	f.on("GET", "/job/a/job/b/lastSuccessfulBuild/api/json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"_class":"org.jenkinsci.plugins.workflow.job.WorkflowRun","number":41}`)
	})
	c := f.client()
	n, err := c.ResolveBuild(ctx, "a/b", core.RefLastSuccessful)
	if err != nil || n != 41 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if r := one(t, f); r.Tree != "number" {
		t.Fatalf("tree = %q", r.Tree)
	}
	// Numeric refs need no request.
	if n, err := c.ResolveBuild(ctx, "a/b", "12"); err != nil || n != 12 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(f.requests()) != 1 {
		t.Fatal("numeric ref made a request")
	}
	// Missing permalink -> NOT_FOUND with a hint.
	_, err = c.ResolveBuild(ctx, "a/b", core.RefLastFailed)
	e := wantCode(t, err, errs.CodeNotFound)
	if !strings.Contains(e.Hint, "lastFailed") {
		t.Fatalf("hint = %q", e.Hint)
	}
	if got := f.requests()[1].Path; got != "/job/a/job/b/lastFailedBuild/api/json" {
		t.Fatalf("path = %q", got)
	}
}

func TestGetBuild(t *testing.T) {
	f := newFake(t)
	f.on("GET", "/job/demo-app/job/feature%252Ffoo/lastBuild/api/json", serveFile(t, "build.json"))
	b, err := f.client().GetBuild(ctx, "demo-app/feature%2Ffoo", core.RefLast)
	if err != nil {
		t.Fatal(err)
	}
	r := one(t, f)
	if !strings.Contains(r.Tree, "actions[causes[shortDescription],parameters[name,value],lastBuiltRevision[SHA1]]") ||
		!strings.Contains(r.Tree, "changeSets[items[commitId,msg,author[fullName]]]") {
		t.Fatalf("tree = %q", r.Tree)
	}
	want := core.Build{
		Job: "demo-app/feature%2Ffoo", Number: 7, Result: "FAILURE",
		DurationMs: 125000, EstimatedDurationMs: 120000,
		Timestamp: time.UnixMilli(1700000000000).UTC(),
		URL:       "https://jenkins.example.com/job/demo-app/job/feature%252Ffoo/7/",
		Causes:    []string{"Started by user Jane Doe"},
		Parameters: []core.Param{
			{Name: "ENV", Value: "staging"},
			{Name: "DRY", Value: true},
		},
		Revision: "0123456789abcdef0123456789abcdef01234567",
		Changes: []core.Change{
			{CommitID: "aaaabbbbccccddddeeeeffff0000111122223333", Message: "Fix the widget", Author: "Jane Doe"},
			{CommitID: "1111222233334444555566667777888899990000", Message: "No author"},
		},
	}
	if !reflect.DeepEqual(b, want) {
		t.Fatalf("got  %+v\nwant %+v", b, want)
	}
	if b.Timestamp.Format(time.RFC3339) != "2023-11-14T22:13:20Z" {
		t.Fatal(b.Timestamp)
	}
}

func TestGetRun(t *testing.T) {
	f := newFake(t)
	f.on("GET", "/job/demo-app/job/main/3/wfapi/describe", serveFile(t, "wfapi.json"))
	run, err := f.client().GetRun(ctx, "demo-app/main", 3)
	if err != nil {
		t.Fatal(err)
	}
	want := core.Run{ID: "3", Name: "#3", Status: "FAILED", DurationMs: 125000, Stages: []core.Stage{
		{ID: "6", Name: "Checkout", Status: "SUCCESS", StartTime: time.UnixMilli(1700000001000).UTC(), DurationMs: 3000},
		{ID: "17", Name: "Build", Status: "FAILED", StartTime: time.UnixMilli(1700000004000).UTC(), DurationMs: 120000},
	}}
	if !reflect.DeepEqual(run, want) {
		t.Fatalf("got %+v", run)
	}
	one(t, f)
}

func TestStageNodesAndLog(t *testing.T) {
	f := newFake(t)
	f.on("GET", "/job/demo-app/job/main/3/execution/node/17/wfapi/describe", serveFile(t, "stage.json"))
	f.on("GET", "/job/demo-app/job/main/3/execution/node/19/wfapi/log", serveFile(t, "nodelog.json"))
	c := f.client()
	nodes, err := c.GetStageNodes(ctx, "demo-app/main", 3, "17")
	if err != nil {
		t.Fatal(err)
	}
	want := []core.StageNode{{ID: "18", Name: "Shell Script", Status: "SUCCESS"}, {ID: "19", Name: "Shell Script", Status: "FAILED"}}
	if !reflect.DeepEqual(nodes, want) {
		t.Fatalf("got %+v", nodes)
	}
	l, err := c.GetNodeLog(ctx, "demo-app/main", 3, "19")
	if err != nil {
		t.Fatal(err)
	}
	if l.Text != "+ make test\n<x> hi & link\nexit 2\n" {
		t.Fatalf("text = %q", l.Text)
	}
	if l.NodeID != "19" || l.Status != "FAILED" || l.Length != 96 || l.HasMore || l.ConsoleURL == "" {
		t.Fatalf("got %+v", l)
	}
}

func TestPlainLogText(t *testing.T) {
	for in, want := range map[string]string{
		`&lt;x&gt; <span class="t">hi</span> &amp;`: "<x> hi &",
		`a &lt;b&gt;c&lt;/b&gt; d`:                  "a <b>c</b> d",
		`5 &lt; 6 and 7 > 3`:                        "5 < 6 and 7 > 3",
		`&quot;q&quot; &#39;s&#39;`:                 `"q" 's'`,
	} {
		if got := plainLogText(in); got != want {
			t.Errorf("plainLogText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStreamConsole(t *testing.T) {
	t.Run("streams beyond the request timeout", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/job/a/5/consoleText", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain;charset=utf-8")
			_, _ = io.WriteString(w, "line 1\n")
			w.(http.Flusher).Flush()
			select {
			case <-time.After(150 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
			_, _ = io.WriteString(w, "line 2\n")
		})
		rc, err := f.client(WithTimeout(50*time.Millisecond)).StreamConsole(ctx, "a", 5)
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		var lines []string
		sc := bufio.NewScanner(rc)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(lines, []string{"line 1", "line 2"}) {
			t.Fatalf("lines = %q", lines)
		}
	})
	t.Run("large body is not buffered", func(t *testing.T) {
		f := newFake(t)
		line := strings.Repeat("x", 99) + "\n"
		f.on("GET", "/job/a/6/consoleText", func(w http.ResponseWriter, r *http.Request) {
			for range 20000 { // ~2MB
				_, _ = io.WriteString(w, line)
			}
		})
		rc, err := f.client().StreamConsole(ctx, "a", 6)
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		n, err := io.Copy(io.Discard, rc)
		if err != nil || n != int64(len(line)*20000) {
			t.Fatalf("n=%d err=%v", n, err)
		}
	})
	t.Run("404", func(t *testing.T) {
		f := newFake(t)
		_, err := f.client().StreamConsole(ctx, "a", 99)
		wantCode(t, err, errs.CodeNotFound)
	})
	t.Run("canceled context stops the stream", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/job/a/7/consoleText", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "first\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		})
		cctx, cancel := context.WithCancel(ctx)
		rc, err := f.client().StreamConsole(cctx, "a", 7)
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		buf := make([]byte, 6)
		if _, err := io.ReadFull(rc, buf); err != nil {
			t.Fatal(err)
		}
		cancel()
		if _, err := io.ReadAll(rc); err == nil {
			t.Fatal("expected an error after cancel")
		}
	})
}

func TestGetTestReport(t *testing.T) {
	t.Run("failed only", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/job/a/3/testReport/api/json", serveFile(t, "testreport.json"))
		tr, err := f.client().GetTestReport(ctx, "a", 3)
		if err != nil {
			t.Fatal(err)
		}
		want := core.TestReport{HasReport: true, FailCount: 2, PassCount: 40, SkipCount: 3, Failed: []core.TestCase{
			{ClassName: "com.example.WidgetTest", Name: "breaks", Status: "FAILED", ErrorDetails: "expected 1 but was 2", DurationMs: 1500},
			{ClassName: "com.example.GadgetTest", Name: "regressed", Status: "REGRESSION", ErrorDetails: "boom", DurationMs: 250},
		}}
		if !reflect.DeepEqual(tr, want) {
			t.Fatalf("got %+v", tr)
		}
		if r := one(t, f); r.Tree != "failCount,passCount,skipCount,suites[cases[className,name,status,errorDetails,duration]]" {
			t.Fatalf("tree = %q", r.Tree)
		}
	})
	t.Run("404 means no report", func(t *testing.T) {
		f := newFake(t)
		tr, err := f.client().GetTestReport(ctx, "a", 3)
		if err != nil {
			t.Fatal(err)
		}
		if tr.HasReport || len(tr.Failed) != 0 {
			t.Fatalf("got %+v", tr)
		}
	})
	t.Run("500 is an error", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/job/a/3/testReport/api/json", status(500))
		_, err := f.client().GetTestReport(ctx, "a", 3)
		wantCode(t, err, errs.CodeUpstream)
	})
}

func TestGetQueueItem(t *testing.T) {
	f := newFake(t)
	f.on("GET", "/queue/item/4242/api/json", serveFile(t, "queue_waiting.json"))
	c := f.client()
	q, err := c.GetQueueItem(ctx, 4242)
	if err != nil {
		t.Fatal(err)
	}
	if q.ID != 4242 || q.BuildNumber != nil || !strings.HasPrefix(q.Why, "In the quiet period") || q.TaskName != "main" {
		t.Fatalf("got %+v", q)
	}
	if r := one(t, f); r.Tree != "id,why,blocked,cancelled,executable[number,url],task[name,url]" {
		t.Fatalf("tree = %q", r.Tree)
	}
	f.on("GET", "/queue/item/4242/api/json", serveFile(t, "queue_left.json"))
	q, err = c.GetQueueItem(ctx, 4242)
	if err != nil {
		t.Fatal(err)
	}
	if q.BuildNumber == nil || *q.BuildNumber != 8 || q.BuildURL != "https://jenkins.example.com/job/demo-app/job/main/8/" {
		t.Fatalf("got %+v", q)
	}
	_, err = c.GetQueueItem(ctx, 1)
	if e := wantCode(t, err, errs.CodeNotFound); e.Hint == "" {
		t.Fatal("missing hint")
	}
}

func TestHTTPErrorMapping(t *testing.T) {
	cases := []struct {
		status    int
		code      errs.Code
		retryable bool
	}{
		{401, errs.CodeAuthRequired, false},
		{403, errs.CodeForbidden, false},
		{404, errs.CodeNotFound, false},
		{409, errs.CodeConflict, false},
		{400, errs.CodeUpstream, false},
		{500, errs.CodeUpstream, true},
		{502, errs.CodeUpstream, true},
		{503, errs.CodeUpstream, true},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			f := newFake(t)
			f.on("GET", "/me/api/json", status(tc.status))
			f.on("GET", "/job/a/api/json", status(tc.status))
			f.on("POST", "/job/a/42/stop", status(tc.status))
			c := f.client()
			_, err := c.Whoami(ctx)
			e := wantCode(t, err, tc.code)
			if e.Retryable != tc.retryable {
				t.Fatalf("retryable = %v", e.Retryable)
			}
			if strings.Contains(e.Message, "<html>") || !strings.Contains(e.Message, "/me/api/json") {
				t.Fatalf("message = %q", e.Message)
			}
			_, err = c.GetJob(ctx, "a")
			wantCode(t, err, tc.code)
			if tc.status != 403 { // POST 403 triggers the crumb path, tested separately
				err = c.StopBuild(ctx, "a", 42)
				wantCode(t, err, tc.code)
			}
		})
	}
}

func TestNonJSONResponse(t *testing.T) {
	f := newFake(t)
	f.on("GET", "/me/api/json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<html>Sign in</html>")
	})
	_, err := f.client().Whoami(ctx)
	e := wantCode(t, err, errs.CodeUpstream)
	if e.Retryable || e.Hint == "" {
		t.Fatalf("got %+v", e)
	}
}

func TestTimeoutAndTransportErrors(t *testing.T) {
	t.Run("timeout is retryable", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/me/api/json", func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
			}
		})
		_, err := f.client(WithTimeout(50 * time.Millisecond)).Whoami(ctx)
		if e := wantCode(t, err, errs.CodeUpstream); !e.Retryable {
			t.Fatal("timeout should be retryable")
		}
	})
	t.Run("canceled is not retryable", func(t *testing.T) {
		f := newFake(t)
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := f.client().Whoami(cctx)
		if e := wantCode(t, err, errs.CodeUpstream); e.Retryable {
			t.Fatal("cancel should not be retryable")
		}
	})
	t.Run("connection refused", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		u := srv.URL
		srv.Close()
		_, err := New(u, testUser, testToken).Whoami(ctx)
		if e := wantCode(t, err, errs.CodeUpstream); !e.Retryable || strings.Contains(e.Error(), testToken) {
			t.Fatalf("got %+v", e)
		}
	})
	t.Run("bad base url", func(t *testing.T) {
		_, err := New("jenkins.example.com", "", "").Whoami(ctx)
		wantCode(t, err, errs.CodeUsage)
	})
}

func TestTriggerBuild(t *testing.T) {
	t.Run("plain build, absolute Location", func(t *testing.T) {
		f := newFake(t)
		f.on("POST", "/job/team-a/job/app/build", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "https://jenkins.example.com/queue/item/1234/")
			w.WriteHeader(http.StatusCreated)
		})
		id, err := f.client().TriggerBuild(ctx, "team-a/app", nil, false)
		if err != nil || id != 1234 {
			t.Fatalf("id=%d err=%v", id, err)
		}
		r := one(t, f)
		if len(r.Form) != 0 {
			t.Fatalf("form = %v", r.Form)
		}
	})
	t.Run("parameterized, relative Location via 302 not followed", func(t *testing.T) {
		f := newFake(t)
		f.on("POST", "/job/demo-app/job/feature%252Ffoo/buildWithParameters", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/queue/item/77")
			w.WriteHeader(http.StatusFound)
		})
		id, err := f.client().TriggerBuild(ctx, "demo-app/feature%2Ffoo", map[string]string{"ENV": "qa", "MSG": "a&b=c d"}, true)
		if err != nil || id != 77 {
			t.Fatalf("id=%d err=%v", id, err)
		}
		r := one(t, f)
		if r.Form["ENV"][0] != "qa" || r.Form["MSG"][0] != "a&b=c d" {
			t.Fatalf("form = %v", r.Form)
		}
		if r.Headers.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatal(r.Headers.Get("Content-Type"))
		}
	})
	t.Run("missing Location", func(t *testing.T) {
		f := newFake(t)
		f.on("POST", "/job/a/build", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) })
		_, err := f.client().TriggerBuild(ctx, "a", nil, false)
		wantCode(t, err, errs.CodeUpstream)
	})
	t.Run("missing job", func(t *testing.T) {
		f := newFake(t)
		_, err := f.client().TriggerBuild(ctx, "nope", nil, false)
		wantCode(t, err, errs.CodeNotFound)
	})
}

func TestParseQueueID(t *testing.T) {
	cases := map[string]int64{
		"https://jenkins.example.com/queue/item/1234/": 1234,
		"https://jenkins.example.com/ci/queue/item/5":  5,
		"/queue/item/77/":                    77,
		"https://jenkins.example.com/job/a/": 0,
		"":                                   0,
		"https://jenkins.example.com/queue/item/abc/":     0,
		"https://jenkins.example.com/queue/item/0/":       0,
		"https://proxy.example.com/x/queue/item/999/?a=b": 999,
	}
	for in, want := range cases {
		got, ok := parseQueueID(in)
		if got != want || ok != (want != 0) {
			t.Errorf("parseQueueID(%q) = %d,%v want %d", in, got, ok, want)
		}
	}
}

func TestCrumbFallback(t *testing.T) {
	t.Run("403 then crumb then success", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/crumbIssuer/api/json", func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID.abc", Value: "sess-1"})
			_, _ = io.WriteString(w, `{"_class":"hudson.security.csrf.DefaultCrumbIssuer","crumb":"crumb-xyz","crumbRequestField":"Jenkins-Crumb"}`)
		})
		f.on("POST", "/job/a/build", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Jenkins-Crumb") != "crumb-xyz" {
				http.Error(w, "No valid crumb", http.StatusForbidden)
				return
			}
			if ck, err := r.Cookie("JSESSIONID.abc"); err != nil || ck.Value != "sess-1" {
				http.Error(w, "no session", http.StatusForbidden)
				return
			}
			w.Header().Set("Location", "/queue/item/9/")
			w.WriteHeader(http.StatusCreated)
		})
		id, err := f.client().TriggerBuild(ctx, "a", nil, false)
		if err != nil || id != 9 {
			t.Fatalf("id=%d err=%v", id, err)
		}
		rs := f.requests()
		if len(rs) != 3 || rs[0].Method != "POST" || rs[1].Path != "/crumbIssuer/api/json" || rs[2].Method != "POST" {
			t.Fatalf("requests = %+v", rs)
		}
	})
	t.Run("crumb body is re-sent", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/crumbIssuer/api/json", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"crumb":"c","crumbRequestField":"Jenkins-Crumb"}`)
		})
		f.on("POST", "/job/a/buildWithParameters", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Jenkins-Crumb") == "" {
				http.Error(w, "No valid crumb", http.StatusForbidden)
				return
			}
			w.Header().Set("Location", "/queue/item/10/")
			w.WriteHeader(http.StatusCreated)
		})
		if _, err := f.client().TriggerBuild(ctx, "a", map[string]string{"K": "v"}, true); err != nil {
			t.Fatal(err)
		}
		if rs := f.requests(); rs[2].Form["K"][0] != "v" {
			t.Fatalf("retry form = %v", rs[2].Form)
		}
	})
	t.Run("403 twice gives FORBIDDEN after one crumb call", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/crumbIssuer/api/json", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"crumb":"c","crumbRequestField":"Jenkins-Crumb"}`)
		})
		f.on("POST", "/job/a/1/stop", status(403))
		err := f.client().StopBuild(ctx, "a", 1)
		wantCode(t, err, errs.CodeForbidden)
		crumbs := 0
		for _, r := range f.requests() {
			if r.Path == "/crumbIssuer/api/json" {
				crumbs++
			}
		}
		if crumbs != 1 || len(f.requests()) != 3 {
			t.Fatalf("requests = %+v", f.requests())
		}
	})
	t.Run("crumb issuer missing keeps FORBIDDEN", func(t *testing.T) {
		f := newFake(t)
		f.on("POST", "/job/a/1/stop", status(403))
		err := f.client().StopBuild(ctx, "a", 1)
		wantCode(t, err, errs.CodeForbidden)
		if len(f.requests()) != 2 {
			t.Fatalf("requests = %+v", f.requests())
		}
	})
	t.Run("GET 403 does not fetch a crumb", func(t *testing.T) {
		f := newFake(t)
		f.on("GET", "/me/api/json", status(403))
		_, err := f.client().Whoami(ctx)
		wantCode(t, err, errs.CodeForbidden)
		one(t, f)
	})
}

func TestReplayBuild(t *testing.T) {
	script := "pipeline {\n  agent any\n  stages { stage('x') { steps { echo \"hi \\\"there\\\"\" } } }\n}\n"
	t.Run("redirect to job page", func(t *testing.T) {
		f := newFake(t)
		f.on("POST", "/job/demo-app/job/main/3/replay/run", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "https://jenkins.example.com/job/demo-app/job/main/")
			w.WriteHeader(http.StatusFound)
		})
		id, err := f.client().ReplayBuild(ctx, "demo-app/main", 3, script)
		if err != nil || id != 0 {
			t.Fatalf("id=%d err=%v", id, err)
		}
		r := one(t, f)
		if r.Form["mainScript"][0] != script {
			t.Fatalf("mainScript = %q", r.Form["mainScript"])
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(r.Form["json"][0]), &payload); err != nil || payload["mainScript"] != script {
			t.Fatalf("json = %q err=%v", r.Form["json"], err)
		}
		// The redirect must not be followed.
		if len(f.requests()) != 1 {
			t.Fatal("redirect followed")
		}
	})
	t.Run("queue location reported", func(t *testing.T) {
		f := newFake(t)
		f.on("POST", "/job/a/3/replay/run", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/queue/item/55/")
			w.WriteHeader(http.StatusCreated)
		})
		id, err := f.client().ReplayBuild(ctx, "a", 3, script)
		if err != nil || id != 55 {
			t.Fatalf("id=%d err=%v", id, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		f := newFake(t)
		_, err := f.client().ReplayBuild(ctx, "a", 3, script)
		wantCode(t, err, errs.CodeNotFound)
	})
	t.Run("5xx is not retryable", func(t *testing.T) {
		f := newFake(t)
		f.on("POST", "/job/a/3/replay/run", status(500))
		_, err := f.client().ReplayBuild(ctx, "a", 3, script)
		if e := wantCode(t, err, errs.CodeUpstream); e.Retryable || e.Hint == "" {
			t.Fatalf("got %+v", e)
		}
	})
}

func TestStopBuild(t *testing.T) {
	f := newFake(t)
	f.on("POST", "/job/demo-app/job/feature%252Ffoo/12/stop", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://jenkins.example.com/job/demo-app/job/feature%252Ffoo/12/")
		w.WriteHeader(http.StatusFound)
	})
	if err := f.client().StopBuild(ctx, "demo-app/feature%2Ffoo", 12); err != nil {
		t.Fatal(err)
	}
	r := one(t, f)
	if r.Method != "POST" || r.Path != "/job/demo-app/job/feature%252Ffoo/12/stop" {
		t.Fatalf("got %+v", r)
	}
}

// The debug log never contains the token or the Authorization header.
func TestLogsHaveNoCredentials(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f := newFake(t)
	f.on("GET", "/me/api/json", serveFile(t, "whoami.json"))
	f.on("GET", "/crumbIssuer/api/json", status(404))
	f.on("POST", "/job/a/build", status(403))
	c := f.client(WithLogger(log))
	_, _ = c.Whoami(ctx)
	_, _ = c.TriggerBuild(ctx, "a", nil, false)
	_, _ = c.GetJob(ctx, "missing")
	out := buf.String()
	if out == "" {
		t.Fatal("expected debug output")
	}
	for _, bad := range []string{testToken, "Authorization", "Basic "} {
		if strings.Contains(out, bad) {
			t.Fatalf("log contains %q:\n%s", bad, out)
		}
	}
}

func TestNoAuthWhenNoCredentials(t *testing.T) {
	f := newFake(t)
	f.on("GET", "/me/api/json", serveFile(t, "whoami.json"))
	c := New(f.srv.URL, "", "", WithHTTPClient(f.srv.Client()))
	if _, err := c.Whoami(ctx); err != nil {
		t.Fatal(err)
	}
	if h := f.requests()[0].Headers.Get("Authorization"); h != "" {
		t.Fatal("unexpected Authorization header")
	}
}

func TestNonIdempotentPostNotRetryable(t *testing.T) {
	t.Run("trigger timeout after send", func(t *testing.T) {
		f := newFake(t)
		f.on("POST", "/job/a/build", func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		})
		_, err := f.client(WithTimeout(50*time.Millisecond)).TriggerBuild(ctx, "a", nil, false)
		e := wantCode(t, err, errs.CodeUpstream)
		if e.Retryable || !strings.Contains(e.Hint, "job_get/build_get") {
			t.Fatalf("got %+v", e)
		}
	})
	t.Run("trigger 503", func(t *testing.T) {
		f := newFake(t)
		f.on("POST", "/job/a/buildWithParameters", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})
		_, err := f.client().TriggerBuild(ctx, "a", map[string]string{"X": "1"}, true)
		if e := wantCode(t, err, errs.CodeUpstream); e.Retryable {
			t.Fatalf("got %+v", e)
		}
	})
	t.Run("replay timeout after send", func(t *testing.T) {
		f := newFake(t)
		f.on("POST", "/job/a/3/replay/run", func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		})
		_, err := f.client(WithTimeout(50*time.Millisecond)).ReplayBuild(ctx, "a", 3, "pipeline {}")
		if e := wantCode(t, err, errs.CodeUpstream); e.Retryable {
			t.Fatalf("got %+v", e)
		}
	})
	t.Run("trigger connection refused stays retryable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		u := srv.URL
		srv.Close()
		_, err := New(u, testUser, testToken).TriggerBuild(ctx, "a", nil, false)
		if e := wantCode(t, err, errs.CodeUpstream); !e.Retryable {
			t.Fatalf("got %+v", e)
		}
	})
	t.Run("stop 503 stays retryable", func(t *testing.T) {
		f := newFake(t)
		f.on("POST", "/job/a/3/stop", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})
		err := f.client().StopBuild(ctx, "a", 3)
		if e := wantCode(t, err, errs.CodeUpstream); !e.Retryable {
			t.Fatalf("got %+v", e)
		}
	})
}

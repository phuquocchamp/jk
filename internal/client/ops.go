package client

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/errs"
)

// Tree queries; keep in sync with docs/design.md "Jenkins API facts".
const (
	treeWhoami = "id,fullName"
	treeJobs   = "jobs[name,_class,color,url]"
	treeJob    = "name,url,_class,color,buildable," +
		"lastBuild[number],lastSuccessfulBuild[number],lastFailedBuild[number],lastCompletedBuild[number]," +
		"property[parameterDefinitions[name,type,description,defaultParameterValue[value],choices]]"
	treeBuild = "number,result,building,duration,estimatedDuration,timestamp,url," +
		"actions[causes[shortDescription],parameters[name,value],lastBuiltRevision[SHA1]]," +
		"changeSets[items[commitId,msg,author[fullName]]]"
	treeBuildNumber = "number"
	treeTests       = "failCount,passCount,skipCount,suites[cases[className,name,status,errorDetails,duration]]"
	treeQueue       = "id,why,blocked,cancelled,executable[number,url],task[name,url]"
)

// Whoami: GET /me/api/json?tree=id,fullName
func (c *Client) Whoami(ctx context.Context) (core.User, error) {
	var r struct {
		ID       string `json:"id"`
		FullName string `json:"fullName"`
	}
	if err := c.getJSON(ctx, "/me/api/json", treeQuery(treeWhoami), &r); err != nil {
		return core.User{}, err
	}
	return core.User{ID: r.ID, FullName: r.FullName}, nil
}

// jobKind maps a Jenkins _class to a core kind and whether it is a container.
func jobKind(class string) (string, bool) {
	switch {
	case strings.HasSuffix(class, ".WorkflowMultiBranchProject"):
		return core.KindMultibranch, true
	case strings.HasSuffix(class, ".WorkflowJob"):
		return core.KindPipeline, false
	case strings.HasSuffix(class, "Folder"): // cloudbees Folder, OrganizationFolder
		return core.KindFolder, true
	case strings.HasSuffix(class, "Project"), strings.HasSuffix(class, "Job"):
		return core.KindJob, false
	}
	return core.KindOther, false
}

func joinPath(folder, name string) string {
	if folder == "" {
		return name
	}
	return folder + "/" + name
}

// ListJobs lists direct children of folder ("" = root). A non-container job
// (no "jobs" field in the response) is a USAGE error.
func (c *Client) ListJobs(ctx context.Context, folder string) ([]core.JobSummary, error) {
	var r struct {
		Jobs *[]struct {
			Class string `json:"_class"`
			Name  string `json:"name"`
			Color string `json:"color"`
			URL   string `json:"url"`
		} `json:"jobs"`
	}
	path := "/api/json"
	if folder != "" {
		path = core.JobURLPath(folder) + "api/json"
	}
	if err := c.getJSON(ctx, path, treeQuery(treeJobs), &r); err != nil {
		return nil, withHint(err, errs.CodeNotFound, jobNotFoundHint)
	}
	if r.Jobs == nil {
		return nil, errs.Usagef("%q is not a folder or multibranch project", folder).
			WithHint("use job_get for a single job")
	}
	out := make([]core.JobSummary, 0, len(*r.Jobs))
	for _, j := range *r.Jobs {
		kind, container := jobKind(j.Class)
		out = append(out, core.JobSummary{
			Name:      j.Name,
			Path:      joinPath(folder, j.Name),
			Kind:      kind,
			Container: container,
			Color:     j.Color,
			URL:       j.URL,
		})
	}
	return out, nil
}

type numberRef struct {
	Number int `json:"number"`
}

func (n *numberRef) ptr() *int {
	if n == nil {
		return nil
	}
	v := n.Number
	return &v
}

// shortClass returns the simple class name of a Java class.
func shortClass(class string) string {
	if i := strings.LastIndexAny(class, ".$"); i >= 0 {
		return class[i+1:]
	}
	return class
}

// GetJob returns job details incl. permalinks and parameter definitions.
func (c *Client) GetJob(ctx context.Context, job string) (core.Job, error) {
	var r struct {
		Class               string     `json:"_class"`
		Name                string     `json:"name"`
		URL                 string     `json:"url"`
		Color               string     `json:"color"`
		Buildable           bool       `json:"buildable"`
		LastBuild           *numberRef `json:"lastBuild"`
		LastSuccessfulBuild *numberRef `json:"lastSuccessfulBuild"`
		LastFailedBuild     *numberRef `json:"lastFailedBuild"`
		LastCompletedBuild  *numberRef `json:"lastCompletedBuild"`
		Property            []struct {
			ParameterDefinitions []struct {
				Class                 string `json:"_class"`
				Name                  string `json:"name"`
				Type                  string `json:"type"`
				Description           string `json:"description"`
				DefaultParameterValue *struct {
					Value any `json:"value"`
				} `json:"defaultParameterValue"`
				Choices []string `json:"choices"`
			} `json:"parameterDefinitions"`
		} `json:"property"`
	}
	if err := c.getJSON(ctx, core.JobURLPath(job)+"api/json", treeQuery(treeJob), &r); err != nil {
		return core.Job{}, withHint(err, errs.CodeNotFound, jobNotFoundHint)
	}
	kind, _ := jobKind(r.Class)
	out := core.Job{
		Name:                r.Name,
		Path:                job,
		URL:                 r.URL,
		Kind:                kind,
		Class:               r.Class,
		Color:               r.Color,
		Buildable:           r.Buildable,
		LastBuild:           r.LastBuild.ptr(),
		LastSuccessfulBuild: r.LastSuccessfulBuild.ptr(),
		LastFailedBuild:     r.LastFailedBuild.ptr(),
		LastCompletedBuild:  r.LastCompletedBuild.ptr(),
	}
	for _, p := range r.Property {
		for _, d := range p.ParameterDefinitions {
			typ := d.Type
			if typ == "" {
				typ = shortClass(d.Class)
			}
			pd := core.ParamDef{Name: d.Name, Type: typ, Description: d.Description, Choices: d.Choices}
			if d.DefaultParameterValue != nil {
				pd.Default = d.DefaultParameterValue.Value
			}
			out.Parameters = append(out.Parameters, pd)
		}
	}
	return out, nil
}

// permalinkHint explains a 404 on a permalink such as lastFailedBuild.
func refNotFound(err error, ref core.BuildRef) error {
	if _, isNum := ref.Number(); isNum {
		return withHint(err, errs.CodeNotFound, "the build does not exist (it may have been discarded) or the job path is wrong")
	}
	return withHint(err, errs.CodeNotFound,
		"the job has no "+string(ref)+" build yet, or the job path is wrong ("+jobNotFoundHint+")")
}

// ResolveBuild turns a ref into a build number. Numbers are returned without a request.
func (c *Client) ResolveBuild(ctx context.Context, job string, ref core.BuildRef) (int, error) {
	if n, ok := ref.Number(); ok {
		return n, nil
	}
	var r numberRef
	if err := c.getJSON(ctx, buildPath(job, ref.Segment())+"api/json", treeQuery(treeBuildNumber), &r); err != nil {
		return 0, refNotFound(err, ref)
	}
	if r.Number <= 0 {
		return 0, errs.Newf(errs.CodeUpstream, "Jenkins returned no build number for %s %s", job, ref)
	}
	return r.Number, nil
}

func millis(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

// GetBuild returns build details; ref may be a number or permalink.
func (c *Client) GetBuild(ctx context.Context, job string, ref core.BuildRef) (core.Build, error) {
	var r struct {
		Number            int    `json:"number"`
		Result            string `json:"result"`
		Building          bool   `json:"building"`
		Duration          int64  `json:"duration"`
		EstimatedDuration int64  `json:"estimatedDuration"`
		Timestamp         int64  `json:"timestamp"`
		URL               string `json:"url"`
		Actions           []struct {
			Causes []struct {
				ShortDescription string `json:"shortDescription"`
			} `json:"causes"`
			Parameters []struct {
				Name  string `json:"name"`
				Value any    `json:"value"`
			} `json:"parameters"`
			LastBuiltRevision *struct {
				SHA1 string `json:"SHA1"`
			} `json:"lastBuiltRevision"`
		} `json:"actions"`
		ChangeSets []struct {
			Items []struct {
				CommitID string `json:"commitId"`
				Msg      string `json:"msg"`
				Author   *struct {
					FullName string `json:"fullName"`
				} `json:"author"`
			} `json:"items"`
		} `json:"changeSets"`
	}
	if err := c.getJSON(ctx, buildPath(job, ref.Segment())+"api/json", treeQuery(treeBuild), &r); err != nil {
		return core.Build{}, refNotFound(err, ref)
	}
	b := core.Build{
		Job:                 job,
		Number:              r.Number,
		Result:              r.Result,
		Building:            r.Building,
		DurationMs:          r.Duration,
		EstimatedDurationMs: r.EstimatedDuration,
		Timestamp:           millis(r.Timestamp),
		URL:                 r.URL,
	}
	// Many action entries are empty {}; the zero-valued fields skip them.
	for _, a := range r.Actions {
		for _, cs := range a.Causes {
			if cs.ShortDescription != "" {
				b.Causes = append(b.Causes, cs.ShortDescription)
			}
		}
		for _, p := range a.Parameters {
			b.Parameters = append(b.Parameters, core.Param{Name: p.Name, Value: p.Value})
		}
		if b.Revision == "" && a.LastBuiltRevision != nil {
			b.Revision = a.LastBuiltRevision.SHA1
		}
	}
	for _, cs := range r.ChangeSets {
		for _, it := range cs.Items {
			ch := core.Change{CommitID: it.CommitID, Message: it.Msg}
			if it.Author != nil {
				ch.Author = it.Author.FullName
			}
			b.Changes = append(b.Changes, ch)
		}
	}
	return b, nil
}

func runPath(job string, number int) string {
	return buildPath(job, strconv.Itoa(number))
}

// GetRun: GET /job/X/<n>/wfapi/describe
func (c *Client) GetRun(ctx context.Context, job string, number int) (core.Run, error) {
	var r struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Status         string `json:"status"`
		DurationMillis int64  `json:"durationMillis"`
		Stages         []struct {
			ID              string    `json:"id"`
			Name            string    `json:"name"`
			Status          string    `json:"status"`
			StartTimeMillis int64     `json:"startTimeMillis"`
			DurationMillis  int64     `json:"durationMillis"`
			Error           *wfapiErr `json:"error"`
		} `json:"stages"`
	}
	if err := c.getJSON(ctx, runPath(job, number)+"wfapi/describe", nil, &r); err != nil {
		return core.Run{}, withHint(err, errs.CodeNotFound,
			"no pipeline data for this build: the job may not be a Pipeline, or the build/job path does not exist")
	}
	run := core.Run{ID: r.ID, Name: r.Name, Status: r.Status, DurationMs: r.DurationMillis, Stages: []core.Stage{}}
	for _, s := range r.Stages {
		run.Stages = append(run.Stages, core.Stage{
			ID: s.ID, Name: s.Name, Status: s.Status,
			StartTime: millis(s.StartTimeMillis), DurationMs: s.DurationMillis, Error: s.Error.stageError(),
		})
	}
	return run, nil
}

func nodePath(job string, number int, nodeID string) string {
	return runPath(job, number) + "execution/node/" + url.PathEscape(nodeID) + "/"
}

// GetStageNodes: GET /job/X/<n>/execution/node/<stageID>/wfapi/describe -> stageFlowNodes.
func (c *Client) GetStageNodes(ctx context.Context, job string, number int, stageID string) ([]core.StageNode, error) {
	var r struct {
		StageFlowNodes []struct {
			ID                   string    `json:"id"`
			Name                 string    `json:"name"`
			Status               string    `json:"status"`
			ParameterDescription string    `json:"parameterDescription"`
			Error                *wfapiErr `json:"error"`
		} `json:"stageFlowNodes"`
	}
	if err := c.getJSON(ctx, nodePath(job, number, stageID)+"wfapi/describe", nil, &r); err != nil {
		return nil, err
	}
	out := make([]core.StageNode, 0, len(r.StageFlowNodes))
	for _, n := range r.StageFlowNodes {
		out = append(out, core.StageNode{ID: n.ID, Name: n.Name, Status: n.Status,
			Command: n.ParameterDescription, Error: n.Error.stageError()})
	}
	return out, nil
}

// wfapiErr is the "error" object wfapi attaches to failed stages and nodes.
type wfapiErr struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (e *wfapiErr) stageError() *core.StageError {
	if e == nil || (e.Message == "" && e.Type == "") {
		return nil
	}
	return &core.StageError{Type: e.Type, Message: e.Message}
}

// pgvNotFoundHint explains a 404 from the Pipeline Graph View endpoints.
const pgvNotFoundHint = "the Pipeline Graph View plugin is not installed, or the build is not a Pipeline"

// graphStage is one node of {run}/stages/tree.
type graphStage struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Type                string       `json:"type"`
	State               string       `json:"state"`
	StartTimeMillis     int64        `json:"startTimeMillis"`
	TotalDurationMillis int64        `json:"totalDurationMillis"`
	Synthetic           bool         `json:"synthetic"`
	Placeholder         bool         `json:"placeholder"`
	Children            []graphStage `json:"children"`
}

func (g graphStage) core() core.GraphStage {
	out := core.GraphStage{ID: g.ID, Name: g.Name, Type: g.Type, State: g.State,
		StartTime: millis(g.StartTimeMillis), DurationMs: g.TotalDurationMillis}
	for _, ch := range g.Children {
		if ch.Placeholder {
			continue
		}
		out.Children = append(out.Children, ch.core())
	}
	return out
}

// GetStageTree: GET /job/X/<n>/stages/tree (Pipeline Graph View).
func (c *Client) GetStageTree(ctx context.Context, job string, number int) ([]core.GraphStage, error) {
	var r struct {
		Data *struct {
			Stages []graphStage `json:"stages"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, runPath(job, number)+"stages/tree", nil, &r); err != nil {
		return nil, withHint(err, errs.CodeNotFound, pgvNotFoundHint)
	}
	if r.Data == nil {
		return nil, errs.New(errs.CodeNotFound, "stages/tree returned no data").WithHint(pgvNotFoundHint)
	}
	out := make([]core.GraphStage, 0, len(r.Data.Stages))
	for _, g := range r.Data.Stages {
		if g.Placeholder {
			continue
		}
		out = append(out, g.core())
	}
	return out, nil
}

// GetSteps: GET /job/X/<n>/stages/allSteps (Pipeline Graph View).
func (c *Client) GetSteps(ctx context.Context, job string, number int) ([]core.GraphStep, error) {
	var r struct {
		Data *struct {
			Steps []struct {
				ID                  string `json:"id"`
				Name                string `json:"name"`
				Title               string `json:"title"`
				StageID             string `json:"stageId"`
				State               string `json:"state"`
				TotalDurationMillis int64  `json:"totalDurationMillis"`
			} `json:"steps"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, runPath(job, number)+"stages/allSteps", nil, &r); err != nil {
		return nil, withHint(err, errs.CodeNotFound, pgvNotFoundHint)
	}
	if r.Data == nil {
		return nil, errs.New(errs.CodeNotFound, "stages/allSteps returned no data").WithHint(pgvNotFoundHint)
	}
	out := make([]core.GraphStep, 0, len(r.Data.Steps))
	for _, st := range r.Data.Steps {
		name := st.Name
		if name == "" {
			name = st.Title
		}
		out = append(out, core.GraphStep{ID: st.ID, Name: name, StageID: st.StageID, State: st.State, DurationMs: st.TotalDurationMillis})
	}
	return out, nil
}

// GetStepError: GET /job/X/<n>/stages/exceptionText?nodeId=<id> (plain text).
func (c *Client) GetStepError(ctx context.Context, job string, number int, nodeID string) (string, error) {
	b, err := c.getText(ctx, runPath(job, number)+"stages/exceptionText", url.Values{"nodeId": {nodeID}}, 64<<10)
	if err != nil {
		return "", withHint(err, errs.CodeNotFound, pgvNotFoundHint)
	}
	return string(b), nil
}

// StreamNodeLog: GET /job/X/<n>/execution/node/<id>/log/logText/progressiveText?start=0.
// Unlike wfapi/log (capped at ~10KB) this returns the node's full plain text.
func (c *Client) StreamNodeLog(ctx context.Context, job string, number int, nodeID string) (io.ReadCloser, error) {
	rc, _, err := c.stream(ctx, http.MethodGet, nodePath(job, number, nodeID)+"log/logText/progressiveText", url.Values{"start": {"0"}})
	if err != nil {
		return nil, withHint(err, errs.CodeNotFound, "the flow node has no log, or the build/job path does not exist")
	}
	return rc, nil
}

// ConsoleSize: HEAD /job/X/<n>/logText/progressiveText -> X-Text-Size. (A GET
// with a start beyond the end returns the whole log, so HEAD is used.)
func (c *Client) ConsoleSize(ctx context.Context, job string, number int) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	rc, h, err := c.stream(ctx, http.MethodHead, runPath(job, number)+"logText/progressiveText", url.Values{"start": {"0"}})
	if err != nil {
		return 0, refNotFound(err, core.BuildRef(strconv.Itoa(number)))
	}
	rc.Close()
	size, perr := strconv.ParseInt(h.Get("X-Text-Size"), 10, 64)
	if perr != nil || size < 0 {
		return 0, errs.New(errs.CodeUpstream, "Jenkins sent no X-Text-Size for the console log")
	}
	return size, nil
}

// StreamConsoleFrom: GET /job/X/<n>/logText/progressiveText?start=<byte>.
func (c *Client) StreamConsoleFrom(ctx context.Context, job string, number int, start int64) (io.ReadCloser, error) {
	rc, _, err := c.stream(ctx, http.MethodGet, runPath(job, number)+"logText/progressiveText",
		url.Values{"start": {strconv.FormatInt(start, 10)}})
	if err != nil {
		return nil, refNotFound(err, core.BuildRef(strconv.Itoa(number)))
	}
	return rc, nil
}

// replayTextareaRE matches the script fields of the replay form:
// <textarea ... name="_.mainScript" ...>TEXT</textarea> (loaded scripts use
// their own names). The text is HTML-escaped.
var replayTextareaRE = regexp.MustCompile(`(?s)<textarea\b[^>]*\bname="_\.([^"]+)"[^>]*>(.*?)</textarea>`)

// GetReplayScript reads the replay form: GET /job/X/<n>/replay/ (HTML; there
// is no JSON API for it).
func (c *Client) GetReplayScript(ctx context.Context, job string, number int) (core.ReplayScript, error) {
	b, err := c.getText(ctx, runPath(job, number)+"replay/", nil, 16<<20)
	if err != nil {
		return core.ReplayScript{}, withHint(err, errs.CodeNotFound,
			"replay is not available for this build: it is not a Pipeline, the user lacks the Replay permission, "+
				"or the Jenkinsfile is not trusted (e.g. pull request builds from forks)")
	}
	rs := core.ReplayScript{Loaded: map[string]string{}}
	found := false
	for _, m := range replayTextareaRE.FindAllStringSubmatch(string(b), -1) {
		// Browsers drop one newline right after <textarea>; do the same.
		text := html.UnescapeString(strings.TrimPrefix(strings.TrimPrefix(m[2], "\r"), "\n"))
		if m[1] == "mainScript" {
			rs.Main, found = text, true
		} else {
			rs.Loaded[m[1]] = text
		}
	}
	if !found {
		return core.ReplayScript{}, errs.New(errs.CodeUpstream, "the replay page has no mainScript field").
			WithHint("check the profile url; a login or proxy page may have been returned")
	}
	return rs, nil
}

// StreamConsole: GET /job/X/<n>/consoleText. The body is returned unread and
// bound only to ctx (no per-request timeout), so large logs can be streamed.
func (c *Client) StreamConsole(ctx context.Context, job string, number int) (io.ReadCloser, error) {
	u, err := c.endpoint(runPath(job, number)+"consoleText", nil)
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(c.http, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, refNotFound(statusError(resp), core.BuildRef(strconv.Itoa(number)))
	}
	return resp.Body, nil
}

// GetTestReport returns counts and only FAILED/REGRESSION cases (untrimmed).
func (c *Client) GetTestReport(ctx context.Context, job string, number int) (core.TestReport, error) {
	var r struct {
		FailCount int `json:"failCount"`
		PassCount int `json:"passCount"`
		SkipCount int `json:"skipCount"`
		Suites    []struct {
			Cases []struct {
				ClassName    string  `json:"className"`
				Name         string  `json:"name"`
				Status       string  `json:"status"`
				ErrorDetails string  `json:"errorDetails"`
				Duration     float64 `json:"duration"` // seconds
			} `json:"cases"`
		} `json:"suites"`
	}
	err := c.getJSON(ctx, runPath(job, number)+"testReport/api/json", treeQuery(treeTests), &r)
	if errs.Is(err, errs.CodeNotFound) {
		return core.TestReport{HasReport: false, Failed: []core.TestCase{}}, nil
	}
	if err != nil {
		return core.TestReport{}, err
	}
	out := core.TestReport{
		HasReport: true,
		FailCount: r.FailCount,
		PassCount: r.PassCount,
		SkipCount: r.SkipCount,
		Failed:    []core.TestCase{},
	}
	for _, s := range r.Suites {
		for _, tc := range s.Cases {
			if tc.Status != "FAILED" && tc.Status != "REGRESSION" {
				continue
			}
			out.Failed = append(out.Failed, core.TestCase{
				ClassName:    tc.ClassName,
				Name:         tc.Name,
				Status:       tc.Status,
				ErrorDetails: tc.ErrorDetails,
				DurationMs:   int64(tc.Duration*1000 + 0.5),
			})
		}
	}
	return out, nil
}

// GetQueueItem: GET /queue/item/<id>/api/json
func (c *Client) GetQueueItem(ctx context.Context, id int64) (core.QueueItem, error) {
	var r struct {
		ID         int64  `json:"id"`
		Why        string `json:"why"`
		Blocked    bool   `json:"blocked"`
		Cancelled  bool   `json:"cancelled"`
		Executable *struct {
			Number int    `json:"number"`
			URL    string `json:"url"`
		} `json:"executable"`
		Task *struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"task"`
	}
	path := "/queue/item/" + strconv.FormatInt(id, 10) + "/api/json"
	if err := c.getJSON(ctx, path, treeQuery(treeQueue), &r); err != nil {
		return core.QueueItem{}, withHint(err, errs.CodeNotFound,
			"queue items are removed a few minutes after the build starts; use build_get with the job instead")
	}
	q := core.QueueItem{ID: r.ID, Why: r.Why, Blocked: r.Blocked, Cancelled: r.Cancelled}
	if r.Executable != nil {
		n := r.Executable.Number
		q.BuildNumber = &n
		q.BuildURL = r.Executable.URL
	}
	if r.Task != nil {
		q.TaskName = r.Task.Name
		q.TaskURL = r.Task.URL
	}
	return q, nil
}

// TriggerBuild POSTs /build, or /buildWithParameters with form values.
func (c *Client) TriggerBuild(ctx context.Context, job string, params map[string]string, parameterized bool) (int64, error) {
	path := core.JobURLPath(job) + "build"
	var form url.Values
	if parameterized {
		path = core.JobURLPath(job) + "buildWithParameters"
		form = url.Values{}
		for k, v := range params {
			form.Set(k, v)
		}
	}
	res, err := c.postForm(ctx, path, form, false)
	if err != nil {
		return 0, withHint(err, errs.CodeNotFound, jobNotFoundHint)
	}
	id, ok := parseQueueID(res.Location)
	if !ok {
		return 0, errs.Newf(errs.CodeUpstream,
			"Jenkins accepted the trigger (HTTP %d) but returned no queue item location", res.Status).
			WithHint("check the job page; the build may still have been queued")
	}
	return id, nil
}

// ReplayBuild POSTs /job/X/<n>/replay/run with mainScript (also as the
// structured "json" form field Jenkins' form binding reads). Loaded library
// scripts are not sent. Returns the queue id when Jenkins reports one, else 0.
func (c *Client) ReplayBuild(ctx context.Context, job string, number int, script string) (int64, error) {
	payload, err := json.Marshal(map[string]any{"mainScript": script})
	if err != nil {
		return 0, errs.Wrap(errs.CodeInternal, err, "encode replay payload")
	}
	form := url.Values{"mainScript": {script}, "json": {string(payload)}}
	res, err := c.postForm(ctx, runPath(job, number)+"replay/run", form, false)
	if err != nil {
		if e, ok := errs.As(err); ok && e.Code == errs.CodeUpstream && strings.Contains(e.Message, "HTTP 5") {
			// Typically a build whose replay form also has loaded scripts.
			e.Hint = "replay of builds that use loaded/library scripts is not supported (only the main script is sent); " +
				"otherwise " + maybeAppliedHint
		}
		return 0, withHint(err, errs.CodeNotFound,
			"replay needs a Pipeline build that still exists and the Replay permission")
	}
	id, _ := parseQueueID(res.Location)
	return id, nil
}

// StopBuild: POST /job/X/<n>/stop
func (c *Client) StopBuild(ctx context.Context, job string, number int) error {
	// Stopping is idempotent, so transport errors stay retryable.
	_, err := c.postForm(ctx, runPath(job, number)+"stop", nil, true)
	return refNotFound(err, core.BuildRef(strconv.Itoa(number)))
}

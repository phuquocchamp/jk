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
			ID              string `json:"id"`
			Name            string `json:"name"`
			Status          string `json:"status"`
			StartTimeMillis int64  `json:"startTimeMillis"`
			DurationMillis  int64  `json:"durationMillis"`
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
			StartTime: millis(s.StartTimeMillis), DurationMs: s.DurationMillis,
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
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"stageFlowNodes"`
	}
	if err := c.getJSON(ctx, nodePath(job, number, stageID)+"wfapi/describe", nil, &r); err != nil {
		return nil, err
	}
	out := make([]core.StageNode, 0, len(r.StageFlowNodes))
	for _, n := range r.StageFlowNodes {
		out = append(out, core.StageNode{ID: n.ID, Name: n.Name, Status: n.Status})
	}
	return out, nil
}

// htmlTagRE matches markup that Jenkins adds to annotated logs (console notes,
// hyperlinks). Literal '<' in the log itself arrives escaped as &lt;.
var htmlTagRE = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)

// plainLogText strips tags first and unescapes second, so escaped literal
// markup in the log survives.
func plainLogText(s string) string {
	return html.UnescapeString(htmlTagRE.ReplaceAllString(s, ""))
}

// GetNodeLog: GET /job/X/<n>/execution/node/<nodeID>/wfapi/log (Text HTML-unescaped).
func (c *Client) GetNodeLog(ctx context.Context, job string, number int, nodeID string) (core.NodeLog, error) {
	var r struct {
		NodeID     string `json:"nodeId"`
		NodeStatus string `json:"nodeStatus"`
		Length     int64  `json:"length"`
		HasMore    bool   `json:"hasMore"`
		Text       string `json:"text"`
		ConsoleURL string `json:"consoleUrl"`
	}
	if err := c.getJSON(ctx, nodePath(job, number, nodeID)+"wfapi/log", nil, &r); err != nil {
		return core.NodeLog{}, err
	}
	return core.NodeLog{
		NodeID:     r.NodeID,
		Status:     r.NodeStatus,
		Length:     r.Length,
		HasMore:    r.HasMore,
		Text:       plainLogText(r.Text),
		ConsoleURL: r.ConsoleURL,
	}, nil
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

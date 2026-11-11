package core

import "time"

// ---------------------------------------------------------------------------
// Domain types. JSON: snake_case; durations in milliseconds (*_ms);
// timestamps are time.Time (RFC3339 in JSON). The `jsonschema` tag is the
// plain property description used by the MCP SDK (must not start with "word=").
// Optional input fields carry `omitempty` so they are not "required" in the schema.
// ---------------------------------------------------------------------------

// User is the authenticated Jenkins user.
type User struct {
	ID       string `json:"id" jsonschema:"Jenkins user id"`
	FullName string `json:"full_name" jsonschema:"display name"`
}

// Job kinds derived from Jenkins _class.
const (
	KindJob         = "job"         // freestyle or other buildable job
	KindPipeline    = "pipeline"    // WorkflowJob (incl. multibranch branch jobs)
	KindFolder      = "folder"      // cloudbees Folder (container)
	KindMultibranch = "multibranch" // WorkflowMultiBranchProject (container)
	KindOther       = "other"
)

// JobSummary is one entry of a job listing.
type JobSummary struct {
	Name      string `json:"name" jsonschema:"job name as stored by Jenkins (branch names may contain %2F)"`
	Path      string `json:"path" jsonschema:"full job path with / separators; pass it to other tools"`
	Kind      string `json:"kind" jsonschema:"job, pipeline, folder, multibranch or other"`
	Container bool   `json:"container" jsonschema:"true for folders and multibranch projects (list them with job_list)"`
	Color     string `json:"color,omitempty" jsonschema:"Jenkins status color, e.g. blue, red, blue_anime"`
	URL       string `json:"url" jsonschema:"Jenkins URL of the job"`
}

// ParamDef is a job parameter definition.
type ParamDef struct {
	Name        string   `json:"name"`
	Type        string   `json:"type" jsonschema:"Jenkins parameter definition class short name"`
	Description string   `json:"description,omitempty"`
	Default     any      `json:"default,omitempty"`
	Choices     []string `json:"choices,omitempty"`
}

// Job is a job with its permalinks and parameter definitions.
type Job struct {
	Name                string     `json:"name"`
	Path                string     `json:"path"`
	URL                 string     `json:"url"`
	Kind                string     `json:"kind"`
	Class               string     `json:"class"`
	Color               string     `json:"color,omitempty"`
	Buildable           bool       `json:"buildable"`
	LastBuild           *int       `json:"last_build,omitempty"`
	LastSuccessfulBuild *int       `json:"last_successful_build,omitempty"`
	LastFailedBuild     *int       `json:"last_failed_build,omitempty"`
	LastCompletedBuild  *int       `json:"last_completed_build,omitempty"`
	Parameters          []ParamDef `json:"parameters,omitempty" jsonschema:"parameter definitions; non-empty means build_trigger uses buildWithParameters"`
}

// Param is a parameter value used by a build.
type Param struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// Change is one SCM change in a build.
type Change struct {
	CommitID string `json:"commit_id"`
	Message  string `json:"message"`
	Author   string `json:"author,omitempty"`
}

// Build is a single build.
type Build struct {
	Job                 string    `json:"job"`
	Number              int       `json:"number"`
	Result              string    `json:"result,omitempty" jsonschema:"SUCCESS, FAILURE, UNSTABLE, ABORTED, NOT_BUILT; empty while building"`
	Building            bool      `json:"building"`
	DurationMs          int64     `json:"duration_ms"`
	EstimatedDurationMs int64     `json:"estimated_duration_ms,omitempty"`
	Timestamp           time.Time `json:"timestamp" jsonschema:"build start time (RFC3339)"`
	URL                 string    `json:"url"`
	Causes              []string  `json:"causes,omitempty"`
	Parameters          []Param   `json:"parameters,omitempty"`
	Revision            string    `json:"revision,omitempty" jsonschema:"last built SCM revision SHA1"`
	Changes             []Change  `json:"changes,omitempty"`
}

// Stage is a pipeline stage from wfapi.
type Stage struct {
	ID         string    `json:"id" jsonschema:"stage flow node id; usable with build_log stage"`
	Name       string    `json:"name"`
	Status     string    `json:"status" jsonschema:"SUCCESS, FAILED, UNSTABLE, ABORTED, IN_PROGRESS, NOT_EXECUTED, PAUSED_PENDING_INPUT"`
	StartTime  time.Time `json:"start_time"`
	DurationMs int64     `json:"duration_ms"`
}

// Run is a pipeline run (wfapi/describe).
type Run struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	DurationMs int64   `json:"duration_ms"`
	Stages     []Stage `json:"stages"`
}

// StageNode is a flow node inside a stage (execution/node/<id>/wfapi/describe).
type StageNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// NodeLog is the log of one flow node (wfapi/log). Text is already HTML-unescaped.
type NodeLog struct {
	NodeID     string `json:"node_id"`
	Status     string `json:"status"`
	Length     int64  `json:"length"`
	HasMore    bool   `json:"has_more"`
	Text       string `json:"text"`
	ConsoleURL string `json:"console_url,omitempty"`
}

// TestCase is a failed test case.
type TestCase struct {
	ClassName    string `json:"class_name"`
	Name         string `json:"name"`
	Status       string `json:"status" jsonschema:"FAILED or REGRESSION"`
	ErrorDetails string `json:"error_details,omitempty" jsonschema:"error message, trimmed to about 500 chars"`
	DurationMs   int64  `json:"duration_ms"`
}

// TestReport summarizes a build's junit report. Failed holds only FAILED/REGRESSION cases.
type TestReport struct {
	HasReport bool       `json:"has_report" jsonschema:"false when the build has no test report"`
	FailCount int        `json:"fail_count"`
	PassCount int        `json:"pass_count"`
	SkipCount int        `json:"skip_count"`
	Failed    []TestCase `json:"failed"`
}

// QueueItem is a Jenkins queue item.
type QueueItem struct {
	ID          int64  `json:"id"`
	Why         string `json:"why,omitempty" jsonschema:"reason the item is still waiting"`
	Blocked     bool   `json:"blocked"`
	Cancelled   bool   `json:"cancelled"`
	BuildNumber *int   `json:"build_number,omitempty" jsonschema:"set once the item has started a build"`
	BuildURL    string `json:"build_url,omitempty"`
	TaskName    string `json:"task_name,omitempty"`
	TaskURL     string `json:"task_url,omitempty"`
}

// Plan describes the HTTP call a write/danger op would make (dry-run, or
// details of NEEDS_CONFIRMATION). It never contains credentials.
type Plan struct {
	Op     string            `json:"op"`
	Method string            `json:"method"`
	Path   string            `json:"path" jsonschema:"request path relative to the Jenkins base URL"`
	Params map[string]string `json:"params,omitempty" jsonschema:"form parameters that would be sent"`
	Note   string            `json:"note,omitempty"`
	// Number is the resolved build number for build-scoped ops, so a confirmed
	// re-run can pin it instead of re-resolving a permalink such as "last".
	Number int `json:"number,omitempty" jsonschema:"resolved build number the plan acts on"`
}

// ---------------------------------------------------------------------------
// Op inputs / outputs (one pair per catalog entry).
// ---------------------------------------------------------------------------

type WhoamiIn struct{}

type WhoamiOut struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
	URL      string `json:"url" jsonschema:"Jenkins base URL of the active profile"`
}

type JobListIn struct {
	Folder string `json:"folder,omitempty" jsonschema:"folder or multibranch project path; empty lists the root"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max entries to return (default 50)"`
}

type JobListOut struct {
	Folder    string       `json:"folder"`
	Jobs      []JobSummary `json:"jobs"`
	Total     int          `json:"total" jsonschema:"entries visible before limit (after policy filtering)"`
	Truncated bool         `json:"truncated"`
}

type JobGetIn struct {
	Job string `json:"job" jsonschema:"job path like folder/sub/job (multibranch: project/branch, encode / in branch names as %2F) or a full Jenkins job URL"`
}

type JobGetOut = Job

type BuildGetIn struct {
	Job string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref BuildRef `json:"ref,omitempty" jsonschema:"build number (42 or \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
}

type BuildGetOut = Build

type BuildStagesIn struct {
	Job string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref BuildRef `json:"ref,omitempty" jsonschema:"build number (42 or \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
}

type BuildStagesOut struct {
	Job        string  `json:"job"`
	Number     int     `json:"number"`
	Status     string  `json:"status"`
	DurationMs int64   `json:"duration_ms"`
	Stages     []Stage `json:"stages"`
}

type BuildLogIn struct {
	Job   string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref   BuildRef `json:"ref,omitempty" jsonschema:"build number (42 or \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
	Lines int      `json:"lines,omitempty" jsonschema:"number of trailing lines (default and cap: policy max_log_lines)"`
	Stage string   `json:"stage,omitempty" jsonschema:"stage name or id; when set only that stage's log is returned"`
}

type BuildLogOut struct {
	Job        string `json:"job"`
	Number     int    `json:"number"`
	Stage      string `json:"stage,omitempty"`
	Text       string `json:"text" jsonschema:"the last lines of the log, newline separated"`
	Lines      int    `json:"lines" jsonschema:"number of lines in text"`
	TotalLines int    `json:"total_lines" jsonschema:"total lines in the (stage) log"`
	Truncated  bool   `json:"truncated" jsonschema:"true when earlier lines were omitted or the request was capped"`
	SpilledTo  string `json:"spilled_to,omitempty" jsonschema:"set when text was too large to inline: local file holding the full text (text is then empty)"`
}

type LogSearchIn struct {
	Job        string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref        BuildRef `json:"ref,omitempty" jsonschema:"build number (42 or \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
	Pattern    string   `json:"pattern" jsonschema:"Go RE2 regular expression matched per line"`
	Context    int      `json:"context,omitempty" jsonschema:"lines of context before and after each match (default 0)"`
	MaxMatches int      `json:"max_matches,omitempty" jsonschema:"stop after this many matches (default 50)"`
}

// LogMatch is one search hit; Line is 1-based.
type LogMatch struct {
	Line   int      `json:"line"`
	Text   string   `json:"text"`
	Before []string `json:"before,omitempty"`
	After  []string `json:"after,omitempty"`
}

type LogSearchOut struct {
	Job          string     `json:"job"`
	Number       int        `json:"number"`
	Pattern      string     `json:"pattern"`
	Matches      []LogMatch `json:"matches"`
	ScannedLines int        `json:"scanned_lines" jsonschema:"lines read before the scan ended; equals the log length only when truncated is false"`
	Truncated    bool       `json:"truncated" jsonschema:"true when max_matches (or the line cap) stopped the scan early"`
	SpilledTo    string     `json:"spilled_to,omitempty" jsonschema:"set when the matches were too large to inline: local file holding them as grep-style text (matches then carry no context)"`
}

type TestFailedIn struct {
	Job   string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref   BuildRef `json:"ref,omitempty" jsonschema:"build number (42 or \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
	Limit int      `json:"limit,omitempty" jsonschema:"max failed cases to return (default 50)"`
}

type TestFailedOut struct {
	Job       string     `json:"job"`
	Number    int        `json:"number"`
	HasReport bool       `json:"has_report"`
	FailCount int        `json:"fail_count"`
	PassCount int        `json:"pass_count"`
	SkipCount int        `json:"skip_count"`
	Failed    []TestCase `json:"failed"`
	Truncated bool       `json:"truncated" jsonschema:"true when more failed cases exist than limit"`
}

type QueueGetIn struct {
	ID int64 `json:"id" jsonschema:"queue item id (returned by build_trigger)"`
}

type QueueGetOut = QueueItem

type BuildTriggerIn struct {
	Job            string            `json:"job" jsonschema:"job path or full Jenkins job URL"`
	Params         map[string]string `json:"params,omitempty" jsonschema:"build parameters as name to value"`
	Wait           bool              `json:"wait,omitempty" jsonschema:"wait until the build starts and finishes (bounded by timeout_seconds)"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty" jsonschema:"wait timeout in seconds (default 600)"`
	DryRun         bool              `json:"dry_run,omitempty" jsonschema:"return the plan without triggering"`
}

type BuildTriggerOut struct {
	Job      string `json:"job"`
	DryRun   bool   `json:"dry_run"`
	Plan     *Plan  `json:"plan,omitempty"`
	QueueID  int64  `json:"queue_id,omitempty"`
	Number   *int   `json:"number,omitempty" jsonschema:"build number once known"`
	Result   string `json:"result,omitempty" jsonschema:"final result when wait completed"`
	Building bool   `json:"building"`
	URL      string `json:"url,omitempty"`
	TimedOut bool   `json:"timed_out" jsonschema:"true when wait hit timeout_seconds"`
}

type BuildReplayIn struct {
	Job    string   `json:"job" jsonschema:"job path or full Jenkins build URL"`
	Ref    BuildRef `json:"ref" jsonschema:"build to replay: number (42 or \"42\") or last, lastFailed, lastSuccessful, lastCompleted"`
	Script string   `json:"script" jsonschema:"full Jenkinsfile (main script) to run instead of the original"`
	DryRun bool     `json:"dry_run,omitempty" jsonschema:"return the plan without replaying"`
}

type BuildReplayOut struct {
	Job        string `json:"job"`
	FromNumber int    `json:"from_number"`
	DryRun     bool   `json:"dry_run"`
	Plan       *Plan  `json:"plan,omitempty"`
	QueueID    int64  `json:"queue_id,omitempty" jsonschema:"queue id when Jenkins reported one"`
}

type BuildStopIn struct {
	Job     string   `json:"job" jsonschema:"job path or full Jenkins build URL"`
	Ref     BuildRef `json:"ref" jsonschema:"build to stop: number (42 or \"42\") or last"`
	Confirm bool     `json:"confirm,omitempty" jsonschema:"must be true to actually stop; otherwise NEEDS_CONFIRMATION with the plan"`
	DryRun  bool     `json:"dry_run,omitempty" jsonschema:"return the plan without stopping"`
}

type BuildStopOut struct {
	Job     string `json:"job"`
	Number  int    `json:"number"`
	DryRun  bool   `json:"dry_run"`
	Plan    *Plan  `json:"plan,omitempty"`
	Stopped bool   `json:"stopped"`
}

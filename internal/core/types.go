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

// StageError is the error Jenkins recorded on a stage or step.
type StageError struct {
	Type    string `json:"type,omitempty" jsonschema:"Java exception class, e.g. hudson.AbortException"`
	Message string `json:"message"`
}

// FailedStep is the step that failed inside a stage.
type FailedStep struct {
	ID    string      `json:"id" jsonschema:"flow node id of the step"`
	Name  string      `json:"name" jsonschema:"step name; for sh/bat steps this is the command"`
	Error *StageError `json:"error,omitempty"`
}

// Stage is a pipeline stage (from Pipeline Graph View, or wfapi as fallback).
type Stage struct {
	ID         string      `json:"id" jsonschema:"stage flow node id; usable with build_log stage"`
	Name       string      `json:"name"`
	Type       string      `json:"type,omitempty" jsonschema:"stage or parallel (a branch of a parallel block); empty with source=wfapi"`
	ParentID   string      `json:"parent_id,omitempty" jsonschema:"id of the enclosing stage or parallel branch"`
	Status     string      `json:"status" jsonschema:"SUCCESS, FAILED, UNSTABLE, ABORTED, SKIPPED, IN_PROGRESS, NOT_EXECUTED, PAUSED_PENDING_INPUT"`
	StartTime  time.Time   `json:"start_time"`
	DurationMs int64       `json:"duration_ms"`
	Error      *StageError `json:"error,omitempty"`
	FailedStep *FailedStep `json:"failed_step,omitempty" jsonschema:"the step that failed in this stage, when known"`
	Inherited  bool        `json:"inherited,omitempty" jsonschema:"FAILED only because a nested stage failed (a parent or parallel branch); not a separate failure"`
	// SkippedAfterFailure is inferred with source=wfapi, which reports stages
	// skipped after an earlier failure as FAILED.
	SkippedAfterFailure bool `json:"skipped_after_failure,omitempty" jsonschema:"inferred (source=wfapi): Jenkins reported FAILED but no step of the stage failed, so it was skipped after an earlier failure; status is SKIPPED"`
}

// GraphStage is a stage of the Pipeline Graph View tree ({run}/stages/tree).
type GraphStage struct {
	ID         string
	Name       string
	Type       string // STAGE, PARALLEL, ...
	State      string // success, failure, skipped, running, ...
	StartTime  time.Time
	DurationMs int64
	Children   []GraphStage
}

// GraphStep is one step from Pipeline Graph View ({run}/stages/allSteps).
type GraphStep struct {
	ID         string
	Name       string // for sh steps: the command
	StageID    string
	State      string
	DurationMs int64
}

// ReplayScript is the replay form of a build: the main script and any loaded scripts.
type ReplayScript struct {
	Main   string
	Loaded map[string]string // loaded script name -> text
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
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Status  string      `json:"status"`
	Command string      `json:"command,omitempty" jsonschema:"parameterDescription, e.g. the sh command"`
	Error   *StageError `json:"error,omitempty"`
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
	Number    int        `json:"number,omitempty" jsonschema:"resolved build number the plan acts on"`
	Diff      string     `json:"diff,omitempty" jsonschema:"unified diff of the replayed script against the build's original script"`
	DiffStats *DiffStats `json:"diff_stats,omitempty"`
}

// DiffStats counts changed lines of a Plan diff.
type DiffStats struct {
	Added     int  `json:"added"`
	Removed   int  `json:"removed"`
	Truncated bool `json:"truncated,omitempty" jsonschema:"true when the diff text was cut"`
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
	Job         string  `json:"job"`
	Number      int     `json:"number"`
	Status      string  `json:"status"`
	DurationMs  int64   `json:"duration_ms"`
	Source      string  `json:"source" jsonschema:"pipeline-graph-view (accurate: skipped stages are SKIPPED, parallel branches have parent_id) or wfapi (fallback: stages skipped after a failure may also show FAILED)"`
	FirstFailed string  `json:"first_failed,omitempty" jsonschema:"id of the stage where the build first failed; pass it as build_log stage"`
	Stages      []Stage `json:"stages" jsonschema:"stages in execution order, nested stages after their parent"`
}

type BuildLogIn struct {
	Job   string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref   BuildRef `json:"ref,omitempty" jsonschema:"build number (42 or \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
	Lines int      `json:"lines,omitempty" jsonschema:"number of trailing lines (default and cap: policy max_log_lines)"`
	Stage string   `json:"stage,omitempty" jsonschema:"stage name or id; when set only that stage's log is returned (all its steps, full text)"`
	Step  string   `json:"step,omitempty" jsonschema:"\"failed\" for only the failing step's log (of stage, or of the first failed stage when stage is empty), or a step node id"`
	Raw   bool     `json:"raw,omitempty" jsonschema:"keep ANSI color codes and timestamper prefixes (default strips them)"`
}

type BuildLogOut struct {
	Job        string   `json:"job"`
	Number     int      `json:"number"`
	Stage      string   `json:"stage,omitempty"`
	Text       string   `json:"text" jsonschema:"the last lines of the log, newline separated"`
	Lines      int      `json:"lines" jsonschema:"number of lines in text"`
	TotalLines int      `json:"total_lines,omitempty" jsonschema:"total lines in the (stage) log; omitted when only the end of the console was read"`
	TotalBytes int64    `json:"total_bytes,omitempty" jsonschema:"size of the raw console log in bytes, when known"`
	ReadBytes  int64    `json:"read_bytes,omitempty" jsonschema:"bytes downloaded to produce the tail, when the console was read from an offset"`
	Step       string   `json:"step,omitempty" jsonschema:"name of the step read (for sh: the command)"`
	StepID     string   `json:"step_id,omitempty"`
	StepsRead  int      `json:"steps_read,omitempty" jsonschema:"step logs downloaded; the last ones are read first and reading stops once there are enough lines"`
	StepsTotal int      `json:"steps_total,omitempty" jsonschema:"steps in the stage (total_lines is given only when all were read)"`
	Source     string   `json:"source" jsonschema:"consoleText, progressiveText (console read from an offset), steps (stage log from its steps) or step (one step)"`
	Stripped   []string `json:"stripped,omitempty" jsonschema:"what was removed from each line: ansi, timestamps"`
	Truncated  bool     `json:"truncated" jsonschema:"true when earlier lines were omitted or the request was capped"`
	SpilledTo  string   `json:"spilled_to,omitempty" jsonschema:"set when text was too large to inline: local file holding the full text (text is then empty)"`
	SpilledURI string   `json:"spilled_uri,omitempty" jsonschema:"MCP resource URI of the spilled file (read it with resources/read when the local path is not readable)"`
}

type LogSearchIn struct {
	Job        string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref        BuildRef `json:"ref,omitempty" jsonschema:"build number (42 or \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
	Pattern    string   `json:"pattern" jsonschema:"Go RE2 regular expression matched per line"`
	Context    int      `json:"context,omitempty" jsonschema:"lines of context before and after each match (default 0)"`
	MaxMatches int      `json:"max_matches,omitempty" jsonschema:"stop after this many matches (default 50)"`
	Raw        bool     `json:"raw,omitempty" jsonschema:"match and return lines with ANSI codes and timestamper prefixes (default strips them)"`
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
	SpilledURI   string     `json:"spilled_uri,omitempty" jsonschema:"MCP resource URI of the spilled file"`
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
	TimeoutSeconds int               `json:"timeout_seconds,omitempty" jsonschema:"wait timeout in seconds (default 600; MCP caps it, then use build_wait)"`
	AllowUnknown   bool              `json:"allow_unknown,omitempty" jsonschema:"send parameters the job does not define (Jenkins ignores them) instead of failing"`
	DryRun         bool              `json:"dry_run,omitempty" jsonschema:"return the plan without triggering"`
}

// Next suggests the follow-up call after a wait that timed out.
type Next struct {
	Tool string         `json:"tool" jsonschema:"MCP tool name"`
	CLI  string         `json:"cli" jsonschema:"equivalent jk command"`
	Args map[string]any `json:"args"`
}

type BuildTriggerOut struct {
	Job      string   `json:"job"`
	DryRun   bool     `json:"dry_run"`
	Plan     *Plan    `json:"plan,omitempty"`
	QueueID  int64    `json:"queue_id,omitempty"`
	Number   *int     `json:"number,omitempty" jsonschema:"build number once known"`
	Result   string   `json:"result,omitempty" jsonschema:"final result when wait completed"`
	Building bool     `json:"building"`
	URL      string   `json:"url,omitempty"`
	TimedOut bool     `json:"timed_out" jsonschema:"true when wait hit timeout_seconds"`
	Next     *Next    `json:"next,omitempty" jsonschema:"how to keep waiting when timed_out is true"`
	Warnings []string `json:"warnings,omitempty"`
}

type BuildWaitIn struct {
	Job            string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref            BuildRef `json:"ref,omitempty" jsonschema:"build number to wait for (or a permalink); omit when queue_id is given"`
	QueueID        int64    `json:"queue_id,omitempty" jsonschema:"queue item id from build_trigger, when the build number is not known yet"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty" jsonschema:"wait timeout in seconds (default 600; MCP caps it)"`
}

type BuildWaitOut struct {
	Job      string `json:"job"`
	QueueID  int64  `json:"queue_id,omitempty"`
	Number   *int   `json:"number,omitempty"`
	Result   string `json:"result,omitempty" jsonschema:"final result once the build finished"`
	Building bool   `json:"building"`
	URL      string `json:"url,omitempty"`
	TimedOut bool   `json:"timed_out"`
	Next     *Next  `json:"next,omitempty" jsonschema:"call again with these arguments when timed_out is true"`
}

type BuildReplayIn struct {
	Job     string   `json:"job" jsonschema:"job path or full Jenkins build URL"`
	Ref     BuildRef `json:"ref" jsonschema:"build to replay: number (42 or \"42\") or last, lastFailed, lastSuccessful, lastCompleted"`
	Script  string   `json:"script" jsonschema:"full Jenkinsfile (main script) to run instead of the original; read the original with build_replay_script"`
	Confirm bool     `json:"confirm,omitempty" jsonschema:"must be true to actually replay; otherwise NEEDS_CONFIRMATION with the plan and a diff against the original script"`
	DryRun  bool     `json:"dry_run,omitempty" jsonschema:"return the plan (with diff) without replaying"`
}

type BuildReplayScriptIn struct {
	Job string   `json:"job" jsonschema:"job path or full Jenkins build URL"`
	Ref BuildRef `json:"ref,omitempty" jsonschema:"build number (42 or \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
}

// LoadedScript is a script the build loaded besides the main Jenkinsfile.
type LoadedScript struct {
	Name  string `json:"name"`
	Bytes int    `json:"bytes"`
}

type BuildReplayScriptOut struct {
	Job           string         `json:"job"`
	Number        int            `json:"number"`
	MainScript    string         `json:"main_script" jsonschema:"the Jenkinsfile the build ran (empty when spilled)"`
	Bytes         int            `json:"bytes"`
	Lines         int            `json:"lines"`
	LoadedScripts []LoadedScript `json:"loaded_scripts,omitempty" jsonschema:"other scripts the build loaded; build_replay keeps them unchanged"`
	SpilledTo     string         `json:"spilled_to,omitempty" jsonschema:"set when the script was too large to inline: local file holding it"`
	SpilledURI    string         `json:"spilled_uri,omitempty" jsonschema:"MCP resource URI of the spilled file"`
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

type BuildDiagnoseIn struct {
	Job      string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref      BuildRef `json:"ref,omitempty" jsonschema:"build number (42 or \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default lastFailed)"`
	LogLines int      `json:"log_lines,omitempty" jsonschema:"lines of the failing step's log to include (default 60, capped by policy)"`
	MaxTests int      `json:"max_tests,omitempty" jsonschema:"max failed test cases to include (default 10)"`
}

// Failure locates where a build failed and carries the end of that log.
type Failure struct {
	StageID      string      `json:"stage_id,omitempty"`
	Stage        string      `json:"stage,omitempty" jsonschema:"name of the stage that failed first"`
	Step         *FailedStep `json:"step,omitempty" jsonschema:"the failing step (for sh: the command) and its error"`
	Log          string      `json:"log" jsonschema:"last lines of the failing step's log (or of the stage/console when the step is unknown), blank edges trimmed"`
	Highlights   []LogMatch  `json:"highlights,omitempty" jsonschema:"error-looking lines of the failing step's whole log (last 10; line numbers within the step)"`
	LogLines     int         `json:"log_lines"`
	LogSource    string      `json:"log_source" jsonschema:"step, stage or console"`
	LogTruncated bool        `json:"log_truncated"`
}

// TestSummary is the failed-test part of a diagnosis.
type TestSummary struct {
	HasReport bool       `json:"has_report"`
	FailCount int        `json:"fail_count"`
	PassCount int        `json:"pass_count"`
	SkipCount int        `json:"skip_count"`
	Failed    []TestCase `json:"failed"`
	Truncated bool       `json:"truncated"`
}

type BuildDiagnoseOut struct {
	Job          string       `json:"job"`
	Number       int          `json:"number"`
	Result       string       `json:"result,omitempty"`
	Building     bool         `json:"building"`
	URL          string       `json:"url"`
	DurationMs   int64        `json:"duration_ms"`
	Causes       []string     `json:"causes,omitempty"`
	Revision     string       `json:"revision,omitempty"`
	Changes      []Change     `json:"changes,omitempty" jsonschema:"SCM changes in this build (at most 10, first message line)"`
	ChangesTotal int          `json:"changes_total"`
	StageSource  string       `json:"stage_source,omitempty" jsonschema:"pipeline-graph-view or wfapi; empty for non-pipeline jobs"`
	Failure      *Failure     `json:"failure,omitempty" jsonschema:"where the build failed; absent for successful builds"`
	FailedStages []string     `json:"failed_stages,omitempty" jsonschema:"all stages with a failing step (the first one is in failure)"`
	Tests        *TestSummary `json:"tests,omitempty"`
	Next         []string     `json:"next,omitempty" jsonschema:"suggested follow-up calls"`
	Warnings     []string     `json:"warnings,omitempty" jsonschema:"parts that could not be read; the rest is still valid"`
}

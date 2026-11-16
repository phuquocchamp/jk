package core

import (
	"context"
	"io"
)

// Backend is the Jenkins transport, implemented by internal/client.
//
// Conventions for implementers:
//   - job is a normalized job path (see NormalizeJobPath); build URLs use JobURLPath.
//   - Errors are *errs.Error mapped from HTTP status (errs.FromHTTPStatus);
//     transport timeouts are UPSTREAM retryable.
//   - Backend does no policy checks, ref defaulting, limiting or trimming;
//     Service does that.
type Backend interface {
	// BaseURL returns the configured Jenkins base URL (no trailing slash).
	BaseURL() string

	// Whoami: GET /me/api/json?tree=id,fullName
	Whoami(ctx context.Context) (User, error)

	// ListJobs lists direct children of folder ("" = root). Path is filled as folder + "/" + name.
	ListJobs(ctx context.Context, folder string) ([]JobSummary, error)

	// GetJob returns job details incl. permalinks and parameter definitions.
	GetJob(ctx context.Context, job string) (Job, error)

	// ResolveBuild turns a ref (number or permalink) into a build number.
	// NOT_FOUND when the permalink does not exist (e.g. no failed build).
	ResolveBuild(ctx context.Context, job string, ref BuildRef) (int, error)

	// GetBuild returns build details; ref may be a number or permalink.
	// Empty action entries are skipped.
	GetBuild(ctx context.Context, job string, ref BuildRef) (Build, error)

	// GetRun: GET /job/X/<n>/wfapi/describe
	GetRun(ctx context.Context, job string, number int) (Run, error)

	// GetStageNodes: GET /job/X/<n>/execution/node/<stageID>/wfapi/describe -> stageFlowNodes.
	GetStageNodes(ctx context.Context, job string, number int, stageID string) ([]StageNode, error)

	// GetStageTree: GET /job/X/<n>/stages/tree (Pipeline Graph View plugin).
	// NOT_FOUND when the plugin is not installed or the build is not a Pipeline.
	GetStageTree(ctx context.Context, job string, number int) ([]GraphStage, error)

	// GetSteps: GET /job/X/<n>/stages/allSteps (Pipeline Graph View plugin).
	GetSteps(ctx context.Context, job string, number int) ([]GraphStep, error)

	// GetStepError: GET /job/X/<n>/stages/exceptionText?nodeId=<id> ("" when none).
	GetStepError(ctx context.Context, job string, number int, nodeID string) (string, error)

	// StreamNodeLog: GET /job/X/<n>/execution/node/<id>/log/logText/progressiveText?start=0.
	// The full plain-text log of one flow node (wfapi/log is capped at ~10KB). Caller must Close.
	StreamNodeLog(ctx context.Context, job string, number int, nodeID string) (io.ReadCloser, error)

	// StreamConsole: GET /job/X/<n>/consoleText. Caller must Close. The body
	// is streamed; the request is bound to ctx (no fixed whole-request timeout).
	StreamConsole(ctx context.Context, job string, number int) (io.ReadCloser, error)

	// ConsoleSize: HEAD /job/X/<n>/logText/progressiveText -> X-Text-Size (raw bytes).
	ConsoleSize(ctx context.Context, job string, number int) (int64, error)

	// StreamConsoleFrom: GET /job/X/<n>/logText/progressiveText?start=<byte>.
	// Raw text: it may contain console notes (ESC[8mha:...ESC[0m). Caller must Close.
	StreamConsoleFrom(ctx context.Context, job string, number int, start int64) (io.ReadCloser, error)

	// GetReplayScript reads the replay form (GET /job/X/<n>/replay/): the main
	// script and loaded scripts. NOT_FOUND when replay is not available.
	GetReplayScript(ctx context.Context, job string, number int) (ReplayScript, error)

	// GetTestReport returns counts and only FAILED/REGRESSION cases (untrimmed).
	// HTTP 404 => TestReport{HasReport: false}, nil error.
	GetTestReport(ctx context.Context, job string, number int) (TestReport, error)

	// GetQueueItem: GET /queue/item/<id>/api/json
	GetQueueItem(ctx context.Context, id int64) (QueueItem, error)

	// TriggerBuild POSTs /build, or /buildWithParameters (form values) when
	// parameterized is true. Returns the queue id parsed from Location.
	TriggerBuild(ctx context.Context, job string, params map[string]string, parameterized bool) (int64, error)

	// ReplayBuild POSTs /job/X/<n>/replay/run with mainScript. Returns the queue
	// id when Jenkins reports one, else 0.
	ReplayBuild(ctx context.Context, job string, number int, script string) (int64, error)

	// StopBuild: POST /job/X/<n>/stop
	StopBuild(ctx context.Context, job string, number int) error
}

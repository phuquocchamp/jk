package core

import "github.com/phuquocchamp/jk/internal/kit/policy"

// Target kinds used for policy allow/deny checks.
const (
	TargetNone = ""
	TargetJob  = "job" // matched against policy rules "jobs"
)

// Op names (MCP tool name = MCPPrefix + Name).
const (
	OpWhoami       = "whoami"
	OpJobList      = "job_list"
	OpJobGet       = "job_get"
	OpBuildGet     = "build_get"
	OpBuildStages  = "build_stages"
	OpBuildLog     = "build_log"
	OpLogSearch    = "log_search"
	OpTestFailed   = "test_failed"
	OpQueueGet     = "queue_get"
	OpBuildTrigger = "build_trigger"
	OpBuildReplay  = "build_replay"
	OpBuildStop    = "build_stop"
)

// MCPPrefix is prepended to Op.Name for MCP tool names.
const MCPPrefix = "jenkins_"

// Op describes one operation for both CLI and MCP.
type Op struct {
	Name       string
	CLIPath    string // usage line under the root command
	Tier       policy.Tier
	TargetKind string
	Summary    string // one line, agent-facing: when to use + what it returns
}

// MCPName returns the MCP tool name.
func (o Op) MCPName() string { return MCPPrefix + o.Name }

// Catalog lists all operations in display order.
var Catalog = []Op{
	{OpWhoami, "auth status", policy.TierRead, TargetNone,
		"Check which Jenkins user the token authenticates as; returns id, full name and base URL."},
	{OpJobList, "job list [folder]", policy.TierRead, TargetJob,
		"Discover jobs: lists direct children of a folder or multibranch project (root if empty); returns name, path, kind, color."},
	{OpJobGet, "job get <job>", policy.TierRead, TargetJob,
		"Inspect a job before building: returns kind, last build numbers and parameter definitions."},
	{OpBuildGet, "build get <job> [ref]", policy.TierRead, TargetJob,
		"Check a build's outcome: returns result, building flag, duration, causes, parameters, revision and changes."},
	{OpBuildStages, "build stages <job> [ref]", policy.TierRead, TargetJob,
		"Find which pipeline stage failed: returns stages with status and duration."},
	{OpBuildLog, "log tail <job> [ref] --lines N [--stage NAME]", policy.TierRead, TargetJob,
		"Read the end of a build log (whole console or one stage); returns the last N lines, capped by policy."},
	{OpLogSearch, "log search <job> [ref] --pattern RE --context N --max-matches N", policy.TierRead, TargetJob,
		"Grep a build log by regex without downloading it; returns matching lines with line numbers and context."},
	{OpTestFailed, "test failed <job> [ref] --limit N", policy.TierRead, TargetJob,
		"List failed JUnit test cases of a build with trimmed error details; has_report=false when there is no report."},
	{OpQueueGet, "queue get <id>", policy.TierRead, TargetNone,
		"Follow a triggered build: returns why a queue item waits, or its build number once started."},
	{OpBuildTrigger, "build trigger <job> [-p k=v ...] [--wait --timeout]", policy.TierWrite, TargetJob,
		"Start a build (with parameters if the job has them); returns queue id, and build number/result when waiting. Supports dry_run."},
	{OpBuildReplay, "build replay <job> <ref> --script-file F", policy.TierWrite, TargetJob,
		"Re-run a pipeline build with a modified Jenkinsfile (main script only); returns queue id if known. Supports dry_run."},
	{OpBuildStop, "build stop <job> <ref>", policy.TierDanger, TargetJob,
		"Abort a running build; requires confirm=true, otherwise returns NEEDS_CONFIRMATION with the plan."},
}

// OpByName returns the catalog entry for name (without MCP prefix).
func OpByName(name string) (Op, bool) {
	for _, o := range Catalog {
		if o.Name == name {
			return o, true
		}
	}
	return Op{}, false
}

# jk design and contracts

jk is one Go binary that works as a CLI (`jk ...`) and as an MCP
server over stdio (`jk mcp`). This document is the contract between the
packages. The section "Package API" at the end is generated with `go doc -all`
from the code. When the prose and the code disagree, the code wins.

## Toolchain and dependencies

- Module: `github.com/phuquocchamp/jk`. `go.mod` says `go 1.26.0`. That is
  the lowest version the pinned dependencies allow: `golang.org/x/sys` v0.49 and
  `golang.org/x/term` v0.47 need 1.26, and go-sdk v1.8.0 needs 1.25. It still
  meets the "Go 1.24+" requirement. Release builds use `CGO_ENABLED=0`.
- Direct dependencies: cobra v1.10.2, modelcontextprotocol/go-sdk v1.8.0,
  zalando/go-keyring v0.2.8, itchyny/gojq v0.12.20, bmatcuk/doublestar/v4,
  gopkg.in/yaml.v3, golang.org/x/term. Add no others without a strong reason.
- Logging: `log/slog` to STDERR only. Stdout carries CLI data and the MCP protocol.
- Lint and test: `gofmt -l .`, `go vet ./...`, `go test ./...`. golangci-lint
  and goreleaser are not installed.
- Cross-build check: `CGO_ENABLED=0 GOOS={darwin,linux,windows} go build ./...`.

## Layout and ownership

```
cmd/jk/main.go     wires config -> auth -> client -> core -> cli|mcp (lazy, see cli.Deps.Open)
internal/core/           domain types, Backend, Catalog, Service, job path and BuildRef helpers.
                         Imports no cobra or go-sdk.
internal/client/         Jenkins REST adapter (net/http), implements core.Backend
internal/cli/            cobra commands; each catalog command calls one core.Service method
internal/mcpserver/      registers MCP tools from core.Catalog, filtered by tier
internal/kit/            reusable infrastructure, to be moved later to a shared "qtoolkit" module.
                         It must NOT import core, client, cli or mcpserver, and must contain nothing Jenkins-specific.
  errs   policy   auth   config   output   clix   mcpx
skills/jk/SKILL.md, skills/jk/references/recipes.md
docs/design.md, README.md, .goreleaser.yaml, Makefile, .gitignore, LICENSE (MIT, phuquocchamp)
```

Import direction (checked by
`go list -deps ./internal/kit/... | grep -E 'jk/internal/(core|client|cli|mcpserver)'`,
which must print nothing):

```
main -> cli, mcpserver, client, core, kit/*
cli -> core, kit/*        mcpserver -> core, kit/mcpx, kit/policy
client -> core, kit/errs  core -> kit/errs, kit/policy
kit/clix -> kit/errs, kit/output   kit/{policy,auth,config,output,mcpx} -> kit/errs (config -> policy)
```

Shared pure logic has a single owner in `core`. `core/jobpath.go` is already
implemented and tested: `ParseJobInput`, `NormalizeJobPath`, `JobURLPath`,
`ParseBuildRef`, `BuildRef.Segment` and `BuildRef.Number`. The CLI and the
client must use these functions and must not reimplement them.

## Rules for all implementers

- Never call the real Jenkins. All tests use `net/http/httptest`. Never print or
  log the API token or `Authorization` headers.
- Fixtures in `testdata/` are synthetic (for example "Jane Doe" and
  `jenkins.example.com`, with fake SHAs). They copy the shape of real Jenkins
  responses, never their values.
- Tests that touch the keyring call `keyring.MockInit()`. Tests that touch config
  set `XDG_CONFIG_HOME` to `t.TempDir()`.
- Check third-party APIs with `go doc` rather than relying on memory.
- Write code and comments in English, and comment only where it helps.

## Error codes and exit codes

| code | exit | notes |
|---|---|---|
| USAGE | 2 | bad flags/args/input |
| NOT_FOUND | 3 | HTTP 404 (except test report, see below) |
| POLICY_DENIED | 4 | tier or allow/deny globs |
| FORBIDDEN | 4 | HTTP 403 (after crumb retry for POST) |
| CONFLICT | 5 | HTTP 409 |
| AUTH_REQUIRED | 6 | no token, HTTP 401 |
| NEEDS_CONFIRMATION | 7 | danger op without confirm; Details = Plan |
| UPSTREAM | 1 | 5xx/timeouts Retryable=true (false for trigger/replay POSTs that may have reached Jenkins); other 4xx Retryable=false |
| INTERNAL | 1 | bugs, not implemented |

`errs.FromHTTPStatus(status, msg)` performs the HTTP mapping. In JSON mode the
CLI writes errors to stderr as
`{"error":{"code":..,"message":..,"hint":..,"retryable":..,"details":..}}`
(`errs.Envelope`) and exits with `errs.ExitCode(code)`. In MCP, a business
error becomes a `CallToolResult` with `IsError: true`, and its text content is
the same inner JSON object. `mcpx.ToolError` produces it by returning an error
whose `Error()` string is that JSON. The SDK copies that string into the
content.

## Tiers and policy

- Tiers: `read < write < danger` (`policy.TierRead=1`, `TierWrite=2`, `TierDanger=3`).
- With no config file the tier is `read`. The config field `policy.tier` can be
  `read`, `write` or `danger`. The `--tier` flag (CLI, and `jk mcp --tier`)
  can only LOWER the tier. `policy.EffectiveTier(configured, flag)` returns
  POLICY_DENIED when the flag asks for more than the config allows. The
  JENKINS_* variables cannot raise the tier. This is not a sandbox:
  XDG_CONFIG_HOME selects the config file and --profile the profile, so a
  shell-capable agent can still pick a more permissive config. `jk mcp`
  prints its profile, config path and tier to stderr at startup.
- `allow.jobs` and `deny.jobs` are doublestar globs matched against the job
  path, with "/" separators (for example `team-a/**`). Deny wins, and an empty
  allow list allows everything. The globs apply to every op whose TargetKind is
  `"job"`. `policy.Rules` is `map[string][]string`, and the engine looks up both
  the kind (`job`) and its plural (`jobs`). `Engine.Check` skips the glob check
  when the target is `""`; `job_list` at root is an example. To filter list
  results, use `Engine.Allowed` (plus `Engine.AllowedOrAncestor` for containers).
- Doublestar note: `team-a/**` also matches `team-a` itself, but not `team-ab`.
- Matching is case-insensitive: globs and targets are folded with
  `strings.ToLower(strings.ToUpper(s))`, mirroring Java's
  `String.CASE_INSENSITIVE_ORDER` that Jenkins uses for item lookup (so
  `PROD/app` cannot bypass `deny: ["prod/**"]`). Folded globs are computed once
  in `NewEngine`; `Engine.Policy()` returns them as written.
- Ancestors of allowed paths are browsable. `Engine.AllowedOrAncestor` is true
  when a path is allowed or is a strict segment-wise prefix of a path an allow
  glob can match (deny still wins). `job_list` uses it for the folder argument
  and for container children; leaf jobs and all other ops use `Engine.Allowed`.
  With `allow: ["org/team-a/**"]`, the root listing shows `org` and
  `job_list org` shows `org/team-a`, but `job_get org` is POLICY_DENIED.
- `max_log_lines` (default 2000) is a hard cap on tail and search output; use
  `Engine.ClampLines`.
- Write ops accept DryRun, which returns a `Plan` without executing anything.
- Danger ops require `Confirm=true`. Without it they return NEEDS_CONFIRMATION
  with `Details=Plan`. The CLI sets Confirm from `--yes` and never prompts when
  stdin or stdout is not a TTY. MCP danger tools have a `confirm` boolean input
  that defaults to false.
- MCP does not register ops above the effective tier (`mcpserver.New`).
- The Service performs the policy check in every op, using
  `s.policy.Check(op.Tier, op.TargetKind, jobPath)` with the values from
  `Catalog`.

## Config

Config path: `$XDG_CONFIG_HOME/qtools/jk.yaml`, with fallback
`~/.config/qtools/jk.yaml`. A missing file is not an error.

```yaml
profiles:
  default:
    url: https://jenkins.example.com
    user: jane
    policy:
      tier: write
      allow: { jobs: ["team-a/**"] }
      deny:  { jobs: ["prod/**"] }
      max_log_lines: 2000
default_profile: default
```

The profile is chosen in this order: `--profile`, then `default_profile`, then
`"default"`. `JENKINS_URL` and `JENKINS_USER`
override url and user. The token comes from
`JENKINS_API_TOKEN`, then the OS keyring (service
`jk`, account `<profile>`). If none is found the result is AUTH_REQUIRED.
The token never appears in any output.

## Job paths

- The user passes `folder/sub/job`, or `project/branch` for multibranch jobs.
  The URL becomes `/job/folder/job/sub/job/job/`, and each segment goes through
  `url.PathEscape`.
- Jenkins stores a multibranch branch name that contains "/" with the slash
  written as `%2F`. Users pass the explicit form, for example
  `proj/feature%2Ffoo`. `PathEscape` turns '%' into "%25", so the URL contains
  `feature%252Ffoo`. The shorthand `proj/feature/foo` is treated as three
  segments and is not resolved automatically; it returns NOT_FOUND.
- A full Jenkins URL is also accepted, with any context prefix, for example
  `https://jenkins.example.com/job/a/job/b/42/console`.
  `ParseJobInput` returns the job path and the BuildRef found right after the
  last job segment. Each URL segment is unescaped once.
- Client: build request URLs as `url.Parse(baseURL + JobURLPath(p) + "api/json")`
  and so on, then set the query with `url.Values`. Never unescape the path a
  second time.
- Build refs: in the MCP schema `ref` is `{"type":["string","integer"]}`
  (mcpserver passes a `TypeSchemas` override for `core.BuildRef` to
  `mcpx.AddTool`), and `BuildRef.UnmarshalJSON` accepts `42` or `"42"`;
  non-integer numbers are rejected. The value is a number, or `last | lastFailed | lastSuccessful | lastCompleted`;
  the default is `last`. Jenkins names such as `lastBuild` are accepted too.
  `BuildRef.Segment()` gives the URL segment. Ops that need a build number call
  `Backend.ResolveBuild` first, so outputs always report the number.
- If the input carries a ref (from a URL) and the `ref` argument is empty, the
  ref from the URL is used.

## Catalog (12 ops; MCP name = "jenkins_" + name)

| name | tier | target | CLI |
|---|---|---|---|
| whoami | read | - | `auth status` (also shows token source) |
| job_list | read | job | `job list [folder]` |
| job_get | read | job | `job get <job>` |
| build_get | read | job | `build get <job> [ref]` |
| build_stages | read | job | `build stages <job> [ref]` |
| build_log | read | job | `log tail <job> [ref] --lines N [--stage NAME]` |
| log_search | read | job | `log search <job> [ref] --pattern RE --context N --max-matches N` |
| test_failed | read | job | `test failed <job> [ref] --limit N` |
| queue_get | read | - | `queue get <id>` |
| build_trigger | write | job | `build trigger <job> [-p k=v ...] [--wait --timeout]` |
| build_replay | write | job | `build replay <job> <ref> --script-file F` |
| build_stop | danger | job | `build stop <job> <ref>` |

The CLI also has commands outside the catalog: `auth login` (reads the token
from stdin with `--with-token`, or from a hidden prompt only when there is a
TTY, and stores it in the keyring), `auth logout`, `auth status`, `version`,
`mcp`, and `completion` (the cobra default).

## Jenkins API facts (controller 2.580.1; pipeline-rest-api 2.41, junit, workflow-multibranch, cloudbees-folder; no mcp-server plugin)

- Auth uses HTTP basic `user:apitoken`. API-token requests are normally exempt
  from CSRF checks. The client still implements a crumb fallback: if a POST gets
  403, it calls `GET /crumbIssuer/api/json` -> `{crumbRequestField, crumb}` and
  retries once with that header.
- Always narrow responses with `?tree=...`. `url.Values` encodes the brackets,
  and Jenkins accepts that.
- whoami: `/me/api/json?tree=id,fullName`
- jobs: `/job/X/api/json?tree=jobs[name,_class,color,url]`; for the root, use
  `/api/json?tree=jobs[...]`. A `_class` of WorkflowMultiBranchProject or Folder
  means the job is a container.
- job: `tree=name,url,_class,color,buildable,lastBuild[number],lastSuccessfulBuild[number],lastFailedBuild[number],lastCompletedBuild[number],property[parameterDefinitions[name,type,description,defaultParameterValue[value],choices]]`
- build: `/job/X/<n|lastBuild|lastFailedBuild|lastSuccessfulBuild|lastCompletedBuild>/api/json?tree=number,result,building,duration,estimatedDuration,timestamp,url,actions[causes[shortDescription],parameters[name,value],lastBuiltRevision[SHA1]],changeSets[items[commitId,msg,author[fullName]]]`.
  `actions` contains many empty `{}` entries; skip them.
- stages: `/job/X/<n>/wfapi/describe` -> `{id,name,status,durationMillis,stages:[{id,name,status,startTimeMillis,durationMillis}]}`.
  To get one stage's log, call `GET /job/X/<n>/execution/node/<stageId>/wfapi/describe`,
  which returns `stageFlowNodes[{id,name,status}]`. Then call
  `/execution/node/<id>/wfapi/log` for each node, which returns
  `{nodeId,nodeStatus,length,hasMore,text(HTML-escaped),consoleUrl}`, and
  HTML-unescape the text.
- full log: `/job/X/<n>/consoleText` can be 500KB+ with 5k+ lines, so STREAM
  it. Tail uses a ring buffer of N lines. Search uses a line scanner with the
  regex and a context ring, and stops at max matches. Report `total_lines` and
  a `truncated` flag. Never read the whole log into memory with `io.ReadAll`;
  use `bufio.Scanner` with a large max token size of 1MB per line.
- tests: `/job/X/<n>/testReport/api/json?tree=failCount,passCount,skipCount,suites[cases[className,name,status,errorDetails,duration]]`.
  Return only FAILED and REGRESSION cases, with errorDetails trimmed to about
  500 chars. A 404 means there is no test report: return `has_report=false`, not
  an error.
- queue: `/queue/item/<id>/api/json?tree=id,why,blocked,cancelled,executable[number,url],task[name,url]`
- trigger: for a parameterized job, `POST /job/X/buildWithParameters` with form
  values; otherwise `POST /job/X/build`. The queue id comes from the Location
  header, `.../queue/item/<id>/`. With `--wait`, poll the queue item until it is
  executable, then poll the build until it finishes, bounded by `--timeout`.
- replay: `POST /job/X/<n>/replay/run` with the form field `mainScript`, plus a
  `json` payload if needed. Document the limitation in the README.
- stop: `POST /job/X/<n>/stop`
- HTTP 401 maps to AUTH_REQUIRED, 403 to FORBIDDEN, 404 to NOT_FOUND, 409 to
  CONFLICT, and 5xx to UPSTREAM (retryable, except for non-idempotent POSTs,
  see Client below). The default timeout is 30s per
  request; log streaming is bound to the context instead.

## CLI UX (agent-first)

- Stdout carries data only. Output is JSON when `--json` or `--jq` is set, or
  when stdout is not a TTY (`clix.Globals.JSONMode`).
- `--fields a,b` keeps only those top-level keys; for lists it projects each
  item, and for a list-valued field such as `jobs` it projects each element.
- `--jq EXPR` is evaluated with gojq.
- Lists default to `--limit 50`.
- The CLI never prompts without a TTY.
- Durations are human-readable in table mode and in milliseconds in JSON.
  Timestamps are RFC3339 in JSON.
- To control human output, result types may implement `output.Tabular` or
  `output.Texter`. Those methods belong in core, next to the types, or in a
  cli-side wrapper.

## Wiring (cmd/jk/main.go)

`cli.Deps.Open(ctx, globals)` runs inside each command's RunE, after cobra has
parsed the flags:

1. Load the config and resolve the profile.
2. Return USAGE if there is no URL.
3. Compute `policy.EffectiveTier(profile.Policy.Tier, g.Tier)`.
4. Resolve the token.
5. Build `client.New(url, user, token)` and `core.NewService(client, policy.NewEngine(pol))`.
6. Return a `cli.Env{Service, Profile, Tier}`.

Because this is lazy, `version`, `completion` and `auth login` work without a
token. `Deps.ServeMCP(ctx, env)` runs `mcpserver.Run(ctx, mcpserver.New(env.Service, env.Tier, version))`.

## MCP specifics

- `mcpx.AddTool[In,Out](server, name, description, tier, handler)` infers the
  schemas from In and Out through google/jsonschema-go:
  - The `jsonschema:"..."` tag is the plain property description. It must not
    start with `word=`.
  - Fields without `omitempty` become `required`. Every optional input field
    therefore has `omitempty`.
  - Slices and pointers allow null, so zero-valued outputs validate.
    `mcpserver/server_test.go` checks this for all 12 In/Out pairs.
- Annotations by tier:

  | tier | annotations |
  |---|---|
  | Read | ReadOnly, Idempotent |
  | Write | Destructive=false |
  | Danger | Destructive=true |

  OpenWorld is always true.

## Ownership for the parallel implementers

| area | owns | notes |
|---|---|---|
| kit | `internal/kit/**` | Finish output (Project, jq, tables), clix (cobra usage-error mapping), config (Resolve edge cases plus tests), auth tests, policy tests. errs, policy and mcpx are already functional. |
| client | `internal/client/**`, `testdata/` | Implement every `core.Backend` method against httptest. |
| core | `internal/core/**` except `jobpath.go` | jobpath.go changes only by agreement. Implement the Service methods with a fake Backend in tests. |
| cli | `internal/cli/**`, `cmd/jk` | |
| mcp, docs, skills | `internal/mcpserver/**`, `README.md`, `skills/**`, `Makefile`, `.goreleaser.yaml` | |

## Behaviour added during implementation

These are additive to the contracts above. The generated API below is the
source of truth for signatures.

- **Results**
  - `BuildLogOut.spilled_to` and `LogSearchOut.spilled_to`: when the result is
    larger than `core.SpillThreshold` (64KB) it is written with `output.Spill` to
    `os.TempDir()/jk/<op>-<build>-<random>.txt`. That dir is used only when
    it is a real directory owned by the current user with mode 0700 (checked
    with Lstat); otherwise a fresh `os.MkdirTemp(os.TempDir(), "jk-*")` is
    used, so another user cannot pre-create it in a shared /tmp. A spilled tail has empty
    `text` but keeps `lines`/`total_lines`. A spilled search file holds
    grep-style text (`N:match`, `N-context`), and the inline matches drop their
    context and are cut to 200 characters. The spill file name never contains
    the job path.
  - `log_search` context is capped at `core.MaxSearchContext` (50) per side, and
    the total output (matches plus context) is capped at `max_log_lines`.
- **Order inside every Service method:** tier check, `ParseJobInput`, allow/deny
  check on the parsed path, input validation, then backend calls (including
  resolving `last` and the other permalinks to a number). The backend is never
  called when policy or input validation fails.
- **NEEDS_CONFIRMATION** (build_stop): the ref is resolved to a number first;
  the hint names it, e.g. `re-run with confirm=true (CLI: --yes) and ref="57"`,
  so a later `last` cannot point at another build. `dry_run` wins over confirm.
  The plan in `details` carries the resolved `number`; the interactive CLI
  prompt pins `ref` to it before the confirmed call.
- **Trigger:** `GetJob` decides between `/build` and `/buildWithParameters`.
  Params for a job without parameter definitions are USAGE. Any error after
  the build was queued is not retryable, carries the partial result (with
  `queue_id`) in `details`, and its hint says not to re-trigger. A wait timeout
  is `timed_out=true`, not an error.
- **Client**
  - `ListJobs` on a non-container job is USAGE.
  - `ResolveBuild` with a numeric ref makes no request.
  - `GetTestReport` treats every 404 as "no report".
  - `ReplayBuild` usually returns queue id 0. A 5xx from replay is UPSTREAM but
    not retryable, with a hint about loaded or library scripts.
  - `postForm(..., idempotent)`: for trigger and replay (idempotent=false),
    timeouts, post-connect network errors and 5xx are returned with
    Retryable=false and a hint to check job_get/build_get, because Jenkins may
    already have queued the build. Dial failures (request never sent) stay
    retryable. `StopBuild` is idempotent and keeps the default.
  - POSTs do not follow redirects, so the `Location` header (queue id) is kept.
    The crumb retry also resends the cookies from the crumb response.
  - `StreamConsole` is bounded only by the caller's context. A client built
    with `WithHTTPClient` must use `Timeout: 0`, or log streams are cut.
  - Logged: method, path and status only. HTML error bodies are not copied into
    error messages.
- **Policy:** `Policy.Validate()` rejects an unknown tier, a negative
  `max_log_lines` and malformed globs (USAGE). POLICY_DENIED errors carry a
  hint: a tier denial says only a config edit can raise it.
- **Config:** YAML is decoded strictly, so an unknown key is USAGE. A file
  readable by group or others adds a warning to `File.Warnings` /
  `Resolved.Warnings` and logs it with `slog.Warn` on stderr (skipped on
  Windows). A missing or empty file is not an error. A named or
  `default_profile` profile that does not exist is USAGE unless an env URL is
  set.
- **Auth:** `(*Resolver).Source(profile)` returns `env:NAME`, `keyring` or
  `none` without returning the token. `Resolve` returns `SourceNone` on
  failure. `Delete` succeeds when the entry is already missing.
- **CLI:** `clix.PrepareUsageErrors` and `clix.GroupRunE` turn unknown
  commands, bad flags and wrong argument counts into USAGE (exit 2). `-y` is the
  short form of `--yes`. In text mode, errors also print `details`.
  `output.HumanDuration` formats `*_ms` values in human output. `--fields`
  projects list-valued fields per element, so `--fields name` on `job get`
  also keeps `parameters[].name`.
- **MCP startup:** `jk mcp` resolves the profile and token before it
  serves, so a missing URL or token makes it exit (code 2 or 6) with the error
  on stderr. `mcpserver.New` filters by the `tier` argument; main passes the
  same effective tier it gives the policy engine.

## Package API (generated by `go doc -all`)

### internal/core

```go
package core // import "github.com/phuquocchamp/jk/internal/core"


CONSTANTS

const (
	TargetNone = ""
	TargetJob  = "job" // matched against policy rules "jobs"
)
    Target kinds used for policy allow/deny checks.

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
    Op names (MCP tool name = MCPPrefix + Name).

const (
	DefaultListLimit      = 50
	DefaultMaxMatches     = 50
	DefaultTriggerTimeout = 600 * time.Second
	DefaultPollInterval   = 2 * time.Second
	ErrorDetailsMaxChars  = 500
	// SpillThreshold is the size above which log text is written to a file
	// (output.Spill) and only its path is returned.
	SpillThreshold = 64 * 1024
	// MaxSearchContext caps log_search context lines on each side.
	MaxSearchContext = 50
)
    Defaults used by Service.

const (
	KindJob         = "job"         // freestyle or other buildable job
	KindPipeline    = "pipeline"    // WorkflowJob (incl. multibranch branch jobs)
	KindFolder      = "folder"      // cloudbees Folder (container)
	KindMultibranch = "multibranch" // WorkflowMultiBranchProject (container)
	KindOther       = "other"
)
    Job kinds derived from Jenkins _class.

const MCPPrefix = "jenkins_"
    MCPPrefix is prepended to Op.Name for MCP tool names.


VARIABLES

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
    Catalog lists all operations in display order.


FUNCTIONS

func JobURLPath(path string) string
    JobURLPath converts a normalized job path to its URL path: "a/b" ->
    "/job/a/job/b/". Each segment is url.PathEscape'd, so '%' becomes "%25"
    ("proj/feature%2Ffoo" -> "/job/proj/job/feature%252Ffoo/"). The empty path
    returns "/". The result is an already-escaped path: build the request URL
    with url.Parse(base + JobURLPath(p)) and never unescape it.

func NormalizeJobPath(s string) (string, error)
    NormalizeJobPath trims surrounding slashes/space and validates segments.
    The path uses "/" separators and the job names exactly as Jenkins stores
    them (a multibranch branch "feature/foo" is the job "feature%2Ffoo").


TYPES

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

	// GetNodeLog: GET /job/X/<n>/execution/node/<nodeID>/wfapi/log (Text HTML-unescaped).
	GetNodeLog(ctx context.Context, job string, number int, nodeID string) (NodeLog, error)

	// StreamConsole: GET /job/X/<n>/consoleText. Caller must Close. The body
	// is streamed; the request is bound to ctx (no fixed whole-request timeout).
	StreamConsole(ctx context.Context, job string, number int) (io.ReadCloser, error)

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
    Backend is the Jenkins transport, implemented by internal/client.

    Conventions for implementers:
      - job is a normalized job path (see NormalizeJobPath); build URLs use
        JobURLPath.
      - Errors are *errs.Error mapped from HTTP status (errs.FromHTTPStatus);
        transport timeouts are UPSTREAM retryable (not for trigger/replay POSTs).
      - Backend does no policy checks, ref defaulting, limiting or trimming;
        Service does that.

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
    Build is a single build.

type BuildGetIn struct {
	Job string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref BuildRef `json:"ref,omitempty" jsonschema:"build number as a string (e.g. \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
}

type BuildGetOut = Build

type BuildLogIn struct {
	Job   string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref   BuildRef `json:"ref,omitempty" jsonschema:"build number as a string (e.g. \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
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

type BuildRef string
    BuildRef identifies a build: a positive number or one of the permalinks.

const (
	RefLast           BuildRef = "last"
	RefLastFailed     BuildRef = "lastFailed"
	RefLastSuccessful BuildRef = "lastSuccessful"
	RefLastCompleted  BuildRef = "lastCompleted"
)
func ParseBuildRef(s string) (BuildRef, error)
    ParseBuildRef normalizes s; "" means last. Jenkins permalink names
    (lastBuild, lastFailedBuild, ...) are accepted case-insensitively.

func ParseJobInput(s string) (string, BuildRef, error)
    ParseJobInput accepts a job path, a "/job/a/job/b" path, or a full Jenkins
    URL (any context prefix, e.g. https://host/jenkins/job/a/job/b/42/console)
    and returns the job path plus the build ref found right after the
    last job segment ("" when none). URL segments are unescaped once,
    so ".../job/feature%252Ffoo/" yields the job name "feature%2Ffoo".

func (r BuildRef) Number() (int, bool)
    Number returns the build number when r is numeric.

func (r BuildRef) Segment() string
    Segment returns the URL path segment for r: "42", "lastBuild",
    "lastFailedBuild", ... r must be normalized by ParseBuildRef; "" maps to
    lastBuild.

type BuildReplayIn struct {
	Job    string   `json:"job" jsonschema:"job path or full Jenkins build URL"`
	Ref    BuildRef `json:"ref" jsonschema:"build to replay: number as a string (e.g. \"42\") or last, lastFailed, lastSuccessful, lastCompleted"`
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

type BuildStagesIn struct {
	Job string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref BuildRef `json:"ref,omitempty" jsonschema:"build number as a string (e.g. \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
}

type BuildStagesOut struct {
	Job        string  `json:"job"`
	Number     int     `json:"number"`
	Status     string  `json:"status"`
	DurationMs int64   `json:"duration_ms"`
	Stages     []Stage `json:"stages"`
}

type BuildStopIn struct {
	Job     string   `json:"job" jsonschema:"job path or full Jenkins build URL"`
	Ref     BuildRef `json:"ref" jsonschema:"build to stop: number as a string (e.g. \"42\") or last"`
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

type Change struct {
	CommitID string `json:"commit_id"`
	Message  string `json:"message"`
	Author   string `json:"author,omitempty"`
}
    Change is one SCM change in a build.

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
    Job is a job with its permalinks and parameter definitions.

type JobGetIn struct {
	Job string `json:"job" jsonschema:"job path like folder/sub/job (multibranch: project/branch, encode / in branch names as %2F) or a full Jenkins job URL"`
}

type JobGetOut = Job

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

type JobSummary struct {
	Name      string `json:"name" jsonschema:"job name as stored by Jenkins (branch names may contain %2F)"`
	Path      string `json:"path" jsonschema:"full job path with / separators; pass it to other tools"`
	Kind      string `json:"kind" jsonschema:"job, pipeline, folder, multibranch or other"`
	Container bool   `json:"container" jsonschema:"true for folders and multibranch projects (list them with job_list)"`
	Color     string `json:"color,omitempty" jsonschema:"Jenkins status color, e.g. blue, red, blue_anime"`
	URL       string `json:"url" jsonschema:"Jenkins URL of the job"`
}
    JobSummary is one entry of a job listing.

type LogMatch struct {
	Line   int      `json:"line"`
	Text   string   `json:"text"`
	Before []string `json:"before,omitempty"`
	After  []string `json:"after,omitempty"`
}
    LogMatch is one search hit; Line is 1-based.

type LogSearchIn struct {
	Job        string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref        BuildRef `json:"ref,omitempty" jsonschema:"build number as a string (e.g. \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
	Pattern    string   `json:"pattern" jsonschema:"Go RE2 regular expression matched per line"`
	Context    int      `json:"context,omitempty" jsonschema:"lines of context before and after each match (default 0)"`
	MaxMatches int      `json:"max_matches,omitempty" jsonschema:"stop after this many matches (default 50)"`
}

type LogSearchOut struct {
	Job        string     `json:"job"`
	Number     int        `json:"number"`
	Pattern    string     `json:"pattern"`
	Matches    []LogMatch `json:"matches"`
	TotalLines int        `json:"total_lines" jsonschema:"lines scanned"`
	Truncated  bool       `json:"truncated" jsonschema:"true when max_matches (or the line cap) stopped the scan early"`
	SpilledTo  string     `json:"spilled_to,omitempty" jsonschema:"set when the matches were too large to inline: local file holding them as grep-style text (matches then carry no context)"`
}

type NodeLog struct {
	NodeID     string `json:"node_id"`
	Status     string `json:"status"`
	Length     int64  `json:"length"`
	HasMore    bool   `json:"has_more"`
	Text       string `json:"text"`
	ConsoleURL string `json:"console_url,omitempty"`
}
    NodeLog is the log of one flow node (wfapi/log). Text is already
    HTML-unescaped.

type Op struct {
	Name       string
	CLIPath    string // usage line under the root command
	Tier       policy.Tier
	TargetKind string
	Summary    string // one line, agent-facing: when to use + what it returns
}
    Op describes one operation for both CLI and MCP.

func OpByName(name string) (Op, bool)
    OpByName returns the catalog entry for name (without MCP prefix).

func (o Op) MCPName() string
    MCPName returns the MCP tool name.

type Option func(*Service)
    Option configures a Service.

func WithClock(now func() time.Time) Option
    WithClock overrides time.Now (tests).

func WithPollInterval(d time.Duration) Option
    WithPollInterval overrides DefaultPollInterval.

func WithSleep(f func(ctx context.Context, d time.Duration) error) Option
    WithSleep overrides the poll sleep used by build_trigger --wait (tests).

func WithSpill(f func(content, prefix string) (string, error)) Option
    WithSpill overrides output.Spill (tests).

type Param struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}
    Param is a parameter value used by a build.

type ParamDef struct {
	Name        string   `json:"name"`
	Type        string   `json:"type" jsonschema:"Jenkins parameter definition class short name"`
	Description string   `json:"description,omitempty"`
	Default     any      `json:"default,omitempty"`
	Choices     []string `json:"choices,omitempty"`
}
    ParamDef is a job parameter definition.

type Plan struct {
	Op     string            `json:"op"`
	Method string            `json:"method"`
	Path   string            `json:"path" jsonschema:"request path relative to the Jenkins base URL"`
	Params map[string]string `json:"params,omitempty" jsonschema:"form parameters that would be sent"`
	Note   string            `json:"note,omitempty"`
}
    Plan describes the HTTP call a write/danger op would make (dry-run,
    or details of NEEDS_CONFIRMATION). It never contains credentials.

type QueueGetIn struct {
	ID int64 `json:"id" jsonschema:"queue item id (returned by build_trigger)"`
}

type QueueGetOut = QueueItem

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
    QueueItem is a Jenkins queue item.

type Run struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	DurationMs int64   `json:"duration_ms"`
	Stages     []Stage `json:"stages"`
}
    Run is a pipeline run (wfapi/describe).

type Service struct {
	// Has unexported fields.
}
    Service implements every catalog op. It owns input normalization
    (ParseJobInput, ParseBuildRef, defaults), policy checks (tier + job globs
    via policy.Engine.Check with the op's Catalog Tier/TargetKind), dry-run
    plans, confirmation for danger ops, limits/trimming, and log streaming
    logic.

func NewService(b Backend, p *policy.Engine, opts ...Option) *Service
    NewService builds a Service. A nil engine means
    policy.NewEngine(policy.Policy{}) (read tier).

func (s *Service) BuildGet(ctx context.Context, in BuildGetIn) (BuildGetOut, error)

func (s *Service) BuildLog(ctx context.Context, in BuildLogIn) (BuildLogOut, error)
    BuildLog returns the last N lines of the console log, or of one stage's log
    when Stage is set. The console is streamed through a ring buffer, so memory
    stays bounded by N lines.

func (s *Service) BuildReplay(ctx context.Context, in BuildReplayIn) (BuildReplayOut, error)
    BuildReplay re-runs a pipeline build with a replacement main script.

func (s *Service) BuildStages(ctx context.Context, in BuildStagesIn) (BuildStagesOut, error)

func (s *Service) BuildStop(ctx context.Context, in BuildStopIn) (BuildStopOut, error)
    BuildStop aborts a build. The ref is resolved to a number first so the plan
    and the confirmation hint name a fixed build.

func (s *Service) BuildTrigger(ctx context.Context, in BuildTriggerIn) (BuildTriggerOut, error)
    BuildTrigger starts a build. Parameterized jobs (non-empty parameter
    definitions) use buildWithParameters. With Wait it polls the queue item
    and then the build until it finishes or the timeout passes (TimedOut=true,
    nil error).

func (s *Service) JobGet(ctx context.Context, in JobGetIn) (JobGetOut, error)

func (s *Service) JobList(ctx context.Context, in JobListIn) (JobListOut, error)
    JobList lists the direct children of a folder. Children hidden by the
    allow/deny globs are dropped before counting and limiting.

func (s *Service) LogSearch(ctx context.Context, in LogSearchIn) (LogSearchOut, error)
    LogSearch scans the console log line by line and stops as soon as
    max_matches (and their trailing context) are collected, or the policy line
    cap on output (matches + context) is reached.

func (s *Service) Policy() *policy.Engine
    Policy returns the policy engine (used by mcpserver/cli for tier filtering).

func (s *Service) QueueGet(ctx context.Context, in QueueGetIn) (QueueGetOut, error)

func (s *Service) TestFailed(ctx context.Context, in TestFailedIn) (TestFailedOut, error)
    TestFailed returns failed cases only, error details trimmed. A build with no
    report returns has_report=false and no error.

func (s *Service) Whoami(ctx context.Context, _ WhoamiIn) (WhoamiOut, error)

type Stage struct {
	ID         string    `json:"id" jsonschema:"stage flow node id; usable with build_log stage"`
	Name       string    `json:"name"`
	Status     string    `json:"status" jsonschema:"SUCCESS, FAILED, UNSTABLE, ABORTED, IN_PROGRESS, NOT_EXECUTED, PAUSED_PENDING_INPUT"`
	StartTime  time.Time `json:"start_time"`
	DurationMs int64     `json:"duration_ms"`
}
    Stage is a pipeline stage from wfapi.

type StageNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}
    StageNode is a flow node inside a stage
    (execution/node/<id>/wfapi/describe).

type TestCase struct {
	ClassName    string `json:"class_name"`
	Name         string `json:"name"`
	Status       string `json:"status" jsonschema:"FAILED or REGRESSION"`
	ErrorDetails string `json:"error_details,omitempty" jsonschema:"error message, trimmed to about 500 chars"`
	DurationMs   int64  `json:"duration_ms"`
}
    TestCase is a failed test case.

type TestFailedIn struct {
	Job   string   `json:"job" jsonschema:"job path or full Jenkins job/build URL"`
	Ref   BuildRef `json:"ref,omitempty" jsonschema:"build number as a string (e.g. \"42\") or last, lastFailed, lastSuccessful, lastCompleted (default last)"`
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

type TestReport struct {
	HasReport bool       `json:"has_report" jsonschema:"false when the build has no test report"`
	FailCount int        `json:"fail_count"`
	PassCount int        `json:"pass_count"`
	SkipCount int        `json:"skip_count"`
	Failed    []TestCase `json:"failed"`
}
    TestReport summarizes a build's junit report. Failed holds only
    FAILED/REGRESSION cases.

type User struct {
	ID       string `json:"id" jsonschema:"Jenkins user id"`
	FullName string `json:"full_name" jsonschema:"display name"`
}
    User is the authenticated Jenkins user.

type WhoamiIn struct{}

type WhoamiOut struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
	URL      string `json:"url" jsonschema:"Jenkins base URL of the active profile"`
}

```

### internal/client

```go
package client // import "github.com/phuquocchamp/jk/internal/client"

Package client is the Jenkins REST adapter implementing core.Backend.

CONSTANTS

const DefaultTimeout = 30 * time.Second
    DefaultTimeout bounds each non-streaming request.


TYPES

type Client struct {
	// Has unexported fields.
}
    Client talks to one Jenkins controller with HTTP basic auth (user:apitoken).
    It retries a POST once with a crumb (GET /crumbIssuer/api/json) on 403.
    The token is never logged.

func New(baseURL, user, token string, opts ...Option) *Client
    New creates a Client. baseURL may have a trailing slash; it is trimmed.

func (c *Client) BaseURL() string
    BaseURL returns the configured Jenkins base URL (no trailing slash).

func (c *Client) GetBuild(ctx context.Context, job string, ref core.BuildRef) (core.Build, error)
    GetBuild returns build details; ref may be a number or permalink.

func (c *Client) GetJob(ctx context.Context, job string) (core.Job, error)
    GetJob returns job details incl. permalinks and parameter definitions.

func (c *Client) GetNodeLog(ctx context.Context, job string, number int, nodeID string) (core.NodeLog, error)
    GetNodeLog: GET /job/X/<n>/execution/node/<nodeID>/wfapi/log (Text
    HTML-unescaped).

func (c *Client) GetQueueItem(ctx context.Context, id int64) (core.QueueItem, error)
    GetQueueItem: GET /queue/item/<id>/api/json

func (c *Client) GetRun(ctx context.Context, job string, number int) (core.Run, error)
    GetRun: GET /job/X/<n>/wfapi/describe

func (c *Client) GetStageNodes(ctx context.Context, job string, number int, stageID string) ([]core.StageNode, error)
    GetStageNodes: GET /job/X/<n>/execution/node/<stageID>/wfapi/describe ->
    stageFlowNodes.

func (c *Client) GetTestReport(ctx context.Context, job string, number int) (core.TestReport, error)
    GetTestReport returns counts and only FAILED/REGRESSION cases (untrimmed).

func (c *Client) ListJobs(ctx context.Context, folder string) ([]core.JobSummary, error)
    ListJobs lists direct children of folder ("" = root). A non-container job
    (no "jobs" field in the response) is a USAGE error.

func (c *Client) ReplayBuild(ctx context.Context, job string, number int, script string) (int64, error)
    ReplayBuild POSTs /job/X/<n>/replay/run with mainScript (also as the
    structured "json" form field Jenkins' form binding reads). Loaded library
    scripts are not sent. Returns the queue id when Jenkins reports one, else 0.

func (c *Client) ResolveBuild(ctx context.Context, job string, ref core.BuildRef) (int, error)
    ResolveBuild turns a ref into a build number. Numbers are returned without a
    request.

func (c *Client) StopBuild(ctx context.Context, job string, number int) error
    StopBuild: POST /job/X/<n>/stop

func (c *Client) StreamConsole(ctx context.Context, job string, number int) (io.ReadCloser, error)
    StreamConsole: GET /job/X/<n>/consoleText. The body is returned unread and
    bound only to ctx (no per-request timeout), so large logs can be streamed.

func (c *Client) TriggerBuild(ctx context.Context, job string, params map[string]string, parameterized bool) (int64, error)
    TriggerBuild POSTs /build, or /buildWithParameters with form values.

func (c *Client) Whoami(ctx context.Context) (core.User, error)
    Whoami: GET /me/api/json?tree=id,fullName

type Option func(*Client)
    Option configures a Client.

func WithHTTPClient(h *http.Client) Option
    WithHTTPClient sets the underlying http.Client (tests pass httptest server
    clients). Its Timeout should be zero: a whole-request timeout would cut
    log streams; the client applies its own per-request timeout (WithTimeout)
    instead.

func WithLogger(l *slog.Logger) Option
    WithLogger sets the debug logger (stderr only; never logs credentials).

func WithTimeout(d time.Duration) Option
    WithTimeout sets the per-request timeout for non-streaming calls.

func WithUserAgent(ua string) Option
    WithUserAgent sets the User-Agent header.

```

### internal/cli

```go
package cli // import "github.com/phuquocchamp/jk/internal/cli"

Package cli defines the cobra command tree. Each catalog command calls exactly
one core.Service method and renders the result with kit/output.

FUNCTIONS

func NewRootCmd(d Deps) *cobra.Command
    NewRootCmd builds the root command with global flags bound to d.Globals.


TYPES

type AuthLoginOut struct {
	Profile string `json:"profile"`
	Stored  bool   `json:"stored"`
	Backend string `json:"backend"`
}
    AuthLoginOut is the result of 'auth login'.

type AuthLogoutOut struct {
	Profile string `json:"profile"`
	Removed bool   `json:"removed"`
}
    AuthLogoutOut is the result of 'auth logout'.

type AuthStatusOut struct {
	Profile     string `json:"profile"`
	URL         string `json:"url"`
	User        string `json:"user,omitempty"`
	ID          string `json:"id"`
	FullName    string `json:"full_name"`
	TokenSource string `json:"token_source"`
	Tier        string `json:"tier"`
	ConfigPath  string `json:"config_path,omitempty"`
}
    AuthStatusOut is the result of 'auth status'. It never contains the token.

type Deps struct {
	Version string
	Globals *clix.Globals
	// Open loads config for g.Profile, computes the effective tier from g.Tier,
	// resolves the token and builds client + Service. Called lazily inside RunE,
	// so version/completion/auth login work without a token.
	Open func(ctx context.Context, g *clix.Globals) (*Env, error)
	// LoadProfile resolves a profile without needing a token (auth commands).
	LoadProfile func(profile string) (config.Resolved, error)
	Auth        *auth.Resolver
	Stdin       *os.File
	Stdout      io.Writer
	Stderr      io.Writer
	// ServeMCP runs the MCP server on stdio for env (wired by main to mcpserver).
	ServeMCP func(ctx context.Context, env *Env) error
}
    Deps are injected by main (and by tests).

type Env struct {
	Service *core.Service
	Profile config.Resolved
	Tier    policy.Tier // effective tier = min(config, --tier)
}
    Env is what a command needs after flags are parsed.

type VersionOut struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
    VersionOut is the result of 'version'.

func (v VersionOut) PlainText() string

```

### internal/mcpserver

```go
package mcpserver // import "github.com/phuquocchamp/jk/internal/mcpserver"

Package mcpserver exposes core.Catalog as MCP tools over stdio.

FUNCTIONS

func New(svc *core.Service, tier policy.Tier, version string) *mcp.Server
    New builds an MCP server registering every catalog op whose tier <= tier.
    Ops above the tier are not registered at all.

func Run(ctx context.Context, s *mcp.Server) error
    Run serves s over stdio until ctx is cancelled or the client disconnects.

```

### internal/kit/errs

```go
package errs // import "github.com/phuquocchamp/jk/internal/kit/errs"

Package errs defines the structured error model shared by CLI and MCP surfaces.

Every error that crosses a package boundary should be an *Error so that the CLI
can map it to an exit code and the MCP server can return {code,message,hint}.

FUNCTIONS

func ExitCode(c Code) int
    ExitCode maps a code to the process exit status used by the CLI.

func ExitCodeOf(err error) int
    ExitCodeOf returns the CLI exit code for err (0 for nil).

func Is(err error, code Code) bool
    Is reports whether err carries the given code.


TYPES

type Code string
    Code is a stable, machine-readable error category.

const (
	CodeUsage             Code = "USAGE"
	CodeNotFound          Code = "NOT_FOUND"
	CodePolicyDenied      Code = "POLICY_DENIED"
	CodeForbidden         Code = "FORBIDDEN"
	CodeConflict          Code = "CONFLICT"
	CodeAuthRequired      Code = "AUTH_REQUIRED"
	CodeNeedsConfirmation Code = "NEEDS_CONFIRMATION"
	CodeUpstream          Code = "UPSTREAM"
	CodeInternal          Code = "INTERNAL"
)
type Envelope struct {
	Error *Error `json:"error"`
}
    Envelope is the JSON shape written to stderr in JSON mode: {"error": {...}}.

type Error struct {
	Code      Code   `json:"code"`
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	Retryable bool   `json:"retryable"`
	Details   any    `json:"details,omitempty"`
	Err       error  `json:"-"`
}
    Error is the structured error. Err is the wrapped cause and is never
    serialized.

func As(err error) (*Error, bool)
    As returns the *Error in err's chain, if any.

func From(err error) *Error
    From converts any error into an *Error. Context deadline errors become a
    retryable UPSTREAM error; context cancellation and unknown errors become
    INTERNAL. From(nil) returns nil.

func FromHTTPStatus(status int, msg string) *Error
    FromHTTPStatus maps an HTTP status to an error. It returns nil for 2xx/3xx.
    401 -> AUTH_REQUIRED, 403 -> FORBIDDEN, 404 -> NOT_FOUND, 409 -> CONFLICT,
    5xx -> UPSTREAM (retryable), other 4xx -> UPSTREAM (not retryable).

func Internal(msg string) *Error

func New(code Code, msg string) *Error
    New creates an error with the given code and message.

func Newf(code Code, format string, args ...any) *Error
    Newf creates an error with a formatted message.

func NotFound(msg string) *Error

func NotImplemented(what string) *Error
    NotImplemented is used by stubs.

func PolicyDenied(msg string) *Error

func Usage(msg string) *Error
    Shorthand constructors for the common cases.

func Usagef(format string, args ...any) *Error

func Wrap(code Code, cause error, msg string) *Error
    Wrap creates an error with the given code that wraps cause.

func (e *Error) Error() string

func (e *Error) Unwrap() error

func (e *Error) WithDetails(d any) *Error
    WithDetails sets the details and returns e for chaining.

func (e *Error) WithHint(hint string) *Error
    WithHint sets the hint and returns e for chaining.

func (e *Error) WithRetryable(r bool) *Error
    WithRetryable sets the retryable flag and returns e for chaining.

```

### internal/kit/policy

```go
package policy // import "github.com/phuquocchamp/jk/internal/kit/policy"

Package policy implements tiered, tighten-only access control with glob
allow/deny rules per target kind. It is product-agnostic.

CONSTANTS

const DefaultMaxLogLines = 2000
    DefaultMaxLogLines is used when Policy.MaxLogLines is 0.


TYPES

type Engine struct {
	// Has unexported fields.
}
    Engine evaluates a Policy.

func NewEngine(p Policy) *Engine
    NewEngine returns an engine for p; zero Tier becomes TierRead and zero
    MaxLogLines becomes DefaultMaxLogLines.

func (e *Engine) Allowed(targetKind, target string) bool
    Allowed reports whether target passes allow/deny globs for targetKind.
    Deny wins; an empty allow list allows everything.

func (e *Engine) Check(op Tier, targetKind, target string) error
    Check returns POLICY_DENIED if op exceeds the tier or target is not allowed.
    An empty targetKind or target skips the glob check.

func (e *Engine) ClampLines(n int) (int, bool)
    ClampLines caps n to MaxLogLines; n <= 0 means "use the cap". The bool
    reports whether the request was reduced.

func (e *Engine) MaxLogLines() int

func (e *Engine) Policy() Policy

func (e *Engine) Tier() Tier

type Policy struct {
	Tier        Tier
	Allow       Rules
	Deny        Rules
	MaxLogLines int
}
    Policy is the effective policy for one profile.

func (p Policy) Validate() error
    Validate reports malformed globs, an out-of-range tier or a negative
    MaxLogLines as a USAGE error. A zero Tier or MaxLogLines is valid
    (defaults).

type Rules map[string][]string
    Rules maps a rule key to doublestar globs. The key is the target kind
    ("job") or its plural as written in config ("jobs"); both are looked up.

type Tier int
    Tier orders operations by risk: Read < Write < Danger.

const (
	TierRead   Tier = 1
	TierWrite  Tier = 2
	TierDanger Tier = 3
)
func EffectiveTier(configured Tier, flag string) (Tier, error)
    EffectiveTier returns min(configured, flag). An empty flag keeps configured.
    A flag above configured returns POLICY_DENIED (tighten-only). A zero
    configured tier is treated as TierRead.

func ParseTier(s string) (Tier, error)
    ParseTier parses "read", "write" or "danger" (case-insensitive).

func (t Tier) MarshalText() ([]byte, error)

func (t Tier) String() string

func (t *Tier) UnmarshalText(b []byte) error

```

### internal/kit/auth

```go
package auth // import "github.com/phuquocchamp/jk/internal/kit/auth"

Package auth resolves API tokens from environment variables or the OS keyring.
Tokens are never logged or returned in any user-facing output.

TYPES

type Resolver struct {
	Service string
	EnvVars []string
	// Getenv defaults to os.Getenv; tests may override it.
	Getenv func(string) string
}
    Resolver finds a token for a profile. Env vars are checked in order,
    then the keyring entry (Service, account=profile).

func NewResolver(service string, envVars ...string) *Resolver
    NewResolver builds a Resolver, e.g. NewResolver("jk",
    "JENKINS_API_TOKEN").

func (r *Resolver) Delete(profile string) error
    Delete removes the keyring entry; a missing entry is not an error.

func (r *Resolver) Resolve(profile string) (string, Source, error)
    Resolve returns the token and its source, or AUTH_REQUIRED with SourceNone.
    A keyring backend failure (no Secret Service, locked keychain) is also
    AUTH_REQUIRED, with a hint pointing at the env vars.

func (r *Resolver) Source(profile string) Source
    Source reports where a token for profile would come from without returning
    it: "env:NAME", "keyring" or "none". Keyring errors yield "none".

func (r *Resolver) Store(profile, token string) error
    Store saves token in the keyring under (Service, profile). Surrounding
    whitespace (e.g. a trailing newline from stdin) is trimmed.

type Source string
    Source says where a token came from, e.g. "env:JENKINS_API_TOKEN" or
    "keyring".

const (
	SourceKeyring Source = "keyring"
	SourceNone    Source = "none"
)
func EnvSource(name string) Source
    EnvSource returns the Source for an environment variable name.

```

### internal/kit/config

```go
package config // import "github.com/phuquocchamp/jk/internal/kit/config"

Package config loads the per-tool YAML config with named profiles. Path:
$XDG_CONFIG_HOME/<Dir>/<App>.yaml, falling back to ~/.config/<Dir>/<App>.yaml.

FUNCTIONS

func Path(o Options) (string, error)
    Path returns the config file path for o.


TYPES

type File struct {
	Profiles       map[string]Profile `yaml:"profiles,omitempty"`
	DefaultProfile string             `yaml:"default_profile,omitempty"`

	// Warnings are non-fatal problems found while loading (e.g. file mode).
	Warnings []string `yaml:"-"`
	// Has unexported fields.
}
    File is the whole config file.

func Load(o Options) (*File, string, error)
    Load reads the config file. A missing or empty file yields an empty File and
    no error. Unknown keys are a USAGE error, so a typo such as "dney" cannot
    silently weaken the policy. A file readable by group or others is loaded
    but produces a warning (File.Warnings, also logged via slog). The returned
    string is the path consulted.

func (f *File) ProfileNames() []string
    ProfileNames returns the profile names in the file, sorted.

func (f *File) Resolve(name string, o Options) (Resolved, error)
    Resolve picks the profile name (empty -> DefaultProfile -> "default"),
    applies env overrides for URL/User, and builds and validates the policy.
    A profile named explicitly (argument or default_profile) that is missing
    from the file is a USAGE error unless env supplies the URL. The implicit
    "default" may be missing: env-only setups then get the read tier.

type Options struct {
	Dir     string   // e.g. "qtools"
	App     string   // e.g. "jk" -> jk.yaml
	URLEnv  []string // e.g. JENKINS_URL (first non-empty wins)
	UserEnv []string // e.g. JENKINS_USER
	// Getenv defaults to os.Getenv.
	Getenv func(string) string
}
    Options describe where to look and which env vars override url/user.

type PolicyConfig struct {
	Tier        string       `yaml:"tier,omitempty"`
	Allow       policy.Rules `yaml:"allow,omitempty"`
	Deny        policy.Rules `yaml:"deny,omitempty"`
	MaxLogLines int          `yaml:"max_log_lines,omitempty"`
}
    PolicyConfig is the YAML form of a policy.

type Profile struct {
	URL    string       `yaml:"url,omitempty"`
	User   string       `yaml:"user,omitempty"`
	Policy PolicyConfig `yaml:"policy,omitempty"`
}
    Profile is one named profile in the config file.

type Resolved struct {
	Name     string
	URL      string
	User     string
	Policy   policy.Policy // Tier defaults to read; MaxLogLines to policy.DefaultMaxLogLines
	Path     string        // config file path that was consulted
	FromFile bool          // the profile existed in the file
	Warnings []string      // copied from File.Warnings
}
    Resolved is a profile after env overrides and defaults.

```

### internal/kit/output

```go
package output // import "github.com/phuquocchamp/jk/internal/kit/output"

Package output renders command results as JSON (agents) or tables (humans),
with optional top-level field projection and jq filtering.

VARIABLES

var AppName = "jk"
    AppName names the spill directory under os.TempDir(). Set once by main.


FUNCTIONS

func HumanDuration(ms int64) string
    HumanDuration formats milliseconds as a short duration, e.g. "1m23s",
    "450ms".

func Project(v any, fields []string) any
    Project keeps only the given top-level keys of a JSON-like value. For arrays
    it projects each element. For an object, a value that is an array of objects
    whose elements carry any requested key is kept and projected per element, so
    fields "name,path" on {"jobs":[...]} keep the jobs with only name and path.
    Values that are not maps or arrays are returned unchanged.

func Render(w io.Writer, v any, o Opts) error
    Render writes v to w according to o.

    JSON mode without Fields/JQ encodes v directly (struct field order kept).
    Otherwise v is converted to a generic JSON value, projected by Fields,
    and filtered by JQ (each jq result is written as one JSON document). Human
    mode uses Texter, then Tabular, then a generic key/value and table renderer
    in which numeric keys ending in "_ms" are shown as durations.

func Spill(content, prefix string) (string, error)
    Spill writes content to os.TempDir()/<AppName>/<prefix>-<random>.txt and
    returns the path. The prefix is sanitized (path separators and other unsafe
    characters become '_'). Used when a result is too large to inline.


TYPES

type Opts struct {
	JSON   bool     // JSON output; the CLI sets this when --json or stdout is not a TTY
	Fields []string // project top-level keys (and per item for lists / list fields)
	JQ     string   // gojq expression applied after projection; implies JSON
}
    Opts control rendering.

type Tabular interface {
	TableHeader() []string
	TableRows() [][]string
}
    Tabular can be implemented by result types to control human (table) output.
    Durations should be human-readable here (JSON keeps milliseconds).

type Texter interface {
	PlainText() string
}
    Texter can be implemented by results whose human form is plain text (e.g.
    logs).

```

### internal/kit/clix

```go
package clix // import "github.com/phuquocchamp/jk/internal/kit/clix"

Package clix holds cobra helpers: global flags, TTY detection, and error ->
stderr JSON + exit code mapping.

FUNCTIONS

func BindGlobals(cmd *cobra.Command, g *Globals)
    BindGlobals registers the global flags as persistent flags on cmd.

func CanPrompt(stdin, stdout any) bool
    CanPrompt reports whether both stdin and stdout are terminals.

func Execute(root *cobra.Command, g *Globals, stdout, stderr io.Writer) int
    Execute runs root, routes errors through HandleError and returns the exit
    code. Cobra flag/arg errors and unknown commands are mapped to USAGE.
    Callers should set root.SilenceErrors and root.SilenceUsage.

func GroupRunE(c *cobra.Command, args []string) error
    GroupRunE is the RunE for commands that only group subcommands: with no args
    it prints help, otherwise it reports an unknown subcommand as USAGE (cobra
    would print help and exit 0, which misleads agents).

func HandleError(stderr io.Writer, err error, jsonMode bool) int
    HandleError writes err to stderr (JSON envelope when jsonMode, else "error:
    CODE: message" plus hint) and returns the exit code. nil -> 0.

func IsTerminal(v any) bool
    IsTerminal reports whether v is an *os.File attached to a terminal.

func PrepareUsageErrors(root *cobra.Command)
    PrepareUsageErrors makes flag parsing and positional-argument errors of
    root and all its subcommands USAGE errors. Execute calls it; tests that run
    root.Execute directly may call it too. It is idempotent.

func ReadSecret(stdin *os.File, stderr io.Writer, prompt string) (string, error)
    ReadSecret prints prompt to stderr and reads a line without echo. It returns
    USAGE when stdin is not a terminal (never prompt without a TTY).


TYPES

type Globals struct {
	Profile string   // --profile
	JSON    bool     // --json
	Fields  []string // --fields a,b
	JQ      string   // --jq EXPR
	Yes     bool     // --yes (confirm danger ops)
	DryRun  bool     // --dry-run (write/danger ops return a plan)
	Tier    string   // --tier (may only lower the configured tier)
}
    Globals are the persistent flags shared by all commands.

func (g *Globals) JSONMode(w io.Writer) bool
    JSONMode reports whether output should be JSON for stdout w.

func (g *Globals) OutputOpts(w io.Writer) output.Opts
    OutputOpts builds output.Opts for stdout w.

```

### internal/kit/mcpx

```go
package mcpx // import "github.com/phuquocchamp/jk/internal/kit/mcpx"

Package mcpx registers typed MCP tools with annotations derived from a policy
tier and turns structured errors into IsError tool results.

FUNCTIONS

func AddTool[In, Out any](s *mcp.Server, name, description string, tier policy.Tier, h Handler[In, Out])
    AddTool registers name on s with schemas inferred from In/Out. Business
    errors from h become a CallToolResult with IsError=true whose text content
    is {"code","message","hint","retryable","details"}.

func Annotations(t policy.Tier, title string) *mcp.ToolAnnotations
    Annotations derives MCP hints from a tier: Read -> ReadOnly+Idempotent;
    Write -> Destructive=false; Danger -> Destructive=true. OpenWorld is always
    true.

func ToolError(err error) error
    ToolError wraps err so that its Error() string is the JSON error body;
    the SDK puts that string into the IsError result content.


TYPES

type Handler[In, Out any] func(ctx context.Context, in In) (Out, error)
    Handler is a plain typed handler; mcpx adapts it to mcp.ToolHandlerFor.

```

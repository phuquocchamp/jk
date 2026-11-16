// Package mcpserver exposes core.Catalog as MCP tools over stdio.
package mcpserver

import (
	"context"
	"io"
	"os"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/mcpx"
	"github.com/phuquocchamp/jk/internal/kit/output"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

// New builds an MCP server registering every catalog op whose tier <= tier.
// Ops above the tier are not registered at all.
func New(svc *core.Service, tier policy.Tier, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "jk", Version: version}, nil)
	// Keep waits below common MCP client request timeouts; a capped wait
	// returns timed_out with a build_wait call to continue.
	svc.SetMaxWait(core.DefaultMCPMaxWait)
	for _, op := range core.Catalog {
		if op.Tier > tier {
			continue
		}
		register(s, svc, op)
	}
	s.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "spill",
		Title:       "Large tool result",
		URITemplate: output.AppName + "://spill/{name}",
		MIMEType:    "text/plain",
		Description: "Full text of a log, search result or script that was too large to inline; tools return its URI as spilled_uri.",
	}, readSpill)
	return s
}

// maxSpillRead bounds a spill resource read.
const maxSpillRead = 16 << 20

// readSpill serves a spill file written by this process (see output.SpillPath).
func readSpill(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	path, ok := output.SpillPath(uri)
	if !ok {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxSpillRead))
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "text/plain", Text: string(b)}}}, nil
}

// Run serves s over stdio until ctx is cancelled or the client disconnects.
func Run(ctx context.Context, s *mcp.Server) error {
	return s.Run(ctx, &mcp.StdioTransport{})
}

// refSchema lets "ref" be a JSON string ("last", "42") or integer (42):
// agents naturally pass the number they read from build_get.
var refSchema = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[core.BuildRef](): {Types: []string{"string", "integer"}},
}

// register binds one op to its Service method. Input schemas come from the
// core In types: write ops carry an optional dry_run, danger ops an optional
// confirm (default false), and Service enforces NEEDS_CONFIRMATION.
func register(s *mcp.Server, svc *core.Service, op core.Op) {
	name, desc, tier := op.MCPName(), op.Summary, op.Tier
	switch op.Name {
	case core.OpWhoami:
		mcpx.AddTool(s, name, desc, tier, svc.Whoami)
	case core.OpJobList:
		mcpx.AddTool(s, name, desc, tier, svc.JobList)
	case core.OpJobGet:
		mcpx.AddTool(s, name, desc, tier, svc.JobGet)
	case core.OpBuildGet:
		mcpx.AddTool(s, name, desc, tier, svc.BuildGet, refSchema)
	case core.OpBuildDiag:
		mcpx.AddTool(s, name, desc, tier, svc.BuildDiagnose, refSchema)
	case core.OpBuildStages:
		mcpx.AddTool(s, name, desc, tier, svc.BuildStages, refSchema)
	case core.OpBuildLog:
		mcpx.AddTool(s, name, desc, tier, svc.BuildLog, refSchema)
	case core.OpLogSearch:
		mcpx.AddTool(s, name, desc, tier, svc.LogSearch, refSchema)
	case core.OpTestFailed:
		mcpx.AddTool(s, name, desc, tier, svc.TestFailed, refSchema)
	case core.OpQueueGet:
		mcpx.AddTool(s, name, desc, tier, svc.QueueGet)
	case core.OpBuildWait:
		mcpx.AddTool(s, name, desc, tier, svc.BuildWait, refSchema)
	case core.OpReplayScript:
		mcpx.AddTool(s, name, desc, tier, svc.BuildReplayScript, refSchema)
	case core.OpBuildTrigger:
		mcpx.AddTool(s, name, desc, tier, svc.BuildTrigger)
	case core.OpBuildReplay:
		mcpx.AddTool(s, name, desc, tier, svc.BuildReplay, refSchema)
	case core.OpBuildStop:
		mcpx.AddTool(s, name, desc, tier, svc.BuildStop, refSchema)
	default:
		panic("mcpserver: unregistered catalog op " + op.Name)
	}
}

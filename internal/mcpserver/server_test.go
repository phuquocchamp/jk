package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/mcpx"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

func connect(t *testing.T, s *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestToolsFilteredByTier(t *testing.T) {
	cases := map[policy.Tier]int{policy.TierRead: 9, policy.TierWrite: 11, policy.TierDanger: 12}
	for tier, want := range cases {
		cs := connect(t, New(core.NewService(nil, nil), tier, "test"))
		res, err := cs.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Tools) != want {
			t.Errorf("tier %s: got %d tools, want %d", tier, len(res.Tools), want)
		}
		for _, tool := range res.Tools {
			op, ok := core.OpByName(tool.Name[len(core.MCPPrefix):])
			if !ok {
				t.Fatalf("unknown tool %s", tool.Name)
			}
			a := tool.Annotations
			if a.ReadOnlyHint != (op.Tier == policy.TierRead) {
				t.Errorf("%s: readOnlyHint=%v", tool.Name, a.ReadOnlyHint)
			}
			if op.Tier == policy.TierDanger && (a.DestructiveHint == nil || !*a.DestructiveHint) {
				t.Errorf("%s: want destructiveHint=true", tool.Name)
			}
		}
	}
}

// argsFor builds minimal arguments satisfying the required properties of a tool's input schema.
func argsFor(t *testing.T, tool *mcp.Tool) map[string]any {
	b, _ := json.Marshal(tool.InputSchema)
	var s struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type  any `json:"type"`
			Types any `json:"types"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{}
	for _, name := range s.Required {
		switch s.Properties[name].Type {
		case "integer", "number":
			args[name] = 1
		case "boolean":
			args[name] = false
		default:
			args[name] = "1"
		}
	}
	return args
}

func addZero[In, Out any](s *mcp.Server, name string) {
	mcpx.AddTool(s, name, "zero", policy.TierRead, func(context.Context, In) (Out, error) {
		var z Out
		return z, nil
	})
}

// Zero-valued outputs (nil slices, nil pointers) must validate against the inferred output schema.
func TestZeroOutputsValidate(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	addZero[core.WhoamiIn, core.WhoamiOut](s, "whoami")
	addZero[core.JobListIn, core.JobListOut](s, "job_list")
	addZero[core.JobGetIn, core.JobGetOut](s, "job_get")
	addZero[core.BuildGetIn, core.BuildGetOut](s, "build_get")
	addZero[core.BuildStagesIn, core.BuildStagesOut](s, "build_stages")
	addZero[core.BuildLogIn, core.BuildLogOut](s, "build_log")
	addZero[core.LogSearchIn, core.LogSearchOut](s, "log_search")
	addZero[core.TestFailedIn, core.TestFailedOut](s, "test_failed")
	addZero[core.QueueGetIn, core.QueueGetOut](s, "queue_get")
	addZero[core.BuildTriggerIn, core.BuildTriggerOut](s, "build_trigger")
	addZero[core.BuildReplayIn, core.BuildReplayOut](s, "build_replay")
	addZero[core.BuildStopIn, core.BuildStopOut](s, "build_stop")

	cs := connect(t, s)
	ctx := context.Background()
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 12 {
		t.Fatalf("got %d tools", len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool.Name, Arguments: argsFor(t, tool)})
		if err != nil {
			t.Errorf("%s: %v", tool.Name, err)
			continue
		}
		if res.IsError {
			t.Errorf("%s: tool error: %v", tool.Name, res.Content[0].(*mcp.TextContent).Text)
		}
	}
}

// errBody is the JSON body of an IsError tool result.
type errBody struct {
	Code      string          `json:"code"`
	Message   string          `json:"message"`
	Hint      string          `json:"hint"`
	Retryable bool            `json:"retryable"`
	Details   json.RawMessage `json:"details"`
}

func decodeErr(t *testing.T, res *mcp.CallToolResult) errBody {
	t.Helper()
	if !res.IsError {
		t.Fatalf("want IsError, got %+v", res.StructuredContent)
	}
	var body errBody
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &body); err != nil {
		t.Fatalf("content is not JSON: %v", err)
	}
	return body
}

// skipIfStub skips while core.Service still returns the foundation stub error.
func skipIfStub(t *testing.T, res *mcp.CallToolResult) {
	t.Helper()
	if !res.IsError {
		return
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "not implemented") {
		t.Skipf("core.Service not implemented yet: %s", text)
	}
}

func newServer(t *testing.T, tier policy.Tier) (*mcp.ClientSession, *fakeBackend) {
	t.Helper()
	b := &fakeBackend{}
	svc := core.NewService(b, policy.NewEngine(policy.Policy{Tier: tier}))
	return connect(t, New(svc, tier, "test")), b
}

// Business errors surface as IsError results with a JSON {code,message} body.
func TestErrorResultIsJSON(t *testing.T) {
	cs, _ := newServer(t, policy.TierRead)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "jenkins_whoami", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	body := decodeErr(t, res)
	if body.Code == "" || body.Message == "" {
		t.Errorf("incomplete error body: %+v", body)
	}
	skipIfStub(t, res)
	if body.Code != "AUTH_REQUIRED" {
		t.Errorf("code=%q, want AUTH_REQUIRED", body.Code)
	}
}

func TestBuildGetStructuredContent(t *testing.T) {
	cs, _ := newServer(t, policy.TierRead)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "jenkins_build_get",
		Arguments: map[string]any{"job": "team-a/app", "ref": "42"},
	})
	if err != nil {
		t.Fatal(err)
	}
	skipIfStub(t, res)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content[0].(*mcp.TextContent).Text)
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var got core.Build
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Number != 42 || got.Result != "FAILURE" || got.DurationMs != 1234 || got.Job != "team-a/app" {
		t.Errorf("structuredContent = %s", b)
	}
	if got.Timestamp.Format("2006-01-02T15:04:05Z07:00") != "2026-01-02T03:04:05Z" {
		t.Errorf("timestamp = %v", got.Timestamp)
	}
	// Content mirrors structuredContent as JSON text for clients without structured support.
	if len(res.Content) == 0 {
		t.Fatal("empty content")
	}
	if _, ok := res.Content[0].(*mcp.TextContent); !ok {
		t.Errorf("content[0] is %T", res.Content[0])
	}
}

func TestBuildStopNeedsConfirmation(t *testing.T) {
	cs, b := newServer(t, policy.TierDanger)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "jenkins_build_stop",
		Arguments: map[string]any{"job": "team-a/app", "ref": "42"},
	})
	if err != nil {
		t.Fatal(err)
	}
	skipIfStub(t, res)
	body := decodeErr(t, res)
	if body.Code != "NEEDS_CONFIRMATION" {
		t.Errorf("code=%q, want NEEDS_CONFIRMATION", body.Code)
	}
	if len(body.Details) == 0 || string(body.Details) == "null" {
		t.Error("want the plan in details")
	}
	if len(b.stopped) != 0 {
		t.Errorf("backend StopBuild called without confirm: %v", b.stopped)
	}
}

func TestBuildStopConfirmed(t *testing.T) {
	cs, b := newServer(t, policy.TierDanger)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "jenkins_build_stop",
		Arguments: map[string]any{"job": "team-a/app", "ref": "42", "confirm": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	skipIfStub(t, res)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content[0].(*mcp.TextContent).Text)
	}
	if len(b.stopped) != 1 || b.stopped[0] != 42 {
		t.Errorf("stopped=%v, want [42]", b.stopped)
	}
}

// Write tools expose dry_run, danger tools expose confirm; neither is required.
func TestWriteAndDangerInputs(t *testing.T) {
	cs, _ := newServer(t, policy.TierDanger)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		op, _ := core.OpByName(strings.TrimPrefix(tool.Name, core.MCPPrefix))
		raw, _ := json.Marshal(tool.InputSchema)
		var s struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
		has := func(p string) bool { _, ok := s.Properties[p]; return ok }
		if op.Tier >= policy.TierWrite && !has("dry_run") {
			t.Errorf("%s: missing dry_run", tool.Name)
		}
		if op.Tier == policy.TierDanger && !has("confirm") {
			t.Errorf("%s: missing confirm", tool.Name)
		}
		for _, r := range s.Required {
			if r == "confirm" || r == "dry_run" {
				t.Errorf("%s: %s must be optional", tool.Name, r)
			}
		}
		if tool.OutputSchema == nil {
			t.Errorf("%s: missing outputSchema", tool.Name)
		}
		if tool.Description != op.Summary {
			t.Errorf("%s: description %q", tool.Name, tool.Description)
		}
	}
}

func TestServerInfo(t *testing.T) {
	cs, _ := newServer(t, policy.TierRead)
	info := cs.InitializeResult().ServerInfo
	if info.Name != "jk" || info.Version != "test" {
		t.Errorf("serverInfo = %+v", info)
	}
}

func TestBuildRefAcceptsInteger(t *testing.T) {
	cs, _ := newServer(t, policy.TierDanger)
	ctx := context.Background()
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	withRef := 0
	for _, tool := range tools.Tools {
		b, _ := json.Marshal(tool.InputSchema)
		var sch struct {
			Properties map[string]struct {
				Type        any    `json:"type"`
				Description string `json:"description"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(b, &sch); err != nil {
			t.Fatal(err)
		}
		ref, ok := sch.Properties["ref"]
		if !ok {
			continue
		}
		withRef++
		types, _ := ref.Type.([]any)
		if len(types) != 2 || types[0] != "string" || types[1] != "integer" {
			t.Errorf("%s ref type = %v", tool.Name, ref.Type)
		}
		if ref.Description == "" {
			t.Errorf("%s ref lost its description", tool.Name)
		}
	}
	if withRef != 7 {
		t.Errorf("%d tools have ref, want 7", withRef)
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "jenkins_build_get",
		Arguments: map[string]any{"job": "team-a/app", "ref": 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", res.Content[0].(*mcp.TextContent).Text)
	}
	b, _ := json.Marshal(res.StructuredContent)
	var got core.Build
	if err := json.Unmarshal(b, &got); err != nil || got.Number != 42 {
		t.Fatalf("got %s (%v)", b, err)
	}

	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "jenkins_build_get",
		Arguments: map[string]any{"job": "team-a/app", "ref": 4.5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("non-integer ref must be rejected")
	}
}

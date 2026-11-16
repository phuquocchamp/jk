package mcpserver

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/output"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

// connectWith connects a client with the given options (capabilities, handlers).
func connectWith(t *testing.T, s *mcp.Server, opts *mcp.ClientOptions) *mcp.ClientSession {
	return connectVersion(t, s, opts, "")
}

// connectVersion connects with a fixed protocol version ("" = latest).
func connectVersion(t *testing.T, s *mcp.Server, opts *mcp.ClientOptions, version string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	var so *mcp.ClientSessionOptions
	if version != "" {
		so = &mcp.ClientSessionOptions{ProtocolVersion: version}
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, opts).Connect(ctx, ct, so)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// eliciting returns client options that answer every elicitation with action
// and confirm, recording the messages shown.
func eliciting(action string, confirm bool, shown *[]string) *mcp.ClientOptions {
	return &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			*shown = append(*shown, req.Params.Message)
			if action != "accept" {
				return &mcp.ElicitResult{Action: action}, nil
			}
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": confirm}}, nil
		},
	}
}

func stopCall(confirm bool) *mcp.CallToolParams {
	args := map[string]any{"job": "team-a/app", "ref": 42}
	if confirm {
		args["confirm"] = true
	}
	return &mcp.CallToolParams{Name: "jenkins_build_stop", Arguments: args}
}

// With elicitation the human decides: an agent's confirm=true is not enough.
func TestBuildStopAsksTheHuman(t *testing.T) {
	var shown []string
	b := &fakeBackend{}
	svc := core.NewService(b, policy.NewEngine(policy.Policy{Tier: policy.TierDanger}))
	cs := connectWith(t, New(svc, policy.TierDanger, "test"), eliciting("decline", false, &shown))

	res, err := cs.CallTool(context.Background(), stopCall(true))
	if err != nil {
		t.Fatal(err)
	}
	if body := decodeErr(t, res); body.Code != "POLICY_DENIED" || !strings.Contains(body.Message, "declined") {
		t.Fatalf("got %+v", body)
	}
	if len(b.stopped) != 0 {
		t.Fatalf("stopped after a decline: %v", b.stopped)
	}
	if len(shown) != 1 || !strings.Contains(shown[0], "Stop build #42 of team-a/app?") || !strings.Contains(shown[0], "/job/team-a/job/app/42/stop") {
		t.Fatalf("prompt %q", shown)
	}
}

func TestBuildStopApprovedByTheHuman(t *testing.T) {
	var shown []string
	b := &fakeBackend{}
	svc := core.NewService(b, policy.NewEngine(policy.Policy{Tier: policy.TierDanger}))
	cs := connectWith(t, New(svc, policy.TierDanger, "test"), eliciting("accept", true, &shown))

	// No confirm flag: the approval comes from the human.
	res, err := cs.CallTool(context.Background(), stopCall(false))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content[0].(*mcp.TextContent).Text)
	}
	if len(b.stopped) != 1 || b.stopped[0] != 42 {
		t.Fatalf("stopped=%v", b.stopped)
	}
}

// Older protocol versions get elicitation/create from the SDK instead.
func TestBuildStopAsksTheHumanLegacyProtocol(t *testing.T) {
	for _, tc := range []struct {
		action  string
		confirm bool
		stopped int
	}{{"decline", false, 0}, {"accept", true, 1}} {
		var shown []string
		b := &fakeBackend{}
		svc := core.NewService(b, policy.NewEngine(policy.Policy{Tier: policy.TierDanger}))
		cs := connectVersion(t, New(svc, policy.TierDanger, "test"), eliciting(tc.action, tc.confirm, &shown), "2025-11-25")
		res, err := cs.CallTool(context.Background(), stopCall(true))
		if err != nil {
			t.Fatal(err)
		}
		if len(shown) != 1 || len(b.stopped) != tc.stopped || res.IsError == (tc.stopped == 1) {
			t.Fatalf("%s: shown=%d stopped=%v isError=%v", tc.action, len(shown), b.stopped, res.IsError)
		}
	}
}

// Without elicitation support the confirm flag decides, as before.
func TestBuildStopWithoutElicitationUsesFlag(t *testing.T) {
	b := &fakeBackend{}
	svc := core.NewService(b, policy.NewEngine(policy.Policy{Tier: policy.TierDanger}))
	cs := connectWith(t, New(svc, policy.TierDanger, "test"), nil)
	res, err := cs.CallTool(context.Background(), stopCall(false))
	if err != nil {
		t.Fatal(err)
	}
	if body := decodeErr(t, res); body.Code != "NEEDS_CONFIRMATION" || len(b.stopped) != 0 {
		t.Fatalf("got %+v", body)
	}
}

func TestBuildStopAcceptWithoutConfirmIsNotApproval(t *testing.T) {
	var shown []string
	b := &fakeBackend{}
	svc := core.NewService(b, policy.NewEngine(policy.Policy{Tier: policy.TierDanger}))
	cs := connectWith(t, New(svc, policy.TierDanger, "test"), eliciting("accept", false, &shown))
	res, err := cs.CallTool(context.Background(), stopCall(true))
	if err != nil {
		t.Fatal(err)
	}
	if body := decodeErr(t, res); body.Code != "POLICY_DENIED" || len(b.stopped) != 0 {
		t.Fatalf("got %+v stopped=%v", body, b.stopped)
	}
}

func TestBuildReplayPromptShowsDiff(t *testing.T) {
	var shown []string
	b := &fakeBackend{}
	svc := core.NewService(b, policy.NewEngine(policy.Policy{Tier: policy.TierDanger}))
	cs := connectWith(t, New(svc, policy.TierDanger, "test"), eliciting("decline", false, &shown))
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "jenkins_build_replay",
		Arguments: map[string]any{"job": "team-a/app", "ref": 42, "script": "pipeline { agent any }\n"}})
	if err != nil {
		t.Fatal(err)
	}
	decodeErr(t, res)
	if len(shown) != 1 || !strings.Contains(shown[0], "-pipeline {}") || !strings.Contains(shown[0], "+pipeline { agent any }") {
		t.Fatalf("prompt %q", shown)
	}
}

// fakeClock advances on every sleep so waits finish instantly.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
	return nil
}

func TestBuildWaitProgressAndCap(t *testing.T) {
	var (
		mu   sync.Mutex
		msgs []string
		last float64
	)
	got := make(chan struct{}, 100)
	opts := &mcp.ClientOptions{ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
		mu.Lock()
		defer mu.Unlock()
		if req.Params.Progress <= last && len(msgs) > 0 {
			t.Errorf("progress not increasing: %v after %v", req.Params.Progress, last)
		}
		last = req.Params.Progress
		msgs = append(msgs, req.Params.Message)
		got <- struct{}{}
	}}
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	// The fake queue item never starts a build, so the wait runs into the cap.
	svc := core.NewService(&fakeBackend{}, policy.NewEngine(policy.Policy{}), core.WithClock(clk.Now), core.WithSleep(clk.Sleep))
	cs := connectWith(t, New(svc, policy.TierRead, "test"), opts)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "jenkins_build_wait",
		Arguments: map[string]any{"job": "team-a/app", "queue_id": 7, "timeout_seconds": 600},
		Meta:      mcp.Meta{"progressToken": "w1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	sc, _ := res.StructuredContent.(map[string]any)
	if res.IsError || sc["timed_out"] != true {
		t.Fatalf("got %+v", res.StructuredContent)
	}
	next, _ := sc["next"].(map[string]any)
	if next["tool"] != "jenkins_build_wait" {
		t.Fatalf("next %+v", next)
	}
	if waited := clk.Now().Sub(time.Unix(1_700_000_000, 0)); waited > core.DefaultMCPMaxWait+5*time.Second {
		t.Fatalf("waited %s, want the MCP cap", waited)
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("no progress notification")
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.HasPrefix(msgs[0], "queue item 7:") {
		t.Fatalf("messages %q", msgs)
	}
}

func TestSpillResource(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	cs, _ := newServer(t, policy.TierRead)
	ctx := context.Background()

	path, err := output.Spill("line 1\nline 2\n", "build_log-3")
	if err != nil {
		t.Fatal(err)
	}
	uri := output.SpillURI(path)
	if !strings.HasPrefix(uri, "jk://spill/build_log-3-") {
		t.Fatalf("uri %q", uri)
	}
	res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 || res.Contents[0].Text != "line 1\nline 2\n" {
		t.Fatalf("contents %+v", res.Contents)
	}

	// Only files this process spilled are served.
	for _, bad := range []string{"jk://spill/unknown.txt", "jk://spill/../etc/passwd", "file:///etc/passwd"} {
		if _, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: bad}); err == nil {
			t.Errorf("%s: served", bad)
		}
	}
}

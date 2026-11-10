package mcpx

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

func TestAnnotations(t *testing.T) {
	r := Annotations(policy.TierRead, "")
	if !r.ReadOnlyHint || !r.IdempotentHint || r.DestructiveHint != nil || r.OpenWorldHint == nil || !*r.OpenWorldHint {
		t.Errorf("read: %+v", r)
	}
	w := Annotations(policy.TierWrite, "")
	if w.ReadOnlyHint || w.IdempotentHint || w.DestructiveHint == nil || *w.DestructiveHint {
		t.Errorf("write: %+v", w)
	}
	d := Annotations(policy.TierDanger, "Stop")
	if d.ReadOnlyHint || d.DestructiveHint == nil || !*d.DestructiveHint || d.Title != "Stop" {
		t.Errorf("danger: %+v", d)
	}
}

func TestToolErrorJSON(t *testing.T) {
	e := errs.New(errs.CodeNeedsConfirmation, "confirm required").WithHint("pass confirm=true")
	e.Details = map[string]string{"path": "/job/x/1/stop"}
	var body map[string]any
	if err := json.Unmarshal([]byte(ToolError(e).Error()), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "NEEDS_CONFIRMATION" || body["hint"] != "pass confirm=true" || body["details"] == nil {
		t.Errorf("body = %v", body)
	}
	if !errors.Is(ToolError(e), e) {
		t.Error("ToolError must unwrap to the *errs.Error")
	}

	var plain map[string]any
	_ = json.Unmarshal([]byte(ToolError(errors.New("boom")).Error()), &plain)
	if plain["code"] != "INTERNAL" || plain["message"] != "boom" {
		t.Errorf("plain error body = %v", plain)
	}
}

type in struct {
	Name    string `json:"name"`
	Confirm bool   `json:"confirm,omitempty"`
}

type out struct {
	Name string    `json:"name"`
	At   time.Time `json:"at"`
}

func TestAddToolRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	AddTool(s, "echo", "echo the name", policy.TierDanger, func(_ context.Context, i in) (out, error) {
		if !i.Confirm {
			return out{}, errs.New(errs.CodeNeedsConfirmation, "confirm required")
		}
		return out{Name: i.Name, At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, nil
	})
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tool := tools.Tools[0]
	if tool.OutputSchema == nil || tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
		t.Errorf("tool = %+v", tool)
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"name": "jane"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("want IsError without confirm")
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &body); err != nil || body["code"] != "NEEDS_CONFIRMATION" {
		t.Errorf("error body = %v (%v)", body, err)
	}

	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"name": "jane", "confirm": true}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %v", res.Content)
	}
	sc, _ := res.StructuredContent.(map[string]any)
	if sc["name"] != "jane" || sc["at"] != "2026-01-02T03:04:05Z" {
		t.Errorf("structuredContent = %v", res.StructuredContent)
	}
}

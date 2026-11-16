package mcpx

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/phuquocchamp/jk/internal/kit/interact"
)

func answered(state string, res *mcp.ElicitResult) *elicitConfirmer {
	var asked string
	return &elicitConfirmer{asked: &asked, req: &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		InputResponses: mcp.InputResponseMap{confirmKey: res},
		RequestState:   state,
	}}}
}

func TestElicitConfirmer(t *testing.T) {
	ctx := context.Background()
	const q = "Stop build #42?"
	yes := &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}

	// First call: no answer yet, the question is queued.
	c := answered("", nil)
	c.req.Params.InputResponses = nil
	if ok, err := c.Confirm(ctx, q); ok || !errors.Is(err, interact.ErrPending) || *c.asked != q {
		t.Fatalf("first call: %v %v %q", ok, err, *c.asked)
	}
	// Retry with the answer to the same question.
	if ok, err := answered(digest(q), yes).Confirm(ctx, q); !ok || err != nil {
		t.Fatalf("approved: %v %v", ok, err)
	}
	// An answer to another plan (e.g. "last" moved to a newer build) is not approval: ask again.
	c = answered(digest("Stop build #41?"), yes)
	if ok, err := c.Confirm(ctx, q); ok || !errors.Is(err, interact.ErrPending) {
		t.Fatalf("stale answer: %v %v", ok, err)
	}
	for _, res := range []*mcp.ElicitResult{
		{Action: "decline"}, {Action: "cancel"}, {Action: "accept", Content: map[string]any{"confirm": false}},
	} {
		if ok, err := answered(digest(q), res).Confirm(ctx, q); ok || err != nil {
			t.Fatalf("%+v approved (%v)", res, err)
		}
	}
}

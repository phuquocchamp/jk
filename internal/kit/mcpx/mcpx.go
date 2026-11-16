// Package mcpx registers typed MCP tools with annotations derived from a policy tier
// and turns structured errors into IsError tool results.
package mcpx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/interact"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

// Handler is a plain typed handler; mcpx adapts it to mcp.ToolHandlerFor.
type Handler[In, Out any] func(ctx context.Context, in In) (Out, error)

// Annotations derives MCP hints from a tier:
// Read -> ReadOnly+Idempotent; Write -> Destructive=false; Danger -> Destructive=true.
// OpenWorld is always true.
func Annotations(t policy.Tier, title string) *mcp.ToolAnnotations {
	a := &mcp.ToolAnnotations{Title: title, OpenWorldHint: ptr(true)}
	switch t {
	case policy.TierRead:
		a.ReadOnlyHint = true
		a.IdempotentHint = true
	case policy.TierWrite:
		a.DestructiveHint = ptr(false)
	default:
		a.DestructiveHint = ptr(true)
	}
	return a
}

// AddTool registers name on s with schemas inferred from In/Out. Business errors
// from h become a CallToolResult with IsError=true whose text content is
// {"code","message","hint","retryable","details"}.
//
// typeSchemas optionally overrides the inferred schema of specific Go types in
// the input schema (e.g. a string type that also accepts integers).
func AddTool[In, Out any](s *mcp.Server, name, description string, tier policy.Tier, h Handler[In, Out], typeSchemas ...map[reflect.Type]*jsonschema.Schema) {
	tool := &mcp.Tool{Name: name, Description: description, Annotations: Annotations(tier, "")}
	if len(typeSchemas) > 0 {
		merged := map[reflect.Type]*jsonschema.Schema{}
		for _, m := range typeSchemas {
			for k, v := range m {
				merged[k] = v
			}
		}
		in, err := jsonschema.For[In](&jsonschema.ForOptions{TypeSchemas: merged})
		if err != nil {
			panic("mcpx: input schema for " + name + ": " + err.Error())
		}
		tool.InputSchema = in
	}
	mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		var asked string
		out, err := h(WithInteraction(ctx, req, &asked), in)
		if err != nil {
			var zero Out
			if errors.Is(err, interact.ErrPending) && asked != "" {
				// Ask the user (multi round-trip request, SEP-2322); the client
				// calls again with the answer in InputResponses. With older
				// protocol versions the SDK sends elicitation/create itself.
				return &mcp.CallToolResult{
					InputRequests: mcp.InputRequestMap{confirmKey: &mcp.ElicitParams{Message: asked, RequestedSchema: confirmSchema}},
					RequestState:  digest(asked),
				}, zero, nil
			}
			return nil, zero, ToolError(err)
		}
		return nil, out, nil
	})
}

// WithInteraction installs, for one tool call, a progress reporter (when the
// client sent a progress token) and a confirmer (when the client supports
// elicitation), so handlers can reach the human through kit/interact. The
// question a confirmer queues is stored in *asked.
func WithInteraction(ctx context.Context, req *mcp.CallToolRequest, asked *string) context.Context {
	if req == nil || req.Session == nil || req.Params == nil {
		return ctx
	}
	if tok := req.Params.GetProgressToken(); tok != nil {
		ctx = interact.WithProgress(ctx, &progress{ss: req.Session, token: tok})
	}
	if caps := req.ClientCapabilities(); caps != nil && caps.Elicitation != nil {
		ctx = interact.WithConfirmer(ctx, &elicitConfirmer{req: req, asked: asked})
	}
	return ctx
}

// progress sends notifications/progress; values must increase, so smaller or
// equal ones are dropped.
type progress struct {
	ss    *mcp.ServerSession
	token any
	mu    sync.Mutex
	last  float64
	sent  bool
}

func (p *progress) Report(ctx context.Context, done, total float64, message string) {
	p.mu.Lock()
	if p.sent && done <= p.last {
		p.mu.Unlock()
		return
	}
	p.last, p.sent = done, true
	p.mu.Unlock()
	_ = p.ss.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
		ProgressToken: p.token, Progress: done, Total: total, Message: message,
	})
}

// confirmKey names the confirmation in InputRequests/InputResponses.
const confirmKey = "confirm"

// confirmSchema asks for one boolean; clients render it as a checkbox or
// yes/no next to the message.
var confirmSchema = &jsonschema.Schema{
	Type: "object",
	Properties: map[string]*jsonschema.Schema{
		"confirm": {Type: "boolean", Title: "Approve", Description: "Approve this action"},
	},
	Required: []string{"confirm"},
}

// digest ties an answer to the question it answers: the request state is the
// hash of the prompt, so an answer to a different plan (e.g. "last" now names
// a newer build) is not taken as approval.
func digest(msg string) string {
	sum := sha256.Sum256([]byte(msg))
	return hex.EncodeToString(sum[:])
}

// elicitConfirmer answers from the user's response when this call carries one
// for the same question; otherwise it queues the question (ErrPending). Only an
// explicit accept with confirm=true approves.
type elicitConfirmer struct {
	req   *mcp.CallToolRequest
	asked *string
}

func (c *elicitConfirmer) Confirm(_ context.Context, message string) (bool, error) {
	p := c.req.Params
	if r, ok := p.InputResponses[confirmKey]; ok && p.RequestState == digest(message) {
		res, _ := r.(*mcp.ElicitResult)
		if res == nil {
			return false, nil
		}
		yes, _ := res.Content["confirm"].(bool)
		return res.Action == "accept" && yes, nil
	}
	*c.asked = message
	return false, interact.ErrPending
}

// ToolError wraps err so that its Error() string is the JSON error body; the SDK
// puts that string into the IsError result content.
func ToolError(err error) error {
	return &toolError{e: errs.From(err)}
}

type toolError struct{ e *errs.Error }

func (t *toolError) Error() string {
	b, err := json.Marshal(t.e)
	if err != nil {
		return t.e.Error()
	}
	return string(b)
}

func (t *toolError) Unwrap() error { return t.e }

func ptr[T any](v T) *T { return &v }

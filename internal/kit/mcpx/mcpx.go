// Package mcpx registers typed MCP tools with annotations derived from a policy tier
// and turns structured errors into IsError tool results.
package mcpx

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/phuquocchamp/jk/internal/kit/errs"
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
	mcp.AddTool(s, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		out, err := h(ctx, in)
		if err != nil {
			var zero Out
			return nil, zero, ToolError(err)
		}
		return nil, out, nil
	})
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

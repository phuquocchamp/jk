// Package interact lets an operation talk to the human behind the caller
// without knowing the transport: ask for confirmation and report progress.
// The MCP layer installs implementations in the context (elicitation and
// progress notifications); without them both are absent and callers fall
// back to their non-interactive behaviour. It is product-agnostic.
package interact

import (
	"context"
	"errors"
)

// ErrPending is returned by Confirm when the question has been queued for
// the human and the answer arrives with a later call of the same operation
// (MCP multi round-trip requests). Callers must return it unchanged so the
// transport can ask and re-run the operation with the answer.
var ErrPending = errors.New("waiting for the user's answer")

// Confirmer asks a human to approve an action.
type Confirmer interface {
	// Confirm shows message to the human and reports whether they approved.
	// ErrPending means the answer comes with a later call; any other error
	// means no answer could be obtained.
	Confirm(ctx context.Context, message string) (bool, error)
}

// Progress receives progress of a long-running operation.
type Progress interface {
	// Report sends done out of total (total 0 = unknown) with a short message.
	Report(ctx context.Context, done, total float64, message string)
}

type confirmerKey struct{}
type progressKey struct{}

// WithConfirmer returns ctx carrying c.
func WithConfirmer(ctx context.Context, c Confirmer) context.Context {
	return context.WithValue(ctx, confirmerKey{}, c)
}

// ConfirmerFrom returns the Confirmer in ctx, if any.
func ConfirmerFrom(ctx context.Context) (Confirmer, bool) {
	c, ok := ctx.Value(confirmerKey{}).(Confirmer)
	return c, ok && c != nil
}

// WithProgress returns ctx carrying p.
func WithProgress(ctx context.Context, p Progress) context.Context {
	return context.WithValue(ctx, progressKey{}, p)
}

// Report forwards progress to the Progress in ctx; without one it does nothing.
func Report(ctx context.Context, done, total float64, message string) {
	if p, ok := ctx.Value(progressKey{}).(Progress); ok && p != nil {
		p.Report(ctx, done, total, message)
	}
}

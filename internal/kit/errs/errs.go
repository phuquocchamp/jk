// Package errs defines the structured error model shared by CLI and MCP surfaces.
//
// Every error that crosses a package boundary should be an *Error so that the
// CLI can map it to an exit code and the MCP server can return {code,message,hint}.
package errs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// Code is a stable, machine-readable error category.
type Code string

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

// ExitCode maps a code to the process exit status used by the CLI.
func ExitCode(c Code) int {
	switch c {
	case CodeUsage:
		return 2
	case CodeNotFound:
		return 3
	case CodePolicyDenied, CodeForbidden:
		return 4
	case CodeConflict:
		return 5
	case CodeAuthRequired:
		return 6
	case CodeNeedsConfirmation:
		return 7
	default:
		return 1
	}
}

// Error is the structured error. Err is the wrapped cause and is never serialized.
type Error struct {
	Code      Code   `json:"code"`
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	Retryable bool   `json:"retryable"`
	Details   any    `json:"details,omitempty"`
	Err       error  `json:"-"`
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// WithHint sets the hint and returns e for chaining.
func (e *Error) WithHint(hint string) *Error { e.Hint = hint; return e }

// WithDetails sets the details and returns e for chaining.
func (e *Error) WithDetails(d any) *Error { e.Details = d; return e }

// WithRetryable sets the retryable flag and returns e for chaining.
func (e *Error) WithRetryable(r bool) *Error { e.Retryable = r; return e }

// New creates an error with the given code and message.
func New(code Code, msg string) *Error { return &Error{Code: code, Message: msg} }

// Newf creates an error with a formatted message.
func Newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap creates an error with the given code that wraps cause.
func Wrap(code Code, cause error, msg string) *Error {
	return &Error{Code: code, Message: msg, Err: cause}
}

// Shorthand constructors for the common cases.
func Usage(msg string) *Error { return New(CodeUsage, msg) }
func Usagef(format string, args ...any) *Error {
	return Newf(CodeUsage, format, args...)
}
func NotFound(msg string) *Error { return New(CodeNotFound, msg) }
func Internal(msg string) *Error { return New(CodeInternal, msg) }
func PolicyDenied(msg string) *Error {
	return New(CodePolicyDenied, msg)
}

// NotImplemented is used by stubs.
func NotImplemented(what string) *Error {
	return New(CodeInternal, what+": not implemented")
}

// As returns the *Error in err's chain, if any.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Is reports whether err carries the given code.
func Is(err error, code Code) bool {
	e, ok := As(err)
	return ok && e.Code == code
}

// From converts any error into an *Error. Context deadline errors become a
// retryable UPSTREAM error; context cancellation and unknown errors become INTERNAL.
// From(nil) returns nil.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := As(err); ok {
		return e
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: CodeUpstream, Message: "request timed out", Retryable: true, Err: err}
	}
	return &Error{Code: CodeInternal, Message: err.Error(), Err: err}
}

// ExitCodeOf returns the CLI exit code for err (0 for nil).
func ExitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	return ExitCode(From(err).Code)
}

// FromHTTPStatus maps an HTTP status to an error. It returns nil for 2xx/3xx.
// 401 -> AUTH_REQUIRED, 403 -> FORBIDDEN, 404 -> NOT_FOUND, 409 -> CONFLICT,
// 5xx -> UPSTREAM (retryable), other 4xx -> UPSTREAM (not retryable).
func FromHTTPStatus(status int, msg string) *Error {
	if status < 400 {
		return nil
	}
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d %s", status, http.StatusText(status))
	}
	switch {
	case status == http.StatusUnauthorized:
		return New(CodeAuthRequired, msg).WithHint("check the user and API token (auth login)")
	case status == http.StatusForbidden:
		return New(CodeForbidden, msg)
	case status == http.StatusNotFound:
		return New(CodeNotFound, msg)
	case status == http.StatusConflict:
		return New(CodeConflict, msg)
	case status >= 500:
		return New(CodeUpstream, msg).WithRetryable(true)
	default:
		return New(CodeUpstream, msg)
	}
}

// Envelope is the JSON shape written to stderr in JSON mode: {"error": {...}}.
type Envelope struct {
	Error *Error `json:"error"`
}

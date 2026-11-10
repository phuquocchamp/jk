package core

import (
	"context"
	"time"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/output"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

// Defaults used by Service.
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
	// maxLineBytes caps a single log line; longer lines are cut.
	maxLineBytes = 1 << 20
)

// Service implements every catalog op. It owns input normalization
// (ParseJobInput, ParseBuildRef, defaults), policy checks (tier + job globs via
// policy.Engine.Check with the op's Catalog Tier/TargetKind), dry-run plans,
// confirmation for danger ops, limits/trimming, and log streaming logic.
type Service struct {
	backend      Backend
	policy       *policy.Engine
	now          func() time.Time
	sleep        func(ctx context.Context, d time.Duration) error
	pollInterval time.Duration
	spill        func(content, prefix string) (string, error)
}

// Option configures a Service.
type Option func(*Service)

// WithClock overrides time.Now (tests).
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// WithSleep overrides the poll sleep used by build_trigger --wait (tests).
func WithSleep(f func(ctx context.Context, d time.Duration) error) Option {
	return func(s *Service) { s.sleep = f }
}

// WithPollInterval overrides DefaultPollInterval.
func WithPollInterval(d time.Duration) Option { return func(s *Service) { s.pollInterval = d } }

// WithSpill overrides output.Spill (tests).
func WithSpill(f func(content, prefix string) (string, error)) Option {
	return func(s *Service) { s.spill = f }
}

// NewService builds a Service. A nil engine means policy.NewEngine(policy.Policy{}) (read tier).
func NewService(b Backend, p *policy.Engine, opts ...Option) *Service {
	if p == nil {
		p = policy.NewEngine(policy.Policy{})
	}
	s := &Service{backend: b, policy: p, now: time.Now, sleep: sleepCtx, pollInterval: DefaultPollInterval, spill: output.Spill}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Policy returns the policy engine (used by mcpserver/cli for tier filtering).
func (s *Service) Policy() *policy.Engine { return s.policy }

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// checkOp enforces the op's tier (from Catalog) without a target.
func (s *Service) checkOp(name string) (Op, error) {
	op, ok := OpByName(name)
	if !ok {
		return Op{}, errs.Internal("unknown op " + name)
	}
	if err := s.policy.Check(op.Tier, TargetNone, ""); err != nil {
		return op, err
	}
	return op, nil
}

// target runs the shared prologue of job ops: tier check, job parsing (USAGE),
// allow/deny check on the parsed path. It returns the job path and the build
// ref embedded in a pasted URL ("" when none).
func (s *Service) target(name, job string) (string, BuildRef, error) {
	op, err := s.checkOp(name)
	if err != nil {
		return "", "", err
	}
	path, urlRef, err := ParseJobInput(job)
	if err != nil {
		return "", "", err
	}
	if err := s.policy.Check(op.Tier, op.TargetKind, path); err != nil {
		return "", "", err
	}
	return path, urlRef, nil
}

// pickRef returns the normalized ref: explicit ref wins, then the URL ref,
// then last. With required=true a missing ref is a USAGE error.
func pickRef(explicit, fromURL BuildRef, required bool) (BuildRef, error) {
	raw := string(explicit)
	if raw == "" {
		raw = string(fromURL)
	}
	if raw == "" {
		if required {
			return "", errs.Usage("build ref is required").
				WithHint("pass a build number or last, lastFailed, lastSuccessful, lastCompleted")
		}
		return RefLast, nil
	}
	return ParseBuildRef(raw)
}

// prepare parses the job and picks the ref without calling the backend.
func (s *Service) prepare(name, job string, ref BuildRef, required bool) (string, BuildRef, error) {
	path, urlRef, err := s.target(name, job)
	if err != nil {
		return "", "", err
	}
	r, err := pickRef(ref, urlRef, required)
	if err != nil {
		return "", "", err
	}
	return path, r, nil
}

// number resolves a ref to a build number (backend call for permalinks).
func (s *Service) number(ctx context.Context, path string, r BuildRef) (int, error) {
	if n, ok := r.Number(); ok {
		return n, nil
	}
	return s.backend.ResolveBuild(ctx, path, r)
}

func nonNegative(name string, v int) error {
	if v < 0 {
		return errs.Usagef("%s must not be negative (got %d)", name, v)
	}
	return nil
}

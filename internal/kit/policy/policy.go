// Package policy implements tiered, tighten-only access control with glob
// allow/deny rules per target kind. It is product-agnostic.
package policy

import (
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

// Tier orders operations by risk: Read < Write < Danger.
type Tier int

const (
	TierRead   Tier = 1
	TierWrite  Tier = 2
	TierDanger Tier = 3
)

// DefaultMaxLogLines is used when Policy.MaxLogLines is 0.
const DefaultMaxLogLines = 2000

func (t Tier) String() string {
	switch t {
	case TierRead:
		return "read"
	case TierWrite:
		return "write"
	case TierDanger:
		return "danger"
	default:
		return fmt.Sprintf("tier(%d)", int(t))
	}
}

// ParseTier parses "read", "write" or "danger" (case-insensitive).
func ParseTier(s string) (Tier, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "read":
		return TierRead, nil
	case "write":
		return TierWrite, nil
	case "danger":
		return TierDanger, nil
	}
	return 0, errs.Newf(errs.CodeUsage, "invalid tier %q", s).WithHint("use read, write or danger")
}

func (t Tier) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

func (t *Tier) UnmarshalText(b []byte) error {
	v, err := ParseTier(string(b))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// Rules maps a rule key to doublestar globs. The key is the target kind
// ("job") or its plural as written in config ("jobs"); both are looked up.
type Rules map[string][]string

// Policy is the effective policy for one profile.
type Policy struct {
	Tier        Tier
	Allow       Rules
	Deny        Rules
	MaxLogLines int
}

// Validate reports malformed globs, an out-of-range tier or a negative
// MaxLogLines as a USAGE error. A zero Tier or MaxLogLines is valid (defaults).
func (p Policy) Validate() error {
	if p.Tier != 0 && (p.Tier < TierRead || p.Tier > TierDanger) {
		return errs.Newf(errs.CodeUsage, "invalid tier %d", int(p.Tier))
	}
	if p.MaxLogLines < 0 {
		return errs.Newf(errs.CodeUsage, "max_log_lines must be >= 0, got %d", p.MaxLogLines)
	}
	for name, rules := range map[string]Rules{"allow": p.Allow, "deny": p.Deny} {
		for kind, globs := range rules {
			for _, g := range globs {
				if g == "" || !doublestar.ValidatePattern(g) {
					return errs.Newf(errs.CodeUsage, "invalid %s.%s glob %q", name, kind, g).
						WithHint("use doublestar globs with '/' separators, e.g. \"team-a/**\"")
				}
			}
		}
	}
	return nil
}

// EffectiveTier returns min(configured, flag). An empty flag keeps configured.
// A flag above configured returns POLICY_DENIED (tighten-only).
// A zero configured tier is treated as TierRead.
func EffectiveTier(configured Tier, flag string) (Tier, error) {
	if configured == 0 {
		configured = TierRead
	}
	if strings.TrimSpace(flag) == "" {
		return configured, nil
	}
	want, err := ParseTier(flag)
	if err != nil {
		return 0, err
	}
	if want > configured {
		return 0, errs.Newf(errs.CodePolicyDenied, "tier %s exceeds configured tier %s", want, configured).
			WithHint("--tier can only lower the tier; raise policy.tier in the config file")
	}
	return want, nil
}

// Engine evaluates a Policy.
type Engine struct {
	p Policy
	// allow and deny hold the globs case-folded once (see fold).
	allow, deny Rules
}

// NewEngine returns an engine for p; zero Tier becomes TierRead and zero
// MaxLogLines becomes DefaultMaxLogLines.
func NewEngine(p Policy) *Engine {
	if p.Tier == 0 {
		p.Tier = TierRead
	}
	if p.MaxLogLines <= 0 {
		p.MaxLogLines = DefaultMaxLogLines
	}
	return &Engine{p: p, allow: foldRules(p.Allow), deny: foldRules(p.Deny)}
}

func (e *Engine) Policy() Policy   { return e.p }
func (e *Engine) Tier() Tier       { return e.p.Tier }
func (e *Engine) MaxLogLines() int { return e.p.MaxLogLines }

// Check returns POLICY_DENIED if op exceeds the tier or target is not allowed.
// An empty targetKind or target skips the glob check.
func (e *Engine) Check(op Tier, targetKind, target string) error {
	if op > e.p.Tier {
		return errs.Newf(errs.CodePolicyDenied, "operation requires tier %s, effective tier is %s", op, e.p.Tier).
			WithHint("a human must raise policy.tier in the config file; flags and env vars cannot raise it")
	}
	if targetKind == "" || target == "" {
		return nil
	}
	if !e.Allowed(targetKind, target) {
		return errs.Newf(errs.CodePolicyDenied, "%s %q is not allowed by policy", targetKind, target).
			WithHint("the profile's allow/deny globs exclude this target")
	}
	return nil
}

// Allowed reports whether target passes allow/deny globs for targetKind.
// Deny wins; an empty allow list allows everything. Matching is
// case-insensitive because Jenkins resolves item names case-insensitively.
func (e *Engine) Allowed(targetKind, target string) bool {
	t := fold(target)
	if matchAny(rulesFor(e.deny, targetKind), t) {
		return false
	}
	allow := rulesFor(e.allow, targetKind)
	return len(allow) == 0 || matchAny(allow, t)
}

// AllowedOrAncestor is Allowed, but also true when target is a strict
// ancestor of a path some allow glob could match (e.g. "team-a" for
// "team-a/app/**"), so containers leading to allowed targets can be listed.
// Deny still wins. Use it only for container browsing, never for actions.
func (e *Engine) AllowedOrAncestor(targetKind, target string) bool {
	t := fold(target)
	if matchAny(rulesFor(e.deny, targetKind), t) {
		return false
	}
	allow := rulesFor(e.allow, targetKind)
	if len(allow) == 0 || matchAny(allow, t) {
		return true
	}
	for _, g := range allow {
		if isAncestor(g, t) {
			return true
		}
	}
	return false
}

// isAncestor reports whether target is a strict prefix (by segments) of some
// path matched by glob. A "**" segment matches anything below it.
func isAncestor(glob, target string) bool {
	if target == "" {
		return true
	}
	gs, ts := strings.Split(glob, "/"), strings.Split(target, "/")
	for i, seg := range ts {
		if i >= len(gs) {
			return false
		}
		if gs[i] == "**" {
			return true
		}
		if ok, _ := doublestar.Match(gs[i], seg); !ok {
			return false
		}
	}
	return len(gs) > len(ts)
}

// fold mirrors Java's String.CASE_INSENSITIVE_ORDER (toUpperCase then
// toLowerCase), which Jenkins uses for item name lookup.
func fold(s string) string { return strings.ToLower(strings.ToUpper(s)) }

func foldRules(r Rules) Rules {
	if r == nil {
		return nil
	}
	out := make(Rules, len(r))
	for k, globs := range r {
		f := make([]string, len(globs))
		for i, g := range globs {
			f[i] = fold(g)
		}
		out[k] = f
	}
	return out
}

// ClampLines caps n to MaxLogLines; n <= 0 means "use the cap". The bool
// reports whether the request was reduced.
func (e *Engine) ClampLines(n int) (int, bool) {
	if n <= 0 {
		return e.p.MaxLogLines, false
	}
	if n > e.p.MaxLogLines {
		return e.p.MaxLogLines, true
	}
	return n, false
}

func rulesFor(r Rules, kind string) []string {
	if r == nil {
		return nil
	}
	out := append([]string{}, r[kind]...)
	return append(out, r[kind+"s"]...)
}

func matchAny(globs []string, target string) bool {
	for _, g := range globs {
		if ok, _ := doublestar.Match(g, target); ok {
			return true
		}
	}
	return false
}

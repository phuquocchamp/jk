package policy

import (
	"testing"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

func TestParseTier(t *testing.T) {
	for in, want := range map[string]Tier{"read": TierRead, "WRITE": TierWrite, " danger ": TierDanger} {
		got, err := ParseTier(in)
		if err != nil || got != want {
			t.Errorf("ParseTier(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseTier("admin"); !errs.Is(err, errs.CodeUsage) {
		t.Errorf("ParseTier(admin) err = %v, want USAGE", err)
	}
	if !(TierRead < TierWrite && TierWrite < TierDanger) {
		t.Fatal("tier ordering broken")
	}
}

func TestTierText(t *testing.T) {
	var tr Tier
	if err := tr.UnmarshalText([]byte("write")); err != nil || tr != TierWrite {
		t.Fatalf("UnmarshalText = %v, %v", tr, err)
	}
	b, _ := TierDanger.MarshalText()
	if string(b) != "danger" {
		t.Fatalf("MarshalText = %s", b)
	}
}

func TestEffectiveTier(t *testing.T) {
	cases := []struct {
		cfg      Tier
		flag     string
		want     Tier
		wantCode errs.Code
	}{
		{0, "", TierRead, ""},
		{TierWrite, "", TierWrite, ""},
		{TierDanger, "read", TierRead, ""},
		{TierDanger, "write", TierWrite, ""},
		{TierWrite, "write", TierWrite, ""},
		{TierRead, "write", 0, errs.CodePolicyDenied},
		{TierWrite, "danger", 0, errs.CodePolicyDenied},
		{0, "write", 0, errs.CodePolicyDenied}, // no config -> read; cannot raise
		{TierWrite, "bogus", 0, errs.CodeUsage},
	}
	for _, c := range cases {
		got, err := EffectiveTier(c.cfg, c.flag)
		if c.wantCode != "" {
			if !errs.Is(err, c.wantCode) {
				t.Errorf("EffectiveTier(%v,%q) err = %v, want %s", c.cfg, c.flag, err, c.wantCode)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("EffectiveTier(%v,%q) = %v, %v; want %v", c.cfg, c.flag, got, err, c.want)
		}
	}
}

func TestEngineDefaults(t *testing.T) {
	e := NewEngine(Policy{})
	if e.Tier() != TierRead || e.MaxLogLines() != DefaultMaxLogLines || DefaultMaxLogLines != 2000 {
		t.Fatalf("defaults: tier=%v max=%d", e.Tier(), e.MaxLogLines())
	}
	if err := e.Check(TierWrite, "", ""); !errs.Is(err, errs.CodePolicyDenied) {
		t.Fatalf("write on read engine: %v", err)
	}
	if err := e.Check(TierRead, "job", "anything/at/all"); err != nil {
		t.Fatalf("read with no rules: %v", err)
	}
}

func TestEngineTierCheck(t *testing.T) {
	e := NewEngine(Policy{Tier: TierWrite})
	if err := e.Check(TierRead, "", ""); err != nil {
		t.Error(err)
	}
	if err := e.Check(TierWrite, "", ""); err != nil {
		t.Error(err)
	}
	if err := e.Check(TierDanger, "", ""); !errs.Is(err, errs.CodePolicyDenied) {
		t.Errorf("danger on write engine: %v", err)
	}
}

func TestEngineAllowDeny(t *testing.T) {
	e := NewEngine(Policy{
		Tier:  TierDanger,
		Allow: Rules{"jobs": {"team-a/**", "shared/tool"}},
		Deny:  Rules{"jobs": {"team-a/prod/**"}, "job": {"**/release-*"}},
	})
	cases := map[string]bool{
		"team-a":                   true, // doublestar: "team-a/**" matches the folder itself
		"team-a/svc":               true,
		"team-a/svc/feature%2Ffoo": true,
		"team-a/prod":              false, // deny wins
		"team-a/prod/deploy":       false,
		"team-a/svc/release-1.0":   false, // deny via singular key
		"shared/tool":              true,
		"shared/other":             false,
		"team-b/svc":               false,
		"org":                      false,
	}
	for target, want := range cases {
		if got := e.Allowed("job", target); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", target, got, want)
		}
		err := e.Check(TierRead, "job", target)
		if want && err != nil {
			t.Errorf("Check(%q) = %v", target, err)
		}
		if !want && !errs.Is(err, errs.CodePolicyDenied) {
			t.Errorf("Check(%q) = %v, want POLICY_DENIED", target, err)
		}
	}
	// Empty target or kind skips globs.
	if err := e.Check(TierRead, "job", ""); err != nil {
		t.Errorf("empty target: %v", err)
	}
	// Other kinds are unaffected by job rules.
	if !e.Allowed("node", "team-b/svc") {
		t.Error("rules for jobs leaked to another kind")
	}
}

func TestDenyOnlyAllowsRest(t *testing.T) {
	e := NewEngine(Policy{Deny: Rules{"jobs": {"prod/**"}}})
	if !e.Allowed("job", "dev/app") || e.Allowed("job", "prod/app") || e.Allowed("job", "prod") {
		t.Fatal("deny-only policy wrong")
	}
}

func TestClampLines(t *testing.T) {
	e := NewEngine(Policy{MaxLogLines: 100})
	for _, c := range []struct {
		in, want int
		clamped  bool
	}{{0, 100, false}, {-5, 100, false}, {50, 50, false}, {100, 100, false}, {101, 100, true}} {
		got, cl := e.ClampLines(c.in)
		if got != c.want || cl != c.clamped {
			t.Errorf("ClampLines(%d) = %d,%v want %d,%v", c.in, got, cl, c.want, c.clamped)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := (Policy{}).Validate(); err != nil {
		t.Fatal(err)
	}
	ok := Policy{Tier: TierWrite, Allow: Rules{"jobs": {"team-a/**"}}, Deny: Rules{"jobs": {"prod/**"}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []Policy{
		{Tier: 9},
		{MaxLogLines: -1},
		{Allow: Rules{"jobs": {"team-[a"}}},
		{Deny: Rules{"jobs": {""}}},
	}
	for _, p := range bad {
		if err := p.Validate(); !errs.Is(err, errs.CodeUsage) {
			t.Errorf("Validate(%+v) = %v, want USAGE", p, err)
		}
	}
}

func TestAllowDenyCaseInsensitive(t *testing.T) {
	e := NewEngine(Policy{
		Tier:  TierDanger,
		Allow: Rules{"jobs": {"team-a/**", "prod/**"}},
		Deny:  Rules{"jobs": {"prod/**"}},
	})
	for _, target := range []string{"prod/app", "PROD/app", "Prod/App", "pRoD/x/y"} {
		if e.Allowed("job", target) {
			t.Errorf("Allowed(%q) = true, want denied", target)
		}
		if err := e.Check(TierRead, "job", target); !errs.Is(err, errs.CodePolicyDenied) {
			t.Errorf("Check(%q) err = %v, want POLICY_DENIED", target, err)
		}
	}
	for _, target := range []string{"team-a/x", "TEAM-A/x", "Team-A/Sub/Job"} {
		if !e.Allowed("job", target) {
			t.Errorf("Allowed(%q) = false, want allowed", target)
		}
	}
	// Long s (U+017F) folds to "s" like Java's CASE_INSENSITIVE_ORDER.
	e2 := NewEngine(Policy{Tier: TierRead, Deny: Rules{"jobs": {"secret/**"}}})
	if e2.Allowed("job", "ſecret/app") {
		t.Error("long-s variant of a denied folder must be denied")
	}
	// The original globs are kept as written.
	if got := e.Policy().Deny["jobs"][0]; got != "prod/**" {
		t.Errorf("Policy().Deny mutated: %q", got)
	}
}

func TestAllowedOrAncestor(t *testing.T) {
	e := NewEngine(Policy{
		Tier:  TierRead,
		Allow: Rules{"jobs": {"team-a/app/**", "team-*/web"}},
		Deny:  Rules{"jobs": {"team-c/**"}},
	})
	cases := map[string]bool{
		"team-a":         true,  // ancestor of team-a/app/**
		"TEAM-A":         true,  // case-insensitive
		"team-a/app":     true,  // ancestor (and ** may match below it)
		"team-a/app/x":   true,  // allowed itself
		"team-a/other":   false, // neither allowed nor ancestor
		"team-b":         true,  // ancestor of team-*/web
		"team-b/web":     true,  // allowed
		"team-b/web/sub": false, // below a non-** leaf
		"team-c":         false, // denied wins
		"other":          false,
	}
	for target, want := range cases {
		if got := e.AllowedOrAncestor("job", target); got != want {
			t.Errorf("AllowedOrAncestor(%q) = %v, want %v", target, got, want)
		}
	}
	if e.Allowed("job", "team-a") {
		t.Error("Allowed(team-a) must stay false; only listing uses the ancestor rule")
	}
}

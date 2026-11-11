package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

const sample = `profiles:
  default:
    url: https://jenkins.example.com/
    user: jane
    policy:
      tier: write
      allow: { jobs: ["team-a/**"] }
      deny:  { jobs: ["prod/**"] }
      max_log_lines: 500
  ci:
    url: https://ci.example.com
default_profile: default
`

func opts(t *testing.T, env map[string]string) Options {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if env == nil {
		env = map[string]string{}
	}
	env["XDG_CONFIG_HOME"] = dir
	return Options{
		Dir: "qtools", App: "jk",
		URLEnv:  []string{"PRIMARY_URL", "JENKINS_URL"},
		UserEnv: []string{"PRIMARY_USER", "JENKINS_USER"},
		Getenv:  func(k string) string { return env[k] },
	}
}

func writeConfig(t *testing.T, o Options, body string, mode os.FileMode) string {
	t.Helper()
	p, err := Path(o)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil { // bypass umask
		t.Fatal(err)
	}
	return p
}

func load(t *testing.T, o Options) *File {
	t.Helper()
	f, _, err := Load(o)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestPath(t *testing.T) {
	o := opts(t, nil)
	p, _ := Path(o)
	if want := filepath.Join(o.Getenv("XDG_CONFIG_HOME"), "qtools", "jk.yaml"); p != want {
		t.Fatalf("Path = %s, want %s", p, want)
	}
	o.Getenv = func(string) string { return "" }
	p, err := Path(o)
	if err != nil || !strings.HasSuffix(p, filepath.Join(".config", "qtools", "jk.yaml")) {
		t.Fatalf("fallback Path = %s, %v", p, err)
	}
}

func TestMissingFileDefaults(t *testing.T) {
	o := opts(t, nil)
	f, p, err := Load(o)
	if err != nil || p == "" {
		t.Fatal(err)
	}
	r, err := f.Resolve("", o)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "default" || r.FromFile || r.URL != "" || r.Path != p {
		t.Fatalf("resolved = %+v", r)
	}
	if r.Policy.Tier != policy.TierRead || r.Policy.MaxLogLines != 2000 {
		t.Fatalf("policy = %+v", r.Policy)
	}
}

func TestResolveFromFile(t *testing.T) {
	o := opts(t, nil)
	writeConfig(t, o, sample, 0o600)
	f := load(t, o)
	if len(f.Warnings) != 0 {
		t.Fatalf("unexpected warnings %v", f.Warnings)
	}
	r, err := f.Resolve("", o)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "default" || !r.FromFile || r.URL != "https://jenkins.example.com" || r.User != "jane" || r.UserSource != "file" {
		t.Fatalf("resolved = %+v", r)
	}
	if r.Policy.Tier != policy.TierWrite || r.Policy.MaxLogLines != 500 {
		t.Fatalf("policy = %+v", r.Policy)
	}
	e := policy.NewEngine(r.Policy)
	if !e.Allowed("job", "team-a/x") || e.Allowed("job", "prod/x") || e.Allowed("job", "team-b/x") {
		t.Fatal("rules not loaded")
	}

	r, err = f.Resolve("ci", o)
	if err != nil || r.URL != "https://ci.example.com" || r.Policy.Tier != policy.TierRead {
		t.Fatalf("ci = %+v, %v", r, err)
	}
}

func TestDefaultProfileKey(t *testing.T) {
	o := opts(t, nil)
	writeConfig(t, o, "profiles:\n  work:\n    url: https://w.example.com\ndefault_profile: work\n", 0o600)
	r, err := load(t, o).Resolve("", o)
	if err != nil || r.Name != "work" || r.URL != "https://w.example.com" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestEnvOverrides(t *testing.T) {
	o := opts(t, map[string]string{
		"JENKINS_URL":  "https://env2.example.com",
		"PRIMARY_URL":  "https://env1.example.com",
		"JENKINS_USER": "john",
		"PRIMARY_USER": "",
	})
	writeConfig(t, o, sample, 0o600)
	r, err := load(t, o).Resolve("", o)
	if err != nil {
		t.Fatal(err)
	}
	if r.URL != "https://env1.example.com" || r.User != "john" || r.UserSource != "env:JENKINS_USER" {
		t.Fatalf("resolved = %+v", r)
	}
	// Env never changes the tier.
	if r.Policy.Tier != policy.TierWrite {
		t.Fatalf("tier = %v", r.Policy.Tier)
	}
}

func TestUnknownProfile(t *testing.T) {
	o := opts(t, nil)
	writeConfig(t, o, sample, 0o600)
	f := load(t, o)
	_, err := f.Resolve("nope", o)
	if !errs.Is(err, errs.CodeUsage) {
		t.Fatalf("err = %v", err)
	}
	if e, _ := errs.As(err); !strings.Contains(e.Hint, "ci, default") {
		t.Errorf("hint = %q", e.Hint)
	}

	// Env URL makes an unknown profile usable (read tier).
	o2 := o
	o2.Getenv = func(k string) string {
		if k == "JENKINS_URL" {
			return "https://env.example.com"
		}
		return o.Getenv(k)
	}
	r, err := f.Resolve("nope", o2)
	if err != nil || r.FromFile || r.Policy.Tier != policy.TierRead {
		t.Fatalf("%+v %v", r, err)
	}

	// default_profile pointing at a missing profile is an error too.
	g := &File{DefaultProfile: "ghost"}
	if _, err := g.Resolve("", o); !errs.Is(err, errs.CodeUsage) {
		t.Fatalf("ghost default: %v", err)
	}
}

func TestInvalidConfig(t *testing.T) {
	cases := map[string]string{
		"bad yaml":    "profiles: [",
		"unknown key": "profiles:\n  default:\n    url: https://x.example.com\n    policy:\n      dney: { jobs: [\"a\"] }\n",
		"bad tier":    "profiles:\n  default:\n    policy:\n      tier: admin\n",
		"bad glob":    "profiles:\n  default:\n    policy:\n      allow: { jobs: [\"team-[a\"] }\n",
		"neg lines":   "profiles:\n  default:\n    policy:\n      max_log_lines: -1\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			o := opts(t, nil)
			writeConfig(t, o, body, 0o600)
			f, _, err := Load(o)
			if err == nil {
				_, err = f.Resolve("", o)
			}
			if !errs.Is(err, errs.CodeUsage) {
				t.Fatalf("err = %v, want USAGE", err)
			}
		})
	}
}

func TestEmptyFile(t *testing.T) {
	o := opts(t, nil)
	writeConfig(t, o, "", 0o600)
	r, err := load(t, o).Resolve("", o)
	if err != nil || r.Policy.Tier != policy.TierRead {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestPermissionWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX modes")
	}
	o := opts(t, nil)
	writeConfig(t, o, sample, 0o644)
	f := load(t, o)
	if len(f.Warnings) != 1 || !strings.Contains(f.Warnings[0], "chmod 600") {
		t.Fatalf("warnings = %v", f.Warnings)
	}
	r, err := f.Resolve("", o)
	if err != nil || len(r.Warnings) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}

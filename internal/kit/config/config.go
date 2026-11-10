// Package config loads the per-tool YAML config with named profiles.
// Path: $XDG_CONFIG_HOME/<Dir>/<App>.yaml, falling back to ~/.config/<Dir>/<App>.yaml.
package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

// PolicyConfig is the YAML form of a policy.
type PolicyConfig struct {
	Tier        string       `yaml:"tier,omitempty"`
	Allow       policy.Rules `yaml:"allow,omitempty"`
	Deny        policy.Rules `yaml:"deny,omitempty"`
	MaxLogLines int          `yaml:"max_log_lines,omitempty"`
}

// Profile is one named profile in the config file.
type Profile struct {
	URL    string       `yaml:"url,omitempty"`
	User   string       `yaml:"user,omitempty"`
	Policy PolicyConfig `yaml:"policy,omitempty"`
}

// File is the whole config file.
type File struct {
	Profiles       map[string]Profile `yaml:"profiles,omitempty"`
	DefaultProfile string             `yaml:"default_profile,omitempty"`

	// Warnings are non-fatal problems found while loading (e.g. file mode).
	Warnings []string `yaml:"-"`
	path     string
}

// Options describe where to look and which env vars override url/user.
type Options struct {
	Dir     string   // e.g. "qtools"
	App     string   // e.g. "jk" -> jk.yaml
	URLEnv  []string // e.g. JENKINS_URL (first non-empty wins)
	UserEnv []string // e.g. JENKINS_USER
	// Getenv defaults to os.Getenv.
	Getenv func(string) string
}

// Resolved is a profile after env overrides and defaults.
type Resolved struct {
	Name     string
	URL      string
	User     string
	Policy   policy.Policy // Tier defaults to read; MaxLogLines to policy.DefaultMaxLogLines
	Path     string        // config file path that was consulted
	FromFile bool          // the profile existed in the file
	Warnings []string      // copied from File.Warnings
}

// Path returns the config file path for o.
func Path(o Options) (string, error) {
	getenv := o.getenv()
	base := getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errs.Wrap(errs.CodeInternal, err, "resolve home directory")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, o.Dir, o.App+".yaml"), nil
}

// Load reads the config file. A missing or empty file yields an empty File and
// no error. Unknown keys are a USAGE error, so a typo such as "dney" cannot
// silently weaken the policy. A file readable by group or others is loaded but
// produces a warning (File.Warnings, also logged via slog). The returned string
// is the path consulted.
func Load(o Options) (*File, string, error) {
	p, err := Path(o)
	if err != nil {
		return nil, "", err
	}
	fh, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return &File{path: p}, p, nil
	}
	if err != nil {
		return nil, p, errs.Wrap(errs.CodeInternal, err, "read config "+p)
	}
	defer fh.Close()

	f := File{path: p}
	dec := yaml.NewDecoder(fh)
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, p, errs.Wrap(errs.CodeUsage, err, "parse config "+p).
			WithHint("fix the YAML in " + p + ": " + err.Error())
	}
	if w := permissionWarning(fh, p); w != "" {
		f.Warnings = append(f.Warnings, w)
		slog.Warn(w)
	}
	return &f, p, nil
}

// permissionWarning returns a warning when the file is readable by group or
// others. Windows has no POSIX mode bits, so it is skipped there.
func permissionWarning(fh *os.File, path string) string {
	if runtime.GOOS == "windows" {
		return ""
	}
	st, err := fh.Stat()
	if err != nil {
		return ""
	}
	if st.Mode().Perm()&0o044 != 0 {
		return fmt.Sprintf("config %s is readable by group or others (mode %04o); run: chmod 600 %s",
			path, st.Mode().Perm(), path)
	}
	return ""
}

// Resolve picks the profile name (empty -> DefaultProfile -> "default"),
// applies env overrides for URL/User, and builds and validates the policy.
// A profile named explicitly (argument or default_profile) that is missing
// from the file is a USAGE error unless env supplies the URL. The implicit
// "default" may be missing: env-only setups then get the read tier.
func (f *File) Resolve(name string, o Options) (Resolved, error) {
	explicit := name != ""
	if name == "" {
		name = f.DefaultProfile
		explicit = name != ""
	}
	if name == "" {
		name = "default"
	}
	prof, ok := f.Profiles[name]
	r := Resolved{Name: name, URL: prof.URL, User: prof.User, FromFile: ok, Path: f.path,
		Warnings: append([]string(nil), f.Warnings...)}
	getenv := o.getenv()
	envURL := firstEnv(getenv, o.URLEnv)
	if envURL != "" {
		r.URL = envURL
	}
	if v := firstEnv(getenv, o.UserEnv); v != "" {
		r.User = v
	}
	if explicit && !ok && envURL == "" {
		return r, errs.Newf(errs.CodeUsage, "profile %q not found in config", name).
			WithHint(profileHint(f))
	}
	r.URL = strings.TrimRight(strings.TrimSpace(r.URL), "/")

	tier := policy.TierRead
	if prof.Policy.Tier != "" {
		t, err := policy.ParseTier(prof.Policy.Tier)
		if err != nil {
			return r, err
		}
		tier = t
	}
	r.Policy = policy.Policy{Tier: tier, Allow: prof.Policy.Allow, Deny: prof.Policy.Deny, MaxLogLines: prof.Policy.MaxLogLines}
	if err := r.Policy.Validate(); err != nil {
		if e, ok := errs.As(err); ok {
			e.Message = fmt.Sprintf("profile %q: %s", name, e.Message)
		}
		return r, err
	}
	if r.Policy.MaxLogLines == 0 {
		r.Policy.MaxLogLines = policy.DefaultMaxLogLines
	}
	return r, nil
}

// ProfileNames returns the profile names in the file, sorted.
func (f *File) ProfileNames() []string {
	names := make([]string, 0, len(f.Profiles))
	for n := range f.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func profileHint(f *File) string {
	names := f.ProfileNames()
	if len(names) == 0 {
		return "no profiles are configured; add one to the config file or set the URL env var"
	}
	return "available profiles: " + strings.Join(names, ", ")
}

func (o Options) getenv() func(string) string {
	if o.Getenv != nil {
		return o.Getenv
	}
	return os.Getenv
}

func firstEnv(getenv func(string) string, names []string) string {
	for _, n := range names {
		if v := getenv(n); v != "" {
			return v
		}
	}
	return ""
}

// Package auth resolves API tokens from environment variables or the OS keyring.
// Tokens are never logged or returned in any user-facing output.
package auth

import (
	"errors"
	"os"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

// Source says where a token came from, e.g. "env:JENKINS_API_TOKEN" or "keyring".
type Source string

const (
	SourceKeyring Source = "keyring"
	SourceNone    Source = "none"
)

// EnvSource returns the Source for an environment variable name.
func EnvSource(name string) Source { return Source("env:" + name) }

// Resolver finds a token for a profile. Env vars are checked in order, then the
// keyring entry (Service, account=profile).
type Resolver struct {
	Service string
	EnvVars []string
	// Getenv defaults to os.Getenv; tests may override it.
	Getenv func(string) string
}

// NewResolver builds a Resolver, e.g. NewResolver("jk", "JENKINS_API_TOKEN").
func NewResolver(service string, envVars ...string) *Resolver {
	return &Resolver{Service: service, EnvVars: envVars, Getenv: os.Getenv}
}

// Resolve returns the token and its source, or AUTH_REQUIRED with SourceNone.
// A keyring backend failure (no Secret Service, locked keychain) is also
// AUTH_REQUIRED, with a hint pointing at the env vars.
func (r *Resolver) Resolve(profile string) (string, Source, error) {
	getenv := r.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	for _, name := range r.EnvVars {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			return v, EnvSource(name), nil
		}
	}
	tok, err := keyring.Get(r.Service, profile)
	tok = strings.TrimSpace(tok)
	if err == nil && tok != "" {
		return tok, SourceKeyring, nil
	}
	e := errs.Newf(errs.CodeAuthRequired, "no API token for profile %q", profile)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		// Keep the cause unserialized (Err is json:"-"); keyring errors carry no secret.
		e.Err = err
		return "", SourceNone, e.WithHint("OS keyring unavailable; set " + r.envList() + " instead")
	}
	return "", SourceNone, e.WithHint("run 'auth login --with-token' or set " + r.envList())
}

// Source reports where a token for profile would come from without returning
// it: "env:NAME", "keyring" or "none". Keyring errors yield "none".
func (r *Resolver) Source(profile string) Source {
	_, src, err := r.Resolve(profile)
	if err != nil {
		return SourceNone
	}
	return src
}

// Store saves token in the keyring under (Service, profile). Surrounding
// whitespace (e.g. a trailing newline from stdin) is trimmed.
func (r *Resolver) Store(profile, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errs.Usage("empty token").WithHint("pipe the API token on stdin: auth login --with-token")
	}
	if err := keyring.Set(r.Service, profile, token); err != nil {
		return errs.Wrap(errs.CodeInternal, err, "store token in keyring").
			WithHint("set " + r.envList() + " if no OS keyring is available")
	}
	return nil
}

// Delete removes the keyring entry; a missing entry is not an error.
func (r *Resolver) Delete(profile string) error {
	if err := keyring.Delete(r.Service, profile); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return errs.Wrap(errs.CodeInternal, err, "delete token from keyring")
	}
	return nil
}

func (r *Resolver) envList() string {
	if len(r.EnvVars) == 0 {
		return "an API token env var"
	}
	return strings.Join(r.EnvVars, " or ")
}

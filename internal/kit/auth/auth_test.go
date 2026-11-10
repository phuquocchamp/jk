package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

func newTestResolver(env map[string]string) *Resolver {
	r := NewResolver("jk-test", "PRIMARY_API_TOKEN", "JENKINS_API_TOKEN")
	r.Getenv = func(k string) string { return env[k] }
	return r
}

func TestResolveOrder(t *testing.T) {
	keyring.MockInit()
	r := newTestResolver(map[string]string{})
	if err := r.Store("default", "kr-token\n"); err != nil {
		t.Fatal(err)
	}

	tok, src, err := r.Resolve("default")
	if err != nil || tok != "kr-token" || src != SourceKeyring {
		t.Fatalf("keyring: %q %q %v", tok, src, err)
	}

	r = newTestResolver(map[string]string{"JENKINS_API_TOKEN": "j-token"})
	tok, src, _ = r.Resolve("default")
	if tok != "j-token" || src != "env:JENKINS_API_TOKEN" {
		t.Fatalf("JENKINS env: %q %q", tok, src)
	}

	r = newTestResolver(map[string]string{"JENKINS_API_TOKEN": "j-token", "PRIMARY_API_TOKEN": "q-token"})
	tok, src, _ = r.Resolve("default")
	if tok != "q-token" || src != EnvSource("PRIMARY_API_TOKEN") {
		t.Fatalf("PRIMARY env: %q %q", tok, src)
	}
	if r.Source("default") != "env:PRIMARY_API_TOKEN" {
		t.Fatalf("Source = %q", r.Source("default"))
	}
}

func TestResolveMissing(t *testing.T) {
	keyring.MockInit()
	r := newTestResolver(map[string]string{"PRIMARY_API_TOKEN": "   "})
	tok, src, err := r.Resolve("other")
	if !errs.Is(err, errs.CodeAuthRequired) || tok != "" || src != SourceNone {
		t.Fatalf("got %q %q %v", tok, src, err)
	}
	e, _ := errs.As(err)
	if !strings.Contains(e.Hint, "auth login") {
		t.Errorf("hint = %q", e.Hint)
	}
	if r.Source("other") != SourceNone {
		t.Errorf("Source = %q", r.Source("other"))
	}
}

func TestProfilesAreSeparate(t *testing.T) {
	keyring.MockInit()
	r := newTestResolver(nil)
	if err := r.Store("a", "tok-a"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Resolve("b"); !errs.Is(err, errs.CodeAuthRequired) {
		t.Fatalf("profile b should have no token: %v", err)
	}
}

func TestStoreDelete(t *testing.T) {
	keyring.MockInit()
	r := newTestResolver(nil)
	if err := r.Store("p", "  "); !errs.Is(err, errs.CodeUsage) {
		t.Fatalf("empty token: %v", err)
	}
	if err := r.Store("p", "secret"); err != nil {
		t.Fatal(err)
	}
	if r.Source("p") != SourceKeyring {
		t.Fatal("expected keyring source")
	}
	if err := r.Delete("p"); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete("p"); err != nil {
		t.Fatalf("deleting a missing entry must succeed: %v", err)
	}
	if r.Source("p") != SourceNone {
		t.Fatal("token still present after delete")
	}
}

func TestKeyringBackendError(t *testing.T) {
	keyring.MockInitWithError(errors.New("no secret service"))
	t.Cleanup(keyring.MockInit)
	r := newTestResolver(nil)
	_, src, err := r.Resolve("p")
	if !errs.Is(err, errs.CodeAuthRequired) || src != SourceNone {
		t.Fatalf("got %q %v", src, err)
	}
	e, _ := errs.As(err)
	if !strings.Contains(e.Hint, "PRIMARY_API_TOKEN") {
		t.Errorf("hint = %q", e.Hint)
	}
	if err := r.Store("p", "x"); !errs.Is(err, errs.CodeInternal) {
		t.Errorf("Store: %v", err)
	}
	if err := r.Delete("p"); !errs.Is(err, errs.CodeInternal) {
		t.Errorf("Delete: %v", err)
	}
}

func TestErrorNeverContainsToken(t *testing.T) {
	keyring.MockInit()
	r := newTestResolver(map[string]string{"JENKINS_API_TOKEN": "super-secret-value"})
	src := r.Source("default")
	if strings.Contains(string(src), "super-secret-value") {
		t.Fatal("source leaked token")
	}
}

package config

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/policy"
)

func readProfile() Profile {
	return Profile{URL: "https://jenkins.example.com", User: "jane", Policy: PolicyConfig{Tier: "read"}}
}

func TestAddProfileCreatesFile(t *testing.T) {
	o := opts(t, nil)
	res, err := AddProfile(o, "default", readProfile(), "Created by jk auth login.\nThe token is in the OS keyring.")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || !res.Added {
		t.Fatalf("result %+v", res)
	}
	data, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# Created by jk auth login.\n# The token is in the OS keyring.\n") {
		t.Fatalf("header missing:\n%s", data)
	}
	if strings.Contains(string(data), "default_profile") {
		t.Fatalf("default_profile set for the default profile:\n%s", data)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(res.Path)
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", st.Mode().Perm())
		}
	}
	f := load(t, o)
	if len(f.Warnings) != 0 {
		t.Fatalf("warnings %v", f.Warnings)
	}
	r, err := f.Resolve("", o)
	if err != nil || r.URL != "https://jenkins.example.com" || r.User != "jane" || r.Policy.Tier != policy.TierRead {
		t.Fatalf("resolved %+v %v", r, err)
	}
}

func TestAddProfileNamedSetsDefault(t *testing.T) {
	o := opts(t, nil)
	if _, err := AddProfile(o, "work", readProfile(), ""); err != nil {
		t.Fatal(err)
	}
	r, err := load(t, o).Resolve("", o)
	if err != nil || r.Name != "work" || r.ProfileSource != ProfileFromDefault {
		t.Fatalf("resolved %+v %v", r, err)
	}
}

func TestAddProfileKeepsExisting(t *testing.T) {
	o := opts(t, nil)
	body := "# my comment\n" + sample
	writeConfig(t, o, body, 0o600)

	res, err := AddProfile(o, "new", readProfile(), "ignored for existing files")
	if err != nil || res.Created || !res.Added {
		t.Fatalf("result %+v %v", res, err)
	}
	data, _ := os.ReadFile(res.Path)
	if !strings.Contains(string(data), "# my comment") || strings.Contains(string(data), "ignored") {
		t.Fatalf("comment handling:\n%s", data)
	}
	f := load(t, o)
	if _, ok := f.Profiles["new"]; !ok {
		t.Fatalf("profile not added:\n%s", data)
	}
	if f.Profiles["default"].Policy.Tier != "write" || f.DefaultProfile != "default" {
		t.Fatalf("existing settings changed:\n%s", data)
	}

	// Existing profile: nothing written.
	before, _ := os.ReadFile(res.Path)
	res, err = AddProfile(o, "default", readProfile(), "")
	after, _ := os.ReadFile(res.Path)
	if err != nil || res.Added || string(before) != string(after) {
		t.Fatalf("overwrote existing profile: %+v %v", res, err)
	}
}

func TestAddProfileRejectsBrokenFile(t *testing.T) {
	o := opts(t, nil)
	p := writeConfig(t, o, "profiles: [unclosed\n", 0o600)
	if _, err := AddProfile(o, "x", readProfile(), ""); err == nil {
		t.Fatal("expected an error")
	}
	data, _ := os.ReadFile(p)
	if string(data) != "profiles: [unclosed\n" {
		t.Fatalf("broken file was modified: %q", data)
	}
}

func TestRepoProfile(t *testing.T) {
	o := opts(t, nil)
	writeConfig(t, o, sample, 0o600)
	o.RepoProfile = func() string { return "ci\n" }
	f := load(t, o)

	r, err := f.Resolve("", o)
	if err != nil || r.Name != "ci" || r.ProfileSource != ProfileFromRepo {
		t.Fatalf("repo profile: %+v %v", r, err)
	}
	// --profile beats the repository.
	r, err = f.Resolve("default", o)
	if err != nil || r.Name != "default" || r.ProfileSource != ProfileFromFlag {
		t.Fatalf("flag: %+v %v", r, err)
	}
	// A repository profile that is missing is an error that says where it came from.
	o.RepoProfile = func() string { return "nope" }
	_, err = f.Resolve("", o)
	if !errors.Is(err, ErrProfileNotFound) || !strings.Contains(err.Error()+hintOf(err), "git config jk.profile") {
		t.Fatalf("missing repo profile: %v", err)
	}
	// No repository value: default_profile.
	o.RepoProfile = func() string { return "" }
	r, _ = f.Resolve("", o)
	if r.Name != "default" || r.ProfileSource != ProfileFromDefault {
		t.Fatalf("fallback: %+v", r)
	}
}

func hintOf(err error) string {
	if e, ok := errs.As(err); ok {
		return e.Hint
	}
	return ""
}

package core

import (
	"encoding/json"
	"testing"
)

func TestJobURLPath(t *testing.T) {
	cases := map[string]string{
		"":                   "/",
		"a":                  "/job/a/",
		"folder/sub/job":     "/job/folder/job/sub/job/job/",
		"proj/feature%2Ffoo": "/job/proj/job/feature%252Ffoo/",
		"team a/release 1.0": "/job/team%20a/job/release%201.0/",
		"sandbox/0.1.0":      "/job/sandbox/job/0.1.0/",
	}
	for in, want := range cases {
		if got := JobURLPath(in); got != want {
			t.Errorf("JobURLPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseJobInput(t *testing.T) {
	cases := []struct {
		in, path string
		ref      BuildRef
	}{
		{"folder/sub/job", "folder/sub/job", ""},
		{"/proj/feature%2Ffoo/", "proj/feature%2Ffoo", ""},
		{"/job/a/job/b/", "a/b", ""},
		{"https://jenkins.example.com/job/proj/job/feature%252Ffoo/", "proj/feature%2Ffoo", ""},
		{"https://jenkins.example.com/job/proj/job/main/42/console", "proj/main", "42"},
		{"https://jenkins.example.com/ci/job/a/job/b/lastFailedBuild/", "a/b", RefLastFailed},
		{"https://jenkins.example.com/job/a/job/b/12/execution/node/6/", "a/b", "12"},
	}
	for _, c := range cases {
		p, r, err := ParseJobInput(c.in)
		if err != nil || p != c.path || r != c.ref {
			t.Errorf("ParseJobInput(%q) = %q, %q, %v; want %q, %q", c.in, p, r, err, c.path, c.ref)
		}
	}
	for _, bad := range []string{"", "/", "a//b", "https://jenkins.example.com/view/all/"} {
		if _, _, err := ParseJobInput(bad); err == nil {
			t.Errorf("ParseJobInput(%q): want error", bad)
		}
	}
}

func TestParseBuildRef(t *testing.T) {
	ok := map[string]BuildRef{"": RefLast, "last": RefLast, "lastBuild": RefLast, "42": "42", "#7": "7",
		"lastfailed": RefLastFailed, "lastSuccessfulBuild": RefLastSuccessful, "lastCompleted": RefLastCompleted}
	for in, want := range ok {
		got, err := ParseBuildRef(in)
		if err != nil || got != want {
			t.Errorf("ParseBuildRef(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"0", "-1", "nope"} {
		if _, err := ParseBuildRef(bad); err == nil {
			t.Errorf("ParseBuildRef(%q): want error", bad)
		}
	}
	if RefLastFailed.Segment() != "lastFailedBuild" || BuildRef("42").Segment() != "42" {
		t.Error("Segment mapping")
	}
}

func TestBuildRefUnmarshalJSON(t *testing.T) {
	var in struct {
		Ref BuildRef `json:"ref"`
	}
	for raw, want := range map[string]BuildRef{`{"ref":42}`: "42", `{"ref":"last"}`: "last", `{"ref":null}`: "", `{}`: ""} {
		in.Ref = ""
		if err := json.Unmarshal([]byte(raw), &in); err != nil || in.Ref != want {
			t.Errorf("%s -> %q, %v; want %q", raw, in.Ref, err, want)
		}
	}
	for _, raw := range []string{`{"ref":4.5}`, `{"ref":true}`} {
		if err := json.Unmarshal([]byte(raw), &in); err == nil {
			t.Errorf("%s: want error", raw)
		}
	}
}

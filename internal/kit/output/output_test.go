package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

type item struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Color string `json:"color,omitempty"`
}

type list struct {
	Folder string `json:"folder"`
	Jobs   []item `json:"jobs"`
	Total  int    `json:"total"`
}

func sample() list {
	return list{Folder: "team-a", Total: 2, Jobs: []item{
		{Name: "api", Path: "team-a/api", Color: "blue"},
		{Name: "web", Path: "team-a/web", Color: "red"},
	}}
}

func decode(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("invalid JSON %q: %v", b, err)
	}
	return v
}

func TestRenderJSONKeepsOrder(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sample(), Opts{JSON: true}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if strings.Index(s, `"folder"`) > strings.Index(s, `"jobs"`) {
		t.Fatalf("struct field order not kept:\n%s", s)
	}
}

func TestRenderFieldsTopLevel(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sample(), Opts{JSON: true, Fields: []string{"total"}}); err != nil {
		t.Fatal(err)
	}
	got := decode(t, buf.Bytes()).(map[string]any)
	if len(got) != 1 || got["total"] != float64(2) {
		t.Fatalf("got %v", got)
	}
}

func TestRenderFieldsPerListItem(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sample(), Opts{JSON: true, Fields: []string{"name,path"}}); err != nil {
		t.Fatal(err)
	}
	got := decode(t, buf.Bytes()).(map[string]any)
	jobs, ok := got["jobs"].([]any)
	if !ok || len(jobs) != 2 || len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	j := jobs[0].(map[string]any)
	if len(j) != 2 || j["name"] != "api" || j["path"] != "team-a/api" {
		t.Fatalf("job not projected: %v", j)
	}
}

func TestProjectArray(t *testing.T) {
	in := []any{map[string]any{"a": 1.0, "b": 2.0}, map[string]any{"a": 3.0}}
	got := Project(in, []string{"a"}).([]any)
	if len(got[0].(map[string]any)) != 1 || got[1].(map[string]any)["a"] != 3.0 {
		t.Fatalf("got %v", got)
	}
}

func TestRenderJQ(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sample(), Opts{JQ: ".jobs[] | .name"}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "\"api\"\n\"web\"\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestRenderJQAfterFields(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sample(), Opts{Fields: []string{"total"}, JQ: "keys"}); err != nil {
		t.Fatal(err)
	}
	if got := decode(t, buf.Bytes()).([]any); len(got) != 1 || got[0] != "total" {
		t.Fatalf("got %v", got)
	}
}

func TestRenderJQErrors(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, sample(), Opts{JQ: ".jobs[ "})
	if !errs.Is(err, errs.CodeUsage) {
		t.Fatalf("parse error: got %v", err)
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Hint == "" {
		t.Fatalf("parse error has no hint: %v", err)
	}
	// runtime error is reported, not printed as a value
	buf.Reset()
	if err := Render(&buf, sample(), Opts{JQ: ".folder.x"}); !errs.Is(err, errs.CodeUsage) {
		t.Fatalf("runtime error: got %v (out %q)", err, buf.String())
	}
}

type texty struct{ Text string }

func (t texty) PlainText() string { return t.Text }

type tab struct{}

func (tab) TableHeader() []string { return []string{"name", "status"} }
func (tab) TableRows() [][]string { return [][]string{{"build", "SUCCESS"}, {"test", "FAILED"}} }

func TestRenderHuman(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, texty{"line1\nline2"}, Opts{}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "line1\nline2\n" {
		t.Fatalf("texter: %q", buf.String())
	}

	buf.Reset()
	if err := Render(&buf, tab{}, Opts{}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "NAME") || !strings.Contains(lines[2], "FAILED") {
		t.Fatalf("tabular: %q", buf.String())
	}

	buf.Reset()
	v := map[string]any{"number": 7, "duration_ms": 83000, "stages": []map[string]any{{"name": "build", "duration_ms": 1500}}}
	if err := Render(&buf, v, Opts{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"number:", "7", "1m23s", "stages:", "NAME", "1.5s"} {
		if !strings.Contains(out, want) {
			t.Fatalf("generic human output missing %q:\n%s", want, out)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	cases := map[int64]string{0: "0s", 450: "450ms", 1500: "1.5s", 83000: "1m23s", 7260000: "2h1m0s"}
	for ms, want := range cases {
		if got := HumanDuration(ms); got != want {
			t.Errorf("HumanDuration(%d) = %q, want %q", ms, got, want)
		}
	}
}

func TestSpillSanitizesPrefix(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	p, err := Spill("hello", "team-a/api/feature%2Ffoo")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(p) != filepath.Join(os.TempDir(), AppName) {
		t.Fatalf("unexpected dir %s", p)
	}
	if strings.ContainsAny(filepath.Base(p), "/%") {
		t.Fatalf("prefix not sanitized: %s", p)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "hello" {
		t.Fatalf("content %q err %v", b, err)
	}
}

func TestSpillRejectsSharedDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	shared := filepath.Join(tmp, AppName)
	if err := os.Mkdir(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o777); err != nil {
		t.Fatal(err)
	}
	p, err := Spill("secret", "log")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(p) == shared {
		t.Fatalf("spilled into world-writable dir: %s", p)
	}
	fi, err := os.Stat(filepath.Dir(p))
	if err != nil || fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("fallback dir not private: %v %v", fi.Mode(), err)
	}

	// A symlink planted at the shared path is not followed.
	tmp2 := t.TempDir()
	t.Setenv("TMPDIR", tmp2)
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(tmp2, AppName)); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	p, err = Spill("secret", "log")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(p) == filepath.Join(tmp2, AppName) || strings.HasPrefix(p, target) {
		t.Fatalf("followed planted symlink: %s", p)
	}
}

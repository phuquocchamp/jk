package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phuquocchamp/jk/internal/kit/policy"
)

func TestSetupSkill(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	dir := t.TempDir()
	skill := filepath.Join(dir, "jenkins", "SKILL.md")

	// --dry-run writes nothing.
	code, out, stderr := h.run("setup", "skill", "--dir", dir, "--dry-run")
	if code != 0 || !strings.Contains(out, `"action": "created"`) {
		t.Fatalf("dry run: exit %d %s %s", code, out, stderr)
	}
	if _, err := os.Stat(skill); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry run wrote the skill")
	}

	code, out, stderr = h.run("setup", "skill", "--dir", dir)
	if code != 0 {
		t.Fatalf("install: exit %d %s", code, stderr)
	}
	m := asMap(t, out)
	files := m["files"].([]any)
	if len(files) < 3 || m["dir"] != filepath.Join(dir, "jenkins") {
		t.Fatalf("got %v", m)
	}
	b, err := os.ReadFile(skill)
	if err != nil || !strings.Contains(string(b), "name: jenkins") {
		t.Fatalf("SKILL.md: %v", err)
	}

	// Again: nothing changes.
	_, out, _ = h.run("setup", "skill", "--dir", dir)
	if strings.Contains(out, `"created"`) || !strings.Contains(out, `"unchanged"`) {
		t.Fatalf("second run %s", out)
	}

	// A local edit is a conflict; nothing is written.
	if err := os.WriteFile(skill, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = h.run("setup", "skill", "--dir", dir)
	if code != 5 || errCode(t, stderr) != "CONFLICT" || !strings.Contains(stderr, "SKILL.md") {
		t.Fatalf("conflict: exit %d %s", code, stderr)
	}
	if b, _ := os.ReadFile(skill); string(b) != "edited" {
		t.Fatal("conflict overwrote the file")
	}
	code, out, _ = h.run("setup", "skill", "--dir", dir, "--force")
	if code != 0 || !strings.Contains(out, `"updated"`) {
		t.Fatalf("force: exit %d %s", code, out)
	}
}

func TestSetupSkillDefaultDir(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	home := t.TempDir()
	h.sys = &System{HomeDir: func() (string, error) { return home, nil }}
	if code, _, stderr := h.run("setup", "skill"); code != 0 {
		t.Fatalf("exit %d %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "jenkins", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

type fakeSys struct {
	claude bool
	ran    [][]string
	out    string
	err    error
}

func (f *fakeSys) system() *System {
	return &System{
		Executable: func() (string, error) { return "/opt/jk/bin/jk", nil },
		LookPath: func(file string) (string, error) {
			if file == "claude" && f.claude {
				return "/usr/local/bin/claude", nil
			}
			return "", errors.New("not found")
		},
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			f.ran = append(f.ran, append([]string{name}, args...))
			return []byte(f.out), f.err
		},
	}
}

func TestSetupMCP(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	fs := &fakeSys{claude: true, out: "Added stdio MCP server jk"}
	h.sys = fs.system()

	code, out, stderr := h.run("setup", "mcp", "--tier", "read", "--scope", "user")
	if code != 0 {
		t.Fatalf("exit %d %s", code, stderr)
	}
	want := "claude mcp add --scope user jk -- /opt/jk/bin/jk mcp --tier read"
	if len(fs.ran) != 1 || strings.Join(fs.ran[0], " ") != want {
		t.Fatalf("ran %q", fs.ran)
	}
	if m := asMap(t, out); m["client"] != "claude" || m["output"] != "Added stdio MCP server jk" {
		t.Fatalf("got %v", m)
	}
	for _, a := range fs.ran[0] {
		if strings.Contains(a, "TOKEN") || a == "-e" || a == "--env" {
			t.Fatalf("token passed to claude: %q", fs.ran[0])
		}
	}

	// --dry-run shows the command and runs nothing.
	fs.ran = nil
	code, out, _ = h.run("setup", "mcp", "--dry-run")
	if code != 0 || len(fs.ran) != 0 || !strings.Contains(out, `"--scope"`) {
		t.Fatalf("dry run: exit %d ran=%v %s", code, fs.ran, out)
	}

	// An existing entry is a conflict with a hint.
	fs.out, fs.err = "MCP server jk already exists in local config", errors.New("exit status 1")
	code, _, stderr = h.run("setup", "mcp")
	if code != 5 || !strings.Contains(stderr, "claude mcp remove jk") {
		t.Fatalf("exists: exit %d %s", code, stderr)
	}

	code, _, stderr = h.run("setup", "mcp", "--tier", "root")
	if code != 2 {
		t.Fatalf("bad tier: exit %d %s", code, stderr)
	}
	code, _, _ = h.run("setup", "mcp", "--scope", "global")
	if code != 2 {
		t.Fatalf("bad scope: exit %d", code)
	}
}

func TestSetupMCPPrintWithoutClaude(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	fs := &fakeSys{claude: false}
	h.sys = fs.system()
	code, out, stderr := h.run("setup", "mcp", "--tier", "read")
	if code != 0 || len(fs.ran) != 0 {
		t.Fatalf("exit %d ran=%v %s", code, fs.ran, stderr)
	}
	jk := asMap(t, out)["config"].(map[string]any)["mcpServers"].(map[string]any)["jk"].(map[string]any)
	if jk["command"] != "/opt/jk/bin/jk" || strings.Join(toStrings(jk["args"]), " ") != "mcp --tier read" {
		t.Fatalf("entry %v", jk)
	}
	// Asking for claude when it is missing is NOT_FOUND.
	code, _, _ = h.run("setup", "mcp", "--client", "claude")
	if code != 3 {
		t.Fatalf("missing claude: exit %d", code)
	}
}

func TestSetupPrintsSteps(t *testing.T) {
	h := newHarness(t, policy.TierRead)
	code, out, _ := h.run("setup")
	if code != 0 || !strings.Contains(out, "jk setup skill") || !strings.Contains(out, "jk auth login") {
		t.Fatalf("exit %d %s", code, out)
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

package clix

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

func TestHandleErrorJSON(t *testing.T) {
	var buf bytes.Buffer
	e := errs.New(errs.CodeNeedsConfirmation, "confirm").WithHint("use --yes").WithDetails(map[string]string{"path": "/x"})
	if code := HandleError(&buf, e, true); code != 7 {
		t.Fatalf("exit %d", code)
	}
	var env struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error["code"] != "NEEDS_CONFIRMATION" || env.Error["hint"] != "use --yes" || env.Error["retryable"] != false {
		t.Fatalf("got %v", env.Error)
	}
}

func TestHandleErrorText(t *testing.T) {
	var buf bytes.Buffer
	if code := HandleError(&buf, errors.New("boom"), false); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(buf.String(), "error: INTERNAL: boom") {
		t.Fatalf("got %q", buf.String())
	}
	if HandleError(&buf, nil, false) != 0 {
		t.Fatal("nil error must exit 0")
	}
}

func newRoot(g *Globals) *cobra.Command {
	root := &cobra.Command{Use: "tool", SilenceErrors: true, SilenceUsage: true}
	BindGlobals(root, g)
	grp := &cobra.Command{Use: "grp", Args: cobra.ArbitraryArgs, RunE: GroupRunE}
	grp.AddCommand(&cobra.Command{Use: "one <x>", Args: cobra.ExactArgs(1), RunE: func(*cobra.Command, []string) error { return nil }})
	root.AddCommand(grp)
	return root
}

func TestExecuteMapsUsage(t *testing.T) {
	for _, args := range [][]string{
		{"nope"}, {"grp", "nope"}, {"grp", "one"}, {"grp", "one", "a", "b"}, {"grp", "one", "a", "--bad"}, {"--tier"},
	} {
		g := &Globals{}
		root := newRoot(g)
		root.SetArgs(args)
		var out, errb bytes.Buffer
		root.SetOut(&out)
		if code := Execute(root, g, &out, &errb); code != 2 {
			t.Errorf("%v: exit %d (%s)", args, code, errb.String())
			continue
		}
		if !strings.Contains(errb.String(), `"USAGE"`) {
			t.Errorf("%v: stderr %q", args, errb.String())
		}
	}

	g := &Globals{}
	root := newRoot(g)
	root.SetArgs([]string{"grp", "one", "a"})
	var out, errb bytes.Buffer
	if code := Execute(root, g, &out, &errb); code != 0 {
		t.Fatalf("valid call: exit %d %s", code, errb.String())
	}
}

func TestJSONMode(t *testing.T) {
	var buf bytes.Buffer
	if !(&Globals{}).JSONMode(&buf) {
		t.Fatal("non-terminal writer must be JSON mode")
	}
	if IsTerminal(&buf) || CanPrompt(nil, &buf) {
		t.Fatal("buffer is not a terminal")
	}
}

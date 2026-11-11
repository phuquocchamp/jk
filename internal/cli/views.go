package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/output"
)

// Human views: named types over core outputs with the same JSON shape, adding
// output.Tabular / output.Texter for table and plain-text rendering.

type jobListView core.JobListOut

func (v jobListView) TableHeader() []string { return []string{"name", "kind", "color", "path"} }
func (v jobListView) TableRows() [][]string {
	rows := make([][]string, 0, len(v.Jobs))
	for _, j := range v.Jobs {
		rows = append(rows, []string{j.Name, j.Kind, j.Color, j.Path})
	}
	return rows
}

type stagesView core.BuildStagesOut

func (v stagesView) TableHeader() []string {
	return []string{"id", "name", "status", "duration", "started"}
}
func (v stagesView) TableRows() [][]string {
	rows := make([][]string, 0, len(v.Stages))
	for _, s := range v.Stages {
		rows = append(rows, []string{s.ID, s.Name, s.Status, output.HumanDuration(s.DurationMs), humanTime(s.StartTime)})
	}
	return rows
}

type logView core.BuildLogOut

func (v logView) PlainText() string { return v.Text }

type searchView core.LogSearchOut

// PlainText renders matches grep-style: "N:match" and "N-context", "--" between groups.
func (v searchView) PlainText() string {
	var b strings.Builder
	for i, m := range v.Matches {
		if i > 0 && (len(m.Before) > 0 || len(v.Matches[i-1].After) > 0) {
			b.WriteString("--\n")
		}
		start := m.Line - len(m.Before)
		for j, l := range m.Before {
			fmt.Fprintf(&b, "%d-%s\n", start+j, l)
		}
		fmt.Fprintf(&b, "%d:%s\n", m.Line, m.Text)
		for j, l := range m.After {
			fmt.Fprintf(&b, "%d-%s\n", m.Line+1+j, l)
		}
	}
	if len(v.Matches) == 0 {
		fmt.Fprintf(&b, "no matches for %q in %d lines\n", v.Pattern, v.ScannedLines)
	} else if v.Truncated {
		b.WriteString("(stopped early: more matches may exist; raise --max-matches)\n")
	}
	return b.String()
}

type testView core.TestFailedOut

func (v testView) PlainText() string {
	if !v.HasReport {
		return fmt.Sprintf("%s #%d: no test report\n", v.Job, v.Number)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s #%d: %d failed, %d passed, %d skipped\n", v.Job, v.Number, v.FailCount, v.PassCount, v.SkipCount)
	for _, c := range v.Failed {
		fmt.Fprintf(&b, "\n%s %s.%s (%s)\n", c.Status, c.ClassName, c.Name, output.HumanDuration(c.DurationMs))
		if c.ErrorDetails != "" {
			for _, l := range strings.Split(strings.TrimRight(c.ErrorDetails, "\n"), "\n") {
				b.WriteString("    " + l + "\n")
			}
		}
	}
	if v.Truncated {
		fmt.Fprintf(&b, "\n(showing %d of %d failed cases; raise --limit)\n", len(v.Failed), v.FailCount)
	}
	return b.String()
}

func humanTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

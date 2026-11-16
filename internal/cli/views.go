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
	return []string{"id", "name", "status", "duration", "failed step"}
}
func (v stagesView) TableRows() [][]string {
	depth := map[string]int{}
	rows := make([][]string, 0, len(v.Stages))
	for _, s := range v.Stages {
		d := 0
		if s.ParentID != "" {
			d = depth[s.ParentID] + 1
		}
		depth[s.ID] = d
		name := strings.Repeat("  ", d) + s.Name
		if s.ID == v.FirstFailed {
			name += "  <- first failure"
		} else if s.Inherited {
			name += "  (inherited)"
		}
		step := ""
		if s.FailedStep != nil {
			step = s.FailedStep.Name
			if s.FailedStep.Error != nil {
				step += " (" + s.FailedStep.Error.Message + ")"
			}
		}
		rows = append(rows, []string{s.ID, name, s.Status, output.HumanDuration(s.DurationMs), step})
	}
	return rows
}

type scriptView core.BuildReplayScriptOut

func (v scriptView) PlainText() string {
	if v.SpilledTo != "" {
		return fmt.Sprintf("script of %s #%d is %d bytes; written to %s\n", v.Job, v.Number, v.Bytes, v.SpilledTo)
	}
	return v.MainScript
}

type diagnoseView core.BuildDiagnoseOut

func (v diagnoseView) PlainText() string {
	var b strings.Builder
	state := v.Result
	if v.Building {
		state = "BUILDING"
	}
	fmt.Fprintf(&b, "%s #%d: %s (%s)\n%s\n", v.Job, v.Number, state, output.HumanDuration(v.DurationMs), v.URL)
	if f := v.Failure; f != nil {
		b.WriteString("\n")
		if f.Stage != "" {
			fmt.Fprintf(&b, "failed stage: %s (id %s)\n", f.Stage, f.StageID)
		}
		if f.Step != nil {
			fmt.Fprintf(&b, "failed step:  %s\n", f.Step.Name)
			if f.Step.Error != nil {
				fmt.Fprintf(&b, "error:        %s\n", f.Step.Error.Message)
			}
		}
		if len(v.FailedStages) > 1 {
			fmt.Fprintf(&b, "also failed:  %s\n", strings.Join(v.FailedStages[1:], ", "))
		}
		fmt.Fprintf(&b, "\n--- last %d lines of the %s log ---\n%s\n", f.LogLines, f.LogSource, f.Log)
	}
	if t := v.Tests; t != nil {
		fmt.Fprintf(&b, "\ntests: %d failed, %d passed, %d skipped\n", t.FailCount, t.PassCount, t.SkipCount)
		for _, c := range t.Failed {
			fmt.Fprintf(&b, "  %s %s.%s\n", c.Status, c.ClassName, c.Name)
		}
	}
	if len(v.Changes) > 0 {
		fmt.Fprintf(&b, "\nchanges (%d):\n", v.ChangesTotal)
		for _, c := range v.Changes {
			id := c.CommitID
			if len(id) > 8 {
				id = id[:8]
			}
			fmt.Fprintf(&b, "  %s %s (%s)\n", id, c.Message, c.Author)
		}
	}
	for _, w := range v.Warnings {
		fmt.Fprintf(&b, "\nwarning: %s\n", w)
	}
	return b.String()
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

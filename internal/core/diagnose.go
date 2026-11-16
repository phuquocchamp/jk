package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

// Defaults of build_diagnose.
const (
	DefaultDiagnoseLogLines = 60
	DefaultDiagnoseTests    = 10
	diagnoseMaxChanges      = 10
	changeMessageMaxRunes   = 200
)

// BuildDiagnose answers "why did this build fail" in one call: the build
// summary, the first failing stage and step with the end of that step's log,
// failed tests and SCM changes. Parts that cannot be read become warnings
// instead of failing the whole call.
func (s *Service) BuildDiagnose(ctx context.Context, in BuildDiagnoseIn) (BuildDiagnoseOut, error) {
	path, urlRef, err := s.target(OpBuildDiag, in.Job)
	if err != nil {
		return BuildDiagnoseOut{}, err
	}
	if err := nonNegative("log_lines", in.LogLines); err != nil {
		return BuildDiagnoseOut{}, err
	}
	if err := nonNegative("max_tests", in.MaxTests); err != nil {
		return BuildDiagnoseOut{}, err
	}
	ref := in.Ref
	if ref == "" && urlRef == "" {
		ref = RefLastFailed
	}
	if ref, err = pickRef(ref, urlRef, false); err != nil {
		return BuildDiagnoseOut{}, err
	}
	logLines := in.LogLines
	if logLines == 0 {
		logLines = DefaultDiagnoseLogLines
	}
	want, _ := s.policy.ClampLines(logLines)
	maxTests := in.MaxTests
	if maxTests == 0 {
		maxTests = DefaultDiagnoseTests
	}

	b, err := s.backend.GetBuild(ctx, path, ref)
	if err != nil {
		return BuildDiagnoseOut{}, err
	}
	n := b.Number
	out := BuildDiagnoseOut{
		Job: path, Number: n, Result: b.Result, Building: b.Building, URL: b.URL,
		DurationMs: b.DurationMs, Causes: b.Causes, Revision: b.Revision, ChangesTotal: len(b.Changes),
	}
	for i, c := range b.Changes {
		if i == diagnoseMaxChanges {
			break
		}
		c.Message = trimText(firstLineRaw(c.Message), changeMessageMaxRunes)
		out.Changes = append(out.Changes, c)
	}
	if b.Result == "SUCCESS" && !b.Building {
		return out, nil
	}
	warn := func(part string, err error) {
		out.Warnings = append(out.Warnings, part+": "+errs.From(err).Message)
	}
	clean := newCleaner(false)

	v, serr := s.loadStages(ctx, path, n, true)
	switch {
	case serr == nil && len(v.out.Stages) > 0:
		out.StageSource = v.out.Source
		for _, st := range v.out.Stages {
			if st.FailedStep != nil {
				out.FailedStages = append(out.FailedStages, st.Name)
			}
		}
		if ff := v.out.FirstFailed; ff != "" {
			var st Stage
			for _, x := range v.out.Stages {
				if x.ID == ff {
					st = x
				}
			}
			f := &Failure{StageID: st.ID, Stage: st.Name, Step: st.FailedStep, LogSource: "stage"}
			var ids []string
			if st.FailedStep != nil {
				ids, f.LogSource = []string{st.FailedStep.ID}, "step"
			} else if ids, err = s.stageNodeIDs(ctx, path, n, v, st); err != nil {
				warn("stage steps", err)
			}
			if len(ids) > 0 {
				if nt, err := s.tailNodes(ctx, path, n, ids, want, clean, f.LogSource == "step"); err != nil {
					warn("step log", err)
				} else {
					fillLog(f, nt.lines, nt.total)
					f.Highlights = nt.highlights
				}
			}
			out.Failure = f
		}
	case serr != nil && !errs.Is(serr, errs.CodeNotFound):
		warn("stages", serr)
	}
	if out.Failure == nil && b.Result != "" && b.Result != "SUCCESS" {
		// Not a Pipeline, or it failed outside any stage: use the console.
		f := &Failure{LogSource: "console"}
		if t, _, err := s.tailConsole(ctx, path, n, want, clean); err != nil {
			warn("console log", err)
		} else {
			fillLog(f, t.lines(), t.total)
		}
		out.Failure = f
	}

	if tr, err := s.failedTests(ctx, path, n, maxTests); err != nil {
		warn("tests", err)
	} else if tr.HasReport {
		out.Tests = &TestSummary{HasReport: true, FailCount: tr.FailCount, PassCount: tr.PassCount,
			SkipCount: tr.SkipCount, Failed: tr.Failed, Truncated: tr.Truncated}
	}

	if f := out.Failure; f != nil {
		if f.StageID != "" {
			out.Next = append(out.Next,
				fmt.Sprintf("build_log job=%s ref=%d stage=%s lines=500 (whole stage, more context)", path, n, f.StageID))
		}
		out.Next = append(out.Next,
			fmt.Sprintf("log_search job=%s ref=%d pattern=\"(?i)error|exception|failed\" context=3", path, n))
	}
	if out.Tests != nil && out.Tests.FailCount > len(out.Tests.Failed) {
		out.Next = append(out.Next, fmt.Sprintf("test_failed job=%s ref=%d limit=%d", path, n, out.Tests.FailCount))
	}
	return out, nil
}

// fillLog sets the log excerpt, without blank lines at either edge.
func fillLog(f *Failure, lines []string, total int) {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = cutLine(l, maxMatchRunes)
	}
	f.Log = strings.Join(lines, "\n")
	f.LogLines = len(lines)
	f.LogTruncated = total > len(lines)
}

// firstLineRaw returns the first non-empty line of s, untrimmed in length.
func firstLineRaw(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

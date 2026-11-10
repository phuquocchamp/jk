package core

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

func (s *Service) Whoami(ctx context.Context, _ WhoamiIn) (WhoamiOut, error) {
	if _, err := s.checkOp(OpWhoami); err != nil {
		return WhoamiOut{}, err
	}
	u, err := s.backend.Whoami(ctx)
	if err != nil {
		return WhoamiOut{}, err
	}
	return WhoamiOut{ID: u.ID, FullName: u.FullName, URL: s.backend.BaseURL()}, nil
}

// JobList lists the direct children of a folder. Children hidden by the
// allow/deny globs are dropped before counting and limiting.
func (s *Service) JobList(ctx context.Context, in JobListIn) (JobListOut, error) {
	var folder string
	if strings.TrimSpace(in.Folder) == "" {
		if _, err := s.checkOp(OpJobList); err != nil {
			return JobListOut{}, err
		}
	} else {
		op, err := s.checkOp(OpJobList)
		if err != nil {
			return JobListOut{}, err
		}
		p, _, err := ParseJobInput(in.Folder)
		if err != nil {
			return JobListOut{}, err
		}
		// A folder on the way to an allowed job may be listed even when it is
		// not allowed itself (allow "team-a/app/**" lists "team-a").
		if !s.policy.AllowedOrAncestor(op.TargetKind, p) {
			return JobListOut{}, errs.Newf(errs.CodePolicyDenied, "%s %q is not allowed by policy", op.TargetKind, p).
				WithHint("the profile's allow/deny globs exclude this target")
		}
		folder = p
	}
	if err := nonNegative("limit", in.Limit); err != nil {
		return JobListOut{}, err
	}
	limit := in.Limit
	if limit == 0 {
		limit = DefaultListLimit
	}
	jobs, err := s.backend.ListJobs(ctx, folder)
	if err != nil {
		return JobListOut{}, err
	}
	visible := make([]JobSummary, 0, len(jobs))
	for _, j := range jobs {
		p := j.Path
		if p == "" {
			p = joinPath(folder, j.Name)
			j.Path = p
		}
		if s.policy.Allowed(TargetJob, p) || (j.Container && s.policy.AllowedOrAncestor(TargetJob, p)) {
			visible = append(visible, j)
		}
	}
	out := JobListOut{Folder: folder, Total: len(visible), Jobs: visible}
	if len(visible) > limit {
		out.Jobs = visible[:limit]
		out.Truncated = true
	}
	return out, nil
}

func joinPath(folder, name string) string {
	if folder == "" {
		return name
	}
	return folder + "/" + name
}

func (s *Service) JobGet(ctx context.Context, in JobGetIn) (JobGetOut, error) {
	path, _, err := s.target(OpJobGet, in.Job)
	if err != nil {
		return JobGetOut{}, err
	}
	j, err := s.backend.GetJob(ctx, path)
	if err != nil {
		return JobGetOut{}, err
	}
	if j.Path == "" {
		j.Path = path
	}
	return j, nil
}

func (s *Service) BuildGet(ctx context.Context, in BuildGetIn) (BuildGetOut, error) {
	path, ref, err := s.prepare(OpBuildGet, in.Job, in.Ref, false)
	if err != nil {
		return BuildGetOut{}, err
	}
	b, err := s.backend.GetBuild(ctx, path, ref)
	if err != nil {
		return BuildGetOut{}, err
	}
	if b.Job == "" {
		b.Job = path
	}
	return b, nil
}

func (s *Service) BuildStages(ctx context.Context, in BuildStagesIn) (BuildStagesOut, error) {
	path, ref, err := s.prepare(OpBuildStages, in.Job, in.Ref, false)
	if err != nil {
		return BuildStagesOut{}, err
	}
	n, err := s.number(ctx, path, ref)
	if err != nil {
		return BuildStagesOut{}, err
	}
	run, err := s.backend.GetRun(ctx, path, n)
	if err != nil {
		return BuildStagesOut{}, err
	}
	stages := run.Stages
	if stages == nil {
		stages = []Stage{}
	}
	return BuildStagesOut{Job: path, Number: n, Status: run.Status, DurationMs: run.DurationMs, Stages: stages}, nil
}

// TestFailed returns failed cases only, error details trimmed. A build with no
// report returns has_report=false and no error.
func (s *Service) TestFailed(ctx context.Context, in TestFailedIn) (TestFailedOut, error) {
	path, ref, err := s.prepare(OpTestFailed, in.Job, in.Ref, false)
	if err != nil {
		return TestFailedOut{}, err
	}
	if err := nonNegative("limit", in.Limit); err != nil {
		return TestFailedOut{}, err
	}
	limit := in.Limit
	if limit == 0 {
		limit = DefaultListLimit
	}
	n, err := s.number(ctx, path, ref)
	if err != nil {
		return TestFailedOut{}, err
	}
	rep, err := s.backend.GetTestReport(ctx, path, n)
	if err != nil {
		return TestFailedOut{}, err
	}
	out := TestFailedOut{
		Job: path, Number: n, HasReport: rep.HasReport,
		FailCount: rep.FailCount, PassCount: rep.PassCount, SkipCount: rep.SkipCount,
		Failed: []TestCase{},
	}
	for _, c := range rep.Failed {
		if c.Status != "" && c.Status != "FAILED" && c.Status != "REGRESSION" {
			continue
		}
		if len(out.Failed) == limit {
			out.Truncated = true
			break
		}
		c.ErrorDetails = trimText(c.ErrorDetails, ErrorDetailsMaxChars)
		out.Failed = append(out.Failed, c)
	}
	return out, nil
}

// trimText cuts s to at most max runes, marking the cut with "...".
func trimText(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "..."
}

func (s *Service) QueueGet(ctx context.Context, in QueueGetIn) (QueueGetOut, error) {
	if _, err := s.checkOp(OpQueueGet); err != nil {
		return QueueGetOut{}, err
	}
	if in.ID <= 0 {
		return QueueGetOut{}, errs.Usagef("invalid queue id %d", in.ID)
	}
	return s.backend.GetQueueItem(ctx, in.ID)
}

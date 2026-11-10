package mcpserver

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/errs"
)

// fakeBackend is a synthetic core.Backend; it never talks to a network.
type fakeBackend struct {
	stopped []int
}

var _ core.Backend = (*fakeBackend)(nil)

const fakeBase = "https://jenkins.example.com"

func (f *fakeBackend) BaseURL() string { return fakeBase }

func (f *fakeBackend) Whoami(context.Context) (core.User, error) {
	return core.User{}, errs.FromHTTPStatus(401, "")
}

func (f *fakeBackend) ListJobs(context.Context, string) ([]core.JobSummary, error) { return nil, nil }

func (f *fakeBackend) GetJob(_ context.Context, job string) (core.Job, error) {
	return core.Job{}, errs.NotFound("job " + job)
}

func (f *fakeBackend) ResolveBuild(_ context.Context, _ string, ref core.BuildRef) (int, error) {
	if n, ok := ref.Number(); ok {
		return n, nil
	}
	return 42, nil
}

func (f *fakeBackend) GetBuild(_ context.Context, job string, ref core.BuildRef) (core.Build, error) {
	n, ok := ref.Number()
	if !ok {
		n = 42
	}
	return core.Build{
		Job:        job,
		Number:     n,
		Result:     "FAILURE",
		DurationMs: 1234,
		Timestamp:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		URL:        fakeBase + "/job/team-a/job/app/42/",
		Causes:     []string{"Started by user Jane Doe"},
		Revision:   "0123456789abcdef0123456789abcdef01234567",
	}, nil
}

func (f *fakeBackend) GetRun(context.Context, string, int) (core.Run, error) { return core.Run{}, nil }

func (f *fakeBackend) GetStageNodes(context.Context, string, int, string) ([]core.StageNode, error) {
	return nil, nil
}

func (f *fakeBackend) GetNodeLog(context.Context, string, int, string) (core.NodeLog, error) {
	return core.NodeLog{}, nil
}

func (f *fakeBackend) StreamConsole(context.Context, string, int) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("line 1\nline 2\n")), nil
}

func (f *fakeBackend) GetTestReport(context.Context, string, int) (core.TestReport, error) {
	return core.TestReport{}, nil
}

func (f *fakeBackend) GetQueueItem(context.Context, int64) (core.QueueItem, error) {
	return core.QueueItem{}, nil
}

func (f *fakeBackend) TriggerBuild(context.Context, string, map[string]string, bool) (int64, error) {
	return 7, nil
}

func (f *fakeBackend) ReplayBuild(context.Context, string, int, string) (int64, error) { return 0, nil }

func (f *fakeBackend) StopBuild(_ context.Context, _ string, n int) error {
	f.stopped = append(f.stopped, n)
	return nil
}

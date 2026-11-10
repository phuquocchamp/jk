package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

// maxMatchRunes caps each line (match or context) returned by log_search.
const maxMatchRunes = 2000

// BuildLog returns the last N lines of the console log, or of one stage's log
// when Stage is set. The console is streamed through a ring buffer, so memory
// stays bounded by N lines.
func (s *Service) BuildLog(ctx context.Context, in BuildLogIn) (BuildLogOut, error) {
	path, ref, err := s.prepare(OpBuildLog, in.Job, in.Ref, false)
	if err != nil {
		return BuildLogOut{}, err
	}
	if err := nonNegative("lines", in.Lines); err != nil {
		return BuildLogOut{}, err
	}
	n, err := s.number(ctx, path, ref)
	if err != nil {
		return BuildLogOut{}, err
	}
	want, capped := s.policy.ClampLines(in.Lines)
	out := BuildLogOut{Job: path, Number: n}

	var src io.Reader
	hasMore := false
	if stage := strings.TrimSpace(in.Stage); stage != "" {
		text, name, more, err := s.stageLog(ctx, path, n, stage)
		if err != nil {
			return BuildLogOut{}, err
		}
		src, hasMore, out.Stage = strings.NewReader(text), more, name
	} else {
		rc, err := s.backend.StreamConsole(ctx, path, n)
		if err != nil {
			return BuildLogOut{}, err
		}
		defer rc.Close()
		src = rc
	}

	tail := newTail(want)
	if err := eachLine(src, func(l string) bool { tail.add(l); return true }); err != nil {
		return BuildLogOut{}, readErr(ctx, err)
	}
	lines := tail.lines()
	out.Lines = len(lines)
	out.TotalLines = tail.total
	out.Truncated = capped || hasMore || tail.total > len(lines)
	out.Text = strings.Join(lines, "\n")
	if len(out.Text) > SpillThreshold {
		p, err := s.spill(out.Text, spillPrefix(OpBuildLog, n))
		if err != nil {
			return BuildLogOut{}, err
		}
		out.SpilledTo, out.Text = p, ""
	}
	return out, nil
}

// stageLog concatenates the logs of all flow nodes of the stage matching sel
// (exact id first, then case-insensitive name). It returns the text, the stage
// name and whether Jenkins reported more text than it returned.
func (s *Service) stageLog(ctx context.Context, job string, n int, sel string) (string, string, bool, error) {
	run, err := s.backend.GetRun(ctx, job, n)
	if err != nil {
		return "", "", false, err
	}
	st, ok := findStage(run.Stages, sel)
	if !ok {
		names := make([]string, 0, len(run.Stages))
		for _, x := range run.Stages {
			names = append(names, x.Name)
		}
		e := errs.NotFound(fmt.Sprintf("stage %q not found in build %d", sel, n))
		if len(names) > 0 {
			e = e.WithHint("stages: " + strings.Join(names, ", "))
		} else {
			e = e.WithHint("the build has no pipeline stages; omit stage to read the console log")
		}
		return "", "", false, e
	}
	nodes, err := s.backend.GetStageNodes(ctx, job, n, st.ID)
	if err != nil {
		return "", "", false, err
	}
	if len(nodes) == 0 {
		nodes = []StageNode{{ID: st.ID, Name: st.Name}}
	}
	var b strings.Builder
	more := false
	for _, nd := range nodes {
		lg, err := s.backend.GetNodeLog(ctx, job, n, nd.ID)
		if err != nil {
			return "", "", false, err
		}
		more = more || lg.HasMore
		if lg.Text == "" {
			continue
		}
		b.WriteString(lg.Text)
		if !strings.HasSuffix(lg.Text, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String(), st.Name, more, nil
}

func findStage(stages []Stage, sel string) (Stage, bool) {
	for _, st := range stages {
		if st.ID == sel {
			return st, true
		}
	}
	for _, st := range stages {
		if strings.EqualFold(st.Name, sel) {
			return st, true
		}
	}
	return Stage{}, false
}

// LogSearch scans the console log line by line and stops as soon as
// max_matches (and their trailing context) are collected, or the policy line
// cap on output (matches + context) is reached.
func (s *Service) LogSearch(ctx context.Context, in LogSearchIn) (LogSearchOut, error) {
	path, ref, err := s.prepare(OpLogSearch, in.Job, in.Ref, false)
	if err != nil {
		return LogSearchOut{}, err
	}
	if in.Pattern == "" {
		return LogSearchOut{}, errs.Usage("pattern is required")
	}
	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return LogSearchOut{}, errs.Usagef("invalid pattern: %v", err).WithHint("use Go RE2 syntax, e.g. (?i)error|exception")
	}
	if err := nonNegative("context", in.Context); err != nil {
		return LogSearchOut{}, err
	}
	if err := nonNegative("max_matches", in.MaxMatches); err != nil {
		return LogSearchOut{}, err
	}
	ctxN := min(in.Context, MaxSearchContext)
	maxMatches := in.MaxMatches
	if maxMatches == 0 {
		maxMatches = DefaultMaxMatches
	}

	n, err := s.number(ctx, path, ref)
	if err != nil {
		return LogSearchOut{}, err
	}
	rc, err := s.backend.StreamConsole(ctx, path, n)
	if err != nil {
		return LogSearchOut{}, err
	}
	defer rc.Close()

	sc := newSearch(re, ctxN, maxMatches, s.policy.MaxLogLines())
	if err := eachLine(rc, sc.line); err != nil {
		return LogSearchOut{}, readErr(ctx, err)
	}
	out := LogSearchOut{
		Job: path, Number: n, Pattern: in.Pattern,
		Matches: sc.matches, TotalLines: sc.lineNo,
		Truncated: sc.stopped || sc.capped,
	}
	if out.Matches == nil {
		out.Matches = []LogMatch{}
	}
	if matchesSize(out.Matches) > SpillThreshold {
		p, err := s.spill(grepText(out.Matches), spillPrefix(OpLogSearch, n))
		if err != nil {
			return LogSearchOut{}, err
		}
		out.SpilledTo = p
		for i := range out.Matches {
			out.Matches[i].Before, out.Matches[i].After = nil, nil
			out.Matches[i].Text = cutLine(out.Matches[i].Text, 200)
		}
	}
	return out, nil
}

// search holds the scan state of LogSearch.
type search struct {
	re         *regexp.Regexp
	ctxN       int
	maxMatches int
	budget     int // max output lines (matches + context)

	lineNo    int
	lastMatch int // line number of the previous match
	emitted   int
	before    *tailBuf
	beforeNos *ringInts
	matches   []LogMatch
	pending   []int // indexes of matches still collecting After
	stopped   bool  // scan ended before EOF
	capped    bool  // output line budget reached
}

func newSearch(re *regexp.Regexp, ctxN, maxMatches, budget int) *search {
	return &search{re: re, ctxN: ctxN, maxMatches: maxMatches, budget: budget,
		before: newTail(ctxN), beforeNos: &ringInts{n: ctxN}}
}

func (sc *search) done() bool { return len(sc.matches) >= sc.maxMatches || sc.capped }

func (sc *search) line(l string) bool {
	if sc.done() && len(sc.pending) == 0 {
		sc.stopped = true
		return false
	}
	sc.lineNo++
	text := cutLine(l, maxMatchRunes)

	// Trailing context for earlier matches.
	keep := sc.pending[:0]
	for _, i := range sc.pending {
		if sc.emitted >= sc.budget {
			sc.capped = true
			continue
		}
		m := &sc.matches[i]
		m.After = append(m.After, text)
		sc.emitted++
		if len(m.After) < sc.ctxN {
			keep = append(keep, i)
		}
	}
	sc.pending = keep

	if !sc.done() && sc.re.MatchString(l) {
		if sc.emitted >= sc.budget {
			sc.capped = true
		} else {
			// Leading context, without lines already shown with the previous match.
			prev := sc.before.lines()
			nos := sc.beforeNos.values()
			var before []string
			for i, no := range nos {
				if no > sc.lastMatch {
					before = prev[i:]
					break
				}
			}
			if room := sc.budget - sc.emitted - 1; len(before) > room {
				before = before[len(before)-room:]
			}
			sc.matches = append(sc.matches, LogMatch{Line: sc.lineNo, Text: text, Before: before})
			sc.emitted += 1 + len(before)
			sc.lastMatch = sc.lineNo
			if sc.ctxN > 0 {
				sc.pending = append(sc.pending, len(sc.matches)-1)
			}
		}
	}
	sc.before.add(text)
	sc.beforeNos.add(sc.lineNo)
	return true
}

func matchesSize(ms []LogMatch) int {
	n := 0
	for _, m := range ms {
		n += len(m.Text) + 16
		for _, l := range m.Before {
			n += len(l) + 3
		}
		for _, l := range m.After {
			n += len(l) + 3
		}
	}
	return n
}

// grepText renders matches like "grep -n": "N:match", "N-context", "--" between groups.
func grepText(ms []LogMatch) string {
	var b strings.Builder
	for i, m := range ms {
		if i > 0 && (len(m.Before) > 0 || len(ms[i-1].After) > 0) {
			b.WriteString("--\n")
		}
		start := m.Line - len(m.Before)
		for j, l := range m.Before {
			b.WriteString(strconv.Itoa(start+j) + "-" + l + "\n")
		}
		b.WriteString(strconv.Itoa(m.Line) + ":" + m.Text + "\n")
		for j, l := range m.After {
			b.WriteString(strconv.Itoa(m.Line+1+j) + "-" + l + "\n")
		}
	}
	return b.String()
}

func spillPrefix(op string, n int) string { return fmt.Sprintf("%s-%d", op, n) }

// readErr maps a stream read failure: context errors via errs.From, anything
// else as a retryable upstream error.
func readErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return errs.From(ctx.Err())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errs.From(err)
	}
	return errs.Wrap(errs.CodeUpstream, err, "reading build log failed").WithRetryable(true)
}

// cutLine limits a log line to max runes without trimming whitespace.
func cutLine(s string, max int) string {
	if len(s) <= max {
		return s
	}
	i := 0
	for pos := range s {
		if i == max {
			return s[:pos] + "..."
		}
		i++
	}
	return s
}

// eachLine calls fn for every line of r without the trailing "\n" or "\r\n".
// Lines longer than maxLineBytes are cut instead of failing the scan. A final
// empty fragment after the last "\n" is not a line. fn returning false stops.
func eachLine(r io.Reader, fn func(string) bool) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		if room := maxLineBytes - len(buf); room > 0 {
			buf = append(buf, chunk[:min(len(chunk), room)]...)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if err == nil || len(buf) > 0 {
			line := buf
			if n := len(line); n > 0 && line[n-1] == '\n' {
				line = line[:n-1]
			}
			if n := len(line); n > 0 && line[n-1] == '\r' {
				line = line[:n-1]
			}
			if !fn(string(line)) {
				return nil
			}
		}
		if err != nil { // io.EOF
			return nil
		}
		buf = buf[:0]
	}
}

// tailBuf keeps the last n strings added.
type tailBuf struct {
	buf   []string
	next  int
	n     int
	total int
}

func newTail(n int) *tailBuf { return &tailBuf{n: n} }

func (t *tailBuf) add(s string) {
	t.total++
	if t.n <= 0 {
		return
	}
	if len(t.buf) < t.n {
		t.buf = append(t.buf, s)
		return
	}
	t.buf[t.next] = s
	t.next = (t.next + 1) % t.n
}

// lines returns the kept strings in insertion order.
func (t *tailBuf) lines() []string {
	if len(t.buf) < t.n || t.next == 0 {
		return append([]string(nil), t.buf...)
	}
	out := make([]string, 0, len(t.buf))
	out = append(out, t.buf[t.next:]...)
	return append(out, t.buf[:t.next]...)
}

// ringInts keeps the last n ints (line numbers paired with tailBuf).
type ringInts struct {
	buf  []int
	next int
	n    int
}

func (r *ringInts) add(v int) {
	if r.n <= 0 {
		return
	}
	if len(r.buf) < r.n {
		r.buf = append(r.buf, v)
		return
	}
	r.buf[r.next] = v
	r.next = (r.next + 1) % r.n
}

func (r *ringInts) values() []int {
	if len(r.buf) < r.n || r.next == 0 {
		return r.buf
	}
	out := make([]int, 0, len(r.buf))
	out = append(out, r.buf[r.next:]...)
	return append(out, r.buf[:r.next]...)
}

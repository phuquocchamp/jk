package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/phuquocchamp/jk/internal/kit/errs"
	"github.com/phuquocchamp/jk/internal/kit/output"
)

// maxMatchRunes caps each line (match or context) returned by log_search.
const maxMatchRunes = 2000

// BuildLog returns the last N lines of the console log, or of one stage's log
// when Stage is set. Logs are streamed through a ring buffer, so memory stays
// bounded by N lines. The console tail is read from a byte offset near the end
// when Jenkins reports the log size, so a large log is not downloaded whole.
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
	clean := newCleaner(in.Raw)
	out := BuildLogOut{Job: path, Number: n, Stripped: clean.stripped()}

	var (
		lines []string
		total int
		full  = true
	)
	step := strings.TrimSpace(in.Step)
	if stage := strings.TrimSpace(in.Stage); stage != "" || step != "" {
		st, ids, err := s.logTarget(ctx, path, n, stage, step)
		if err != nil {
			return BuildLogOut{}, err
		}
		nt, err := s.tailNodes(ctx, path, n, ids, want, clean, false)
		if err != nil {
			return BuildLogOut{}, err
		}
		lines, total, full = nt.lines, nt.total, nt.full
		out.Stage, out.Source = st.Name, "steps"
		if step != "" {
			out.Source, out.StepID = "step", ids[0]
			if st.FailedStep != nil && st.FailedStep.ID == ids[0] {
				out.Step = st.FailedStep.Name
			}
		}
		out.StepsRead, out.StepsTotal = nt.read, len(ids)
	} else {
		t, info, err := s.tailConsole(ctx, path, n, want, clean)
		if err != nil {
			return BuildLogOut{}, err
		}
		lines, total = t.lines(), t.total
		out.Source, out.TotalBytes, full = info.source, info.totalBytes, info.full
		if !info.full {
			out.ReadBytes = info.readBytes
		}
	}
	out.Lines = len(lines)
	if full {
		out.TotalLines = total
	}
	out.Truncated = capped || !full || total > len(lines)
	out.Text = strings.Join(lines, "\n")
	if len(out.Text) > SpillThreshold {
		p, err := s.spill(out.Text, spillPrefix(OpBuildLog, n))
		if err != nil {
			return BuildLogOut{}, err
		}
		out.SpilledTo, out.SpilledURI, out.Text = p, output.SpillURI(p), ""
	}
	return out, nil
}

// logTarget resolves what a stage/step log request reads: the stage (by name
// or id; with only step set, the stage where the build first failed) and the
// flow node ids, in order. step "failed" selects the failing step of that
// stage (or of a stage nested in it); any other step value is a node id.
func (s *Service) logTarget(ctx context.Context, path string, n int, sel, step string) (Stage, []string, error) {
	v, err := s.loadStages(ctx, path, n, false)
	if err != nil {
		return Stage{}, nil, err
	}
	var st Stage
	if sel != "" {
		if st, err = selectStage(v.out.Stages, sel, n); err != nil {
			return Stage{}, nil, err
		}
	} else {
		i := slices.IndexFunc(v.out.Stages, func(x Stage) bool { return x.ID == v.out.FirstFailed })
		if i < 0 {
			return Stage{}, nil, errs.NotFound(fmt.Sprintf("build %d has no failed stage", n)).
				WithHint("pass stage to read a stage's log, or omit step to read the console")
		}
		st = v.out.Stages[i]
	}
	switch {
	case step == "":
		ids, err := s.stageNodeIDs(ctx, path, n, v, st)
		return st, ids, err
	case strings.EqualFold(step, "failed"):
		fs := st.FailedStep
		if fs == nil {
			// A parent or parallel branch: use the first failing nested stage.
			in := descendants(v.out.Stages, st.ID)
			if id := firstFailed(filterStages(v.out.Stages, in)); id != "" {
				j := slices.IndexFunc(v.out.Stages, func(x Stage) bool { return x.ID == id })
				st, fs = v.out.Stages[j], v.out.Stages[j].FailedStep
			}
		}
		if fs == nil {
			return Stage{}, nil, errs.NotFound(fmt.Sprintf("no failed step found in stage %q of build %d", st.Name, n)).
				WithHint("read the whole stage (omit step), or check build_stages for first_failed")
		}
		return st, []string{fs.ID}, nil
	default:
		return st, []string{step}, nil
	}
}

func filterStages(stages []Stage, keep map[string]bool) []Stage {
	var out []Stage
	for _, st := range stages {
		if keep[st.ID] {
			out = append(out, st)
		}
	}
	return out
}

// stageNodeIDs lists the flow nodes whose logs make up stage st.
func (s *Service) stageNodeIDs(ctx context.Context, path string, n int, v stageView, st Stage) ([]string, error) {
	if v.out.Source == SourcePGV && v.steps != nil {
		in := descendants(v.out.Stages, st.ID)
		var ids []string
		for _, step := range v.steps {
			if in[step.StageID] {
				ids = append(ids, step.ID)
			}
		}
		return ids, nil
	}
	nodes, err := s.backend.GetStageNodes(ctx, path, n, st.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(nodes))
	for _, nd := range nodes {
		ids = append(ids, nd.ID)
	}
	if len(ids) == 0 {
		ids = []string{st.ID}
	}
	return ids, nil
}

// nodeFetchConcurrency bounds parallel node log downloads.
const nodeFetchConcurrency = 4

// maxHighlights bounds the error-looking lines kept from a log.
const maxHighlights = 10

// highlightRE matches lines that usually carry the reason of a failure.
var highlightRE = regexp.MustCompile(`(?i)(\berror\b|\bfail(ed|ure)?\b|\bfatal\b|exception|exit code [1-9]|\bdenied\b|\brefused\b|\btimed out\b|\bnot (running|found)\b)`)

// nodeTail is the end of the concatenated logs of some flow nodes.
type nodeTail struct {
	lines      []string
	total      int  // lines in the nodes read
	full       bool // every node was read, so total is the whole length
	read       int  // nodes read
	highlights []LogMatch
}

type oneTail struct {
	lines      []string
	total      int
	highlights []LogMatch
	err        error
}

// tailNodes returns the last want lines of the logs of ids, in order. It reads
// from the last node backwards, nodeFetchConcurrency at a time, and stops once
// the nodes read hold want lines, so earlier steps are not downloaded. Each
// node keeps only its own last want lines, so memory stays bounded. With hl,
// error-looking lines are collected (line numbers count within each node).
func (s *Service) tailNodes(ctx context.Context, path string, n int, ids []string, want int, clean cleaner, hl bool) (nodeTail, error) {
	res := make([]oneTail, len(ids))
	start, have := len(ids), 0
	for start > 0 && have < want {
		lo := max(0, start-nodeFetchConcurrency)
		var wg sync.WaitGroup
		for i := lo; i < start; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				res[i] = s.readNode(ctx, path, n, ids[i], want, clean, hl)
			}(i)
		}
		wg.Wait()
		for i := lo; i < start; i++ {
			if res[i].err != nil {
				return nodeTail{}, res[i].err
			}
			have += res[i].total
		}
		start = lo
	}
	out := nodeTail{full: start == 0, read: len(ids) - start, total: have}
	var all []string
	for _, r := range res[start:] {
		all = append(all, r.lines...)
		out.highlights = append(out.highlights, r.highlights...)
	}
	if len(all) > want {
		all = all[len(all)-want:]
	}
	if len(out.highlights) > maxHighlights {
		out.highlights = out.highlights[len(out.highlights)-maxHighlights:]
	}
	out.lines = all
	return out, nil
}

func (s *Service) readNode(ctx context.Context, path string, n int, id string, want int, clean cleaner, hl bool) oneTail {
	rc, err := s.backend.StreamNodeLog(ctx, path, n, id)
	if err != nil {
		return oneTail{err: err}
	}
	defer rc.Close()
	t := newTail(want)
	var hls []LogMatch
	err = eachLine(rc, func(l string) bool {
		l = clean.line(l)
		t.add(l)
		if hl && highlightRE.MatchString(l) {
			hls = append(hls, LogMatch{Line: t.total, Text: cutLine(l, maxMatchRunes)})
			if len(hls) > 2*maxHighlights {
				hls = hls[len(hls)-maxHighlights:]
			}
		}
		return true
	})
	if err != nil {
		return oneTail{err: readErr(ctx, err)}
	}
	if len(hls) > maxHighlights {
		hls = hls[len(hls)-maxHighlights:]
	}
	return oneTail{lines: t.lines(), total: t.total, highlights: hls}
}

// consoleInfo describes how the console tail was read.
type consoleInfo struct {
	source     string
	full       bool // the whole log was read, so the line count is total
	totalBytes int64
	readBytes  int64
}

// maxTailWindow bounds the offset reads before falling back to the full log.
const maxTailWindow = 32 << 20

// tailConsole returns the last want lines of the console. With a known size it
// reads from an offset near the end (dropping the first, partial line) and
// widens the window until it has enough lines; otherwise, or when Jenkins
// does not support offsets, it streams consoleText from the start.
func (s *Service) tailConsole(ctx context.Context, path string, n, want int, clean cleaner) (*tailBuf, consoleInfo, error) {
	if size, err := s.backend.ConsoleSize(ctx, path, n); err == nil && size > 0 {
		window := max(int64(64<<10), int64(want)*512)
		for window <= maxTailWindow {
			start := max(0, size-window)
			rc, err := s.backend.StreamConsoleFrom(ctx, path, n, start)
			if err != nil {
				break
			}
			t := newTail(want)
			skip := start > 0
			err = eachLine(rc, func(l string) bool {
				if skip {
					skip = false
					return true
				}
				t.add(clean.line(l))
				return true
			})
			rc.Close()
			if err != nil {
				return nil, consoleInfo{}, readErr(ctx, err)
			}
			if start == 0 || t.total >= want {
				return t, consoleInfo{source: "progressiveText", full: start == 0, totalBytes: size, readBytes: size - start}, nil
			}
			window *= 4
		}
	}
	rc, err := s.backend.StreamConsole(ctx, path, n)
	if err != nil {
		return nil, consoleInfo{}, err
	}
	defer rc.Close()
	t := newTail(want)
	if err := eachLine(rc, func(l string) bool { t.add(clean.line(l)); return true }); err != nil {
		return nil, consoleInfo{}, readErr(ctx, err)
	}
	return t, consoleInfo{source: "consoleText", full: true}, nil
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
	clean := newCleaner(in.Raw)
	if err := eachLine(rc, func(l string) bool { return sc.line(clean.line(l)) }); err != nil {
		return LogSearchOut{}, readErr(ctx, err)
	}
	out := LogSearchOut{
		Job: path, Number: n, Pattern: in.Pattern,
		Matches: sc.matches, ScannedLines: sc.lineNo,
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
		out.SpilledTo, out.SpilledURI = p, output.SpillURI(p)
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

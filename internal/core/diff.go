package core

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// maxDiffLines caps the diff text in a replay plan.
	maxDiffLines = 200
	// maxLCSCells bounds the LCS table (about 16MB); larger changed regions
	// are shown as a whole-block replacement.
	maxLCSCells = 4_000_000
)

type diffResult struct {
	text           string
	added, removed int
	truncated      bool
}

type diffOp struct {
	kind byte // ' ', '-', '+'
	line string
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// unifiedDiff renders a line diff of a and b in unified format with ctxN
// lines of context, cut to maxLines lines.
func unifiedDiff(a, b, nameA, nameB string, ctxN, maxLines int) diffResult {
	ops := diffLines(splitLines(a), splitLines(b))
	var res diffResult
	for _, o := range ops {
		switch o.kind {
		case '-':
			res.removed++
		case '+':
			res.added++
		}
	}
	if res.added == 0 && res.removed == 0 {
		return res
	}
	// Line numbers (1-based) in a and b at each op.
	type pos struct{ a, b int }
	ps := make([]pos, len(ops)+1)
	ai, bi := 1, 1
	for k, o := range ops {
		ps[k] = pos{ai, bi}
		if o.kind != '+' {
			ai++
		}
		if o.kind != '-' {
			bi++
		}
	}
	ps[len(ops)] = pos{ai, bi}

	lines := []string{"--- " + nameA, "+++ " + nameB}
	for k := 0; k < len(ops); {
		for k < len(ops) && ops[k].kind == ' ' {
			k++
		}
		if k >= len(ops) {
			break
		}
		start, last := max(0, k-ctxN), k
		for j := k; j < len(ops); j++ {
			if ops[j].kind != ' ' {
				last = j
			} else if j-last > 2*ctxN {
				break
			}
		}
		end := min(len(ops), last+ctxN+1)
		aLen, bLen := 0, 0
		for _, o := range ops[start:end] {
			if o.kind != '+' {
				aLen++
			}
			if o.kind != '-' {
				bLen++
			}
		}
		lines = append(lines, fmt.Sprintf("@@ -%s +%s @@", hunkRange(ps[start].a, aLen), hunkRange(ps[start].b, bLen)))
		for _, o := range ops[start:end] {
			lines = append(lines, string(o.kind)+o.line)
		}
		k = end
	}
	if len(lines) > maxLines {
		lines, res.truncated = lines[:maxLines], true
	}
	res.text = strings.Join(lines, "\n")
	return res
}

func hunkRange(start, n int) string {
	switch n {
	case 0:
		return fmt.Sprintf("%d,0", start-1)
	case 1:
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d,%d", start, n)
}

// diffLines trims the common prefix and suffix, then diffs the middle by LCS.
func diffLines(a, b []string) []diffOp {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	sfx := 0
	for sfx < len(a)-p && sfx < len(b)-p && a[len(a)-1-sfx] == b[len(b)-1-sfx] {
		sfx++
	}
	ops := make([]diffOp, 0, len(a)+len(b))
	for _, l := range a[:p] {
		ops = append(ops, diffOp{' ', l})
	}
	ops = append(ops, lcsOps(a[p:len(a)-sfx], b[p:len(b)-sfx])...)
	for _, l := range a[len(a)-sfx:] {
		ops = append(ops, diffOp{' ', l})
	}
	return ops
}

func lcsOps(a, b []string) []diffOp {
	n, m := len(a), len(b)
	var ops []diffOp
	if n == 0 || m == 0 || n*m > maxLCSCells {
		for _, l := range a {
			ops = append(ops, diffOp{'-', l})
		}
		for _, l := range b {
			ops = append(ops, diffOp{'+', l})
		}
		return ops
	}
	w := m + 1
	dp := make([]int32, (n+1)*w) // dp[i*w+j] = LCS length of a[i:] and b[j:]
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				dp[i*w+j] = dp[(i+1)*w+j+1] + 1
			case dp[(i+1)*w+j] >= dp[i*w+j+1]:
				dp[i*w+j] = dp[(i+1)*w+j]
			default:
				dp[i*w+j] = dp[i*w+j+1]
			}
		}
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case dp[(i+1)*w+j] >= dp[i*w+j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}

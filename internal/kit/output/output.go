// Package output renders command results as JSON (agents) or tables (humans),
// with optional top-level field projection and jq filtering.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/itchyny/gojq"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

// AppName names the spill directory under os.TempDir(). Set once by main.
var AppName = "jk"

// Opts control rendering.
type Opts struct {
	JSON   bool     // JSON output; the CLI sets this when --json or stdout is not a TTY
	Fields []string // project top-level keys (and per item for lists / list fields)
	JQ     string   // gojq expression applied after projection; implies JSON
}

// Tabular can be implemented by result types to control human (table) output.
// Durations should be human-readable here (JSON keeps milliseconds).
type Tabular interface {
	TableHeader() []string
	TableRows() [][]string
}

// Texter can be implemented by results whose human form is plain text (e.g. logs).
type Texter interface {
	PlainText() string
}

// Render writes v to w according to o.
//
// JSON mode without Fields/JQ encodes v directly (struct field order kept).
// Otherwise v is converted to a generic JSON value, projected by Fields, and
// filtered by JQ (each jq result is written as one JSON document). Human mode
// uses Texter, then Tabular, then a generic key/value and table renderer in
// which numeric keys ending in "_ms" are shown as durations.
func Render(w io.Writer, v any, o Opts) error {
	fields := cleanFields(o.Fields)
	var query *gojq.Query
	if o.JQ != "" {
		q, err := gojq.Parse(o.JQ)
		if err != nil {
			return errs.Wrap(errs.CodeUsage, err, "invalid --jq expression: "+err.Error()).
				WithHint("check the jq syntax and wrap the expression in single quotes, e.g. --jq '[.jobs[] | .name]'")
		}
		query = q
	}

	if query == nil && len(fields) == 0 {
		if o.JSON {
			return writeJSON(w, v)
		}
		if t, ok := v.(Texter); ok {
			return writeText(w, t.PlainText())
		}
		if t, ok := v.(Tabular); ok {
			return writeTable(w, t.TableHeader(), t.TableRows())
		}
	}

	g, err := toGeneric(v)
	if err != nil {
		return err
	}
	if len(fields) > 0 {
		g = Project(g, fields)
	}
	if query != nil {
		return runJQ(w, query, g)
	}
	if o.JSON {
		return writeJSON(w, g)
	}
	return writeHuman(w, g)
}

func cleanFields(in []string) []string {
	var out []string
	for _, f := range in {
		for _, p := range strings.Split(f, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func toGeneric(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, err, "encode result")
	}
	var g any
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, err, "decode result")
	}
	return g, nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func runJQ(w io.Writer, q *gojq.Query, v any) error {
	iter := q.Run(v)
	for {
		r, ok := iter.Next()
		if !ok {
			return nil
		}
		if err, ok := r.(error); ok {
			// "halt" yields a HaltError with a nil value: a normal stop.
			if he, ok := err.(*gojq.HaltError); ok && he.Value() == nil {
				return nil
			}
			return errs.Wrap(errs.CodeUsage, err, "--jq: "+err.Error())
		}
		if err := writeJSON(w, r); err != nil {
			return err
		}
	}
}

// Project keeps only the given top-level keys of a JSON-like value. For arrays
// it projects each element. For an object, a value that is an array of objects
// whose elements carry any requested key is kept and projected per element, so
// fields "name,path" on {"jobs":[...]} keep the jobs with only name and path.
// Values that are not maps or arrays are returned unchanged.
func Project(v any, fields []string) any {
	if len(fields) == 0 {
		return v
	}
	want := make(map[string]bool, len(fields))
	for _, f := range fields {
		want[f] = true
	}
	return project(v, want, true)
}

func project(v any, want map[string]bool, top bool) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = project(e, want, false)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, val := range t {
			if want[k] {
				out[k] = val
				continue
			}
			if top {
				if arr, ok := val.([]any); ok && arrayHasKey(arr, want) {
					out[k] = project(arr, want, false)
				}
			}
		}
		return out
	}
	return v
}

func arrayHasKey(arr []any, want map[string]bool) bool {
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			return false
		}
		for k := range m {
			if want[k] {
				return true
			}
		}
	}
	return false
}

// ---- human rendering ----

func writeText(w io.Writer, s string) error {
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	_, err := io.WriteString(w, s)
	return err
}

func writeTable(w io.Writer, header []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if len(header) > 0 {
		fmt.Fprintln(tw, strings.Join(upper(header), "\t"))
	}
	for _, r := range rows {
		cells := make([]string, len(r))
		for i, c := range r {
			cells[i] = oneLine(c)
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

func upper(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToUpper(s)
	}
	return out
}

// writeHuman renders a generic JSON value: an object as "key: value" lines
// (arrays of objects become indented tables), an array of objects as a table,
// anything else as scalar lines.
func writeHuman(w io.Writer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		return writeObject(w, t)
	case []any:
		if isObjectArray(t) {
			h, rows := objectTable(t)
			return writeTable(w, h, rows)
		}
		for _, e := range t {
			fmt.Fprintln(w, scalar("", e))
		}
		return nil
	default:
		_, err := fmt.Fprintln(w, scalar("", v))
		return err
	}
}

func writeObject(w io.Writer, m map[string]any) error {
	keys := sortedKeys(m)
	var simple, nested []string
	for _, k := range keys {
		if arr, ok := m[k].([]any); ok && isObjectArray(arr) {
			nested = append(nested, k)
		} else {
			simple = append(simple, k)
		}
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, k := range simple {
		fmt.Fprintf(tw, "%s:\t%s\n", k, oneLine(scalar(k, m[k])))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, k := range nested {
		fmt.Fprintf(w, "\n%s:\n", k)
		h, rows := objectTable(m[k].([]any))
		var buf bytes.Buffer
		if err := writeTable(&buf, h, rows); err != nil {
			return err
		}
		for _, line := range strings.SplitAfter(buf.String(), "\n") {
			if line != "" {
				io.WriteString(w, "  "+line)
			}
		}
	}
	return nil
}

func isObjectArray(arr []any) bool {
	if len(arr) == 0 {
		return false
	}
	for _, e := range arr {
		if _, ok := e.(map[string]any); !ok {
			return false
		}
	}
	return true
}

// objectTable builds a table from objects; columns are the union of keys,
// each object's keys taken in sorted order, so output is deterministic.
func objectTable(arr []any) ([]string, [][]string) {
	seen := map[string]bool{}
	var header []string
	for _, e := range arr {
		for _, k := range sortedKeys(e.(map[string]any)) {
			if !seen[k] {
				seen[k] = true
				header = append(header, k)
			}
		}
	}
	rows := make([][]string, 0, len(arr))
	for _, e := range arr {
		m := e.(map[string]any)
		row := make([]string, len(header))
		for i, k := range header {
			if val, ok := m[k]; ok {
				row[i] = scalar(k, val)
			}
		}
		rows = append(rows, row)
	}
	return header, rows
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// scalar formats a generic value for humans. Numbers under keys ending in
// "_ms" are shown as durations.
func scalar(key string, v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		if strings.HasSuffix(key, "_ms") {
			return HumanDuration(int64(t))
		}
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = scalar("", e)
		}
		return strings.Join(parts, ", ")
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " | ")
	return strings.ReplaceAll(s, "\t", " ")
}

// HumanDuration formats milliseconds as a short duration, e.g. "1m23s", "450ms".
func HumanDuration(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Second:
		return d.String()
	case d < time.Minute:
		return d.Round(100 * time.Millisecond).String()
	case d < time.Hour:
		return d.Round(time.Second).String()
	default:
		return d.Round(time.Minute).String()
	}
}

// Spill writes content to os.TempDir()/<AppName>/<prefix>-<random>.txt and
// returns the path. The prefix is sanitized (path separators and other unsafe
// characters become '_'). Used when a result is too large to inline.
//
// The shared directory is only used when it is a real directory owned by the
// current user with no group/other permissions; otherwise (e.g. another user
// pre-created it in a shared /tmp) a fresh private os.MkdirTemp dir is used.
func Spill(content, prefix string) (string, error) {
	dir, err := spillDir()
	if err != nil {
		return "", errs.Wrap(errs.CodeInternal, err, "create spill dir")
	}
	f, err := os.CreateTemp(dir, sanitizePrefix(prefix)+"-*.txt")
	if err != nil {
		return "", errs.Wrap(errs.CodeInternal, err, "create spill file")
	}
	defer f.Close()
	if _, err := io.WriteString(f, content); err != nil {
		return "", errs.Wrap(errs.CodeInternal, err, "write spill file")
	}
	return f.Name(), nil
}

func spillDir() (string, error) {
	dir := filepath.Join(os.TempDir(), AppName)
	if err := os.MkdirAll(dir, 0o700); err == nil && privateDir(dir) {
		return dir, nil
	}
	return os.MkdirTemp(os.TempDir(), AppName+"-*")
}

// privateDir reports whether dir is a directory (not a symlink) owned by the
// current user with no group or other permission bits.
func privateDir(dir string) bool {
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return fi.Mode().Perm()&0o077 == 0 && ownedByMe(fi)
}

func sanitizePrefix(p string) string {
	var b strings.Builder
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), ".")
	if s == "" {
		s = "spill"
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

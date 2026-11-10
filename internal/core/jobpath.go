package core

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

// BuildRef identifies a build: a positive number or one of the permalinks.
type BuildRef string

const (
	RefLast           BuildRef = "last"
	RefLastFailed     BuildRef = "lastFailed"
	RefLastSuccessful BuildRef = "lastSuccessful"
	RefLastCompleted  BuildRef = "lastCompleted"
)

var refAliases = map[string]BuildRef{
	"":                    RefLast,
	"last":                RefLast,
	"lastbuild":           RefLast,
	"lastfailed":          RefLastFailed,
	"lastfailedbuild":     RefLastFailed,
	"lastsuccessful":      RefLastSuccessful,
	"lastsuccessfulbuild": RefLastSuccessful,
	"lastcompleted":       RefLastCompleted,
	"lastcompletedbuild":  RefLastCompleted,
}

// ParseBuildRef normalizes s; "" means last. Jenkins permalink names
// (lastBuild, lastFailedBuild, ...) are accepted case-insensitively.
func ParseBuildRef(s string) (BuildRef, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if n, err := strconv.Atoi(s); err == nil {
		if n <= 0 {
			return "", errs.Usagef("invalid build number %q", s)
		}
		return BuildRef(strconv.Itoa(n)), nil
	}
	if r, ok := refAliases[strings.ToLower(s)]; ok {
		return r, nil
	}
	return "", errs.Usagef("invalid build ref %q", s).
		WithHint("use a build number or last, lastFailed, lastSuccessful, lastCompleted")
}

// UnmarshalJSON accepts a string ("last", "42") or a JSON integer (42); null
// leaves r empty. Validation happens later in ParseBuildRef.
func (r *BuildRef) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case bytes.Equal(b, []byte("null")):
		return nil
	case len(b) > 0 && b[0] == '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*r = BuildRef(s)
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err != nil {
		return errs.Usagef("invalid build ref %s", b).
			WithHint("use a build number or last, lastFailed, lastSuccessful, lastCompleted")
	}
	*r = BuildRef(strconv.FormatInt(n, 10))
	return nil
}

// Number returns the build number when r is numeric.
func (r BuildRef) Number() (int, bool) {
	n, err := strconv.Atoi(string(r))
	return n, err == nil && n > 0
}

// Segment returns the URL path segment for r: "42", "lastBuild", "lastFailedBuild", ...
// r must be normalized by ParseBuildRef; "" maps to lastBuild.
func (r BuildRef) Segment() string {
	switch r {
	case "", RefLast:
		return "lastBuild"
	case RefLastFailed:
		return "lastFailedBuild"
	case RefLastSuccessful:
		return "lastSuccessfulBuild"
	case RefLastCompleted:
		return "lastCompletedBuild"
	}
	return string(r)
}

// NormalizeJobPath trims surrounding slashes/space and validates segments.
// The path uses "/" separators and the job names exactly as Jenkins stores them
// (a multibranch branch "feature/foo" is the job "feature%2Ffoo").
func NormalizeJobPath(s string) (string, error) {
	s = strings.Trim(strings.TrimSpace(s), "/")
	if s == "" {
		return "", errs.Usage("job path is empty")
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", errs.Usagef("invalid job path %q", s)
		}
	}
	return s, nil
}

// JobURLPath converts a normalized job path to its URL path:
// "a/b" -> "/job/a/job/b/". Each segment is url.PathEscape'd, so '%' becomes
// "%25" ("proj/feature%2Ffoo" -> "/job/proj/job/feature%252Ffoo/").
// The empty path returns "/". The result is an already-escaped path: build the
// request URL with url.Parse(base + JobURLPath(p)) and never unescape it.
func JobURLPath(path string) string {
	if path == "" {
		return "/"
	}
	var b strings.Builder
	for _, seg := range strings.Split(path, "/") {
		b.WriteString("/job/")
		b.WriteString(url.PathEscape(seg))
	}
	b.WriteString("/")
	return b.String()
}

// ParseJobInput accepts a job path, a "/job/a/job/b" path, or a full Jenkins URL
// (any context prefix, e.g. https://host/jenkins/job/a/job/b/42/console) and
// returns the job path plus the build ref found right after the last job
// segment ("" when none). URL segments are unescaped once, so
// ".../job/feature%252Ffoo/" yields the job name "feature%2Ffoo".
func ParseJobInput(s string) (string, BuildRef, error) {
	s = strings.TrimSpace(s)
	raw := s
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", "", errs.Usagef("invalid Jenkins URL %q", s)
		}
		raw = u.EscapedPath()
	} else if !strings.HasPrefix(strings.TrimLeft(s, "/"), "job/") {
		p, err := NormalizeJobPath(s)
		return p, "", err
	}

	segs := strings.Split(strings.Trim(raw, "/"), "/")
	var names []string
	var ref BuildRef
	for i := 0; i < len(segs); i++ {
		if segs[i] == "job" && i+1 < len(segs) {
			name, err := url.PathUnescape(segs[i+1])
			if err != nil {
				return "", "", errs.Usagef("invalid job segment %q", segs[i+1])
			}
			names = append(names, name)
			ref = ""
			i++
			continue
		}
		if len(names) > 0 && ref == "" {
			if r, err := ParseBuildRef(segs[i]); err == nil && segs[i] != "" {
				ref = r
			}
		}
		if len(names) > 0 && ref != "" {
			break
		}
	}
	if len(names) == 0 {
		return "", "", errs.Usagef("no /job/ segment in %q", s)
	}
	p, err := NormalizeJobPath(strings.Join(names, "/"))
	return p, ref, err
}

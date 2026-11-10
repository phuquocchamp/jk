// Package client is the Jenkins REST adapter implementing core.Backend.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/phuquocchamp/jk/internal/core"
	"github.com/phuquocchamp/jk/internal/kit/errs"
)

// DefaultTimeout bounds each non-streaming request.
const DefaultTimeout = 30 * time.Second

// Client talks to one Jenkins controller with HTTP basic auth (user:apitoken).
// It retries a POST once with a crumb (GET /crumbIssuer/api/json) on 403.
// The token is never logged.
type Client struct {
	baseURL   string
	user      string
	token     string
	http      *http.Client
	timeout   time.Duration
	userAgent string
	log       *slog.Logger
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets the underlying http.Client (tests pass httptest server clients).
// Its Timeout should be zero: a whole-request timeout would cut log streams;
// the client applies its own per-request timeout (WithTimeout) instead.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithTimeout sets the per-request timeout for non-streaming calls.
func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithLogger sets the debug logger (stderr only; never logs credentials).
func WithLogger(l *slog.Logger) Option { return func(c *Client) { c.log = l } }

// New creates a Client. baseURL may have a trailing slash; it is trimmed.
func New(baseURL, user, token string, opts ...Option) *Client {
	c := &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		user:      user,
		token:     token,
		http:      &http.Client{},
		timeout:   DefaultTimeout,
		userAgent: "jk",
		log:       slog.New(slog.DiscardHandler),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

var _ core.Backend = (*Client)(nil)

// BaseURL returns the configured Jenkins base URL (no trailing slash).
func (c *Client) BaseURL() string { return c.baseURL }

// ---------------------------------------------------------------------------
// Transport helpers
// ---------------------------------------------------------------------------

// endpoint builds an absolute URL from an already-escaped path (see
// core.JobURLPath) and an optional query. The path is never re-escaped.
func (c *Client) endpoint(escapedPath string, q url.Values) (*url.URL, error) {
	u, err := url.Parse(c.baseURL + escapedPath)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, errs.Usagef("invalid Jenkins URL %q", c.baseURL).
			WithHint("check the profile url, e.g. https://jenkins.example.com")
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	return u, nil
}

func treeQuery(tree string) url.Values {
	return url.Values{"tree": {tree}}
}

func (c *Client) newRequest(ctx context.Context, method string, u *url.URL, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, err, "build request")
	}
	if c.user != "" || c.token != "" {
		req.SetBasicAuth(c.user, c.token)
	}
	req.Header.Set("User-Agent", c.userAgent)
	return req, nil
}

// do sends req and logs method, path and status only (never headers).
func (c *Client) do(hc *http.Client, req *http.Request) (*http.Response, error) {
	resp, err := c.doRaw(hc, req)
	if err != nil {
		return nil, transportError(req, err)
	}
	return resp, nil
}

// doRaw is do without mapping the transport error.
func (c *Client) doRaw(hc *http.Client, req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := hc.Do(req)
	if err != nil {
		c.log.Debug("jenkins request failed", "method", req.Method, "path", req.URL.EscapedPath(), "err", errString(err))
		return nil, err
	}
	c.log.Debug("jenkins request", "method", req.Method, "path", req.URL.EscapedPath(),
		"status", resp.StatusCode, "elapsed", time.Since(start))
	return resp, nil
}

// neverSent reports whether a transport error happened before the request
// could reach the server (DNS or connect failure).
func neverSent(err error) bool {
	var oe *net.OpError
	return errors.As(err, &oe) && oe.Op == "dial"
}

// maybeAppliedHint is attached to errors of non-idempotent POSTs.
const maybeAppliedHint = "the request may already have been applied (e.g. the build may already be queued); check job_get/build_get before retrying"

// notRetryable clears Retryable on an UPSTREAM error of a non-idempotent
// request that may have reached Jenkins, so callers do not repeat it blindly.
func notRetryable(err error) error {
	if e, ok := errs.As(err); ok && e.Code == errs.CodeUpstream && e.Retryable {
		e.Retryable = false
		e.Hint = maybeAppliedHint
	}
	return err
}

// errString renders a transport error without the request URL.
func errString(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// transportError maps a client.Do failure. Timeouts and network errors are
// retryable UPSTREAM; cancellation by the caller is UPSTREAM, not retryable.
func transportError(req *http.Request, err error) error {
	what := req.Method + " " + req.URL.EscapedPath()
	ctxErr := req.Context().Err()
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctxErr, context.DeadlineExceeded):
		return errs.Wrap(errs.CodeUpstream, err, what+": request timed out").WithRetryable(true)
	case errors.Is(err, context.Canceled) || errors.Is(ctxErr, context.Canceled):
		return errs.Wrap(errs.CodeUpstream, err, what+": request canceled")
	}
	var ue *url.Error
	if errors.As(err, &ue) && ue.Timeout() {
		return errs.Wrap(errs.CodeUpstream, err, what+": request timed out").WithRetryable(true)
	}
	return errs.New(errs.CodeUpstream, what+": "+errString(err)).WithRetryable(true).
		WithHint("check the Jenkins URL and network/VPN connectivity")
}

// statusError maps a >= 400 response. The body is drained (bounded) and not
// included: Jenkins error pages are large HTML documents.
func statusError(resp *http.Response) *errs.Error {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	msg := fmt.Sprintf("%s %s: HTTP %d %s", resp.Request.Method, resp.Request.URL.EscapedPath(),
		resp.StatusCode, http.StatusText(resp.StatusCode))
	e := errs.FromHTTPStatus(resp.StatusCode, msg)
	if e != nil && e.Code == errs.CodeForbidden && e.Hint == "" {
		e.Hint = "the Jenkins user lacks the permission for this action"
	}
	return e
}

// jobNotFoundHint is attached to NOT_FOUND errors of job URLs.
const jobNotFoundHint = "check the job path (job_list shows valid paths); multibranch branch names containing / are written with %2F, e.g. project/feature%2Ffoo"

func withHint(err error, code errs.Code, hint string) error {
	if e, ok := errs.As(err); ok && e.Code == code && e.Hint == "" {
		e.Hint = hint
	}
	return err
}

// getJSON GETs path?query and decodes the JSON body into out, bounded by the
// per-request timeout (which covers reading the body).
func (c *Client) getJSON(ctx context.Context, escapedPath string, q url.Values, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	u, err := c.endpoint(escapedPath, q)
	if err != nil {
		return err
	}
	req, err := c.newRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.do(c.http, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return statusError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		if ctx.Err() != nil {
			return transportError(req, ctx.Err())
		}
		return errs.Wrap(errs.CodeUpstream, err,
			"GET "+u.EscapedPath()+": unexpected response (not Jenkins JSON)").
			WithHint("check the profile url; a login or proxy page may have been returned")
	}
	return nil
}

// postResult is what a POST returns to the caller.
type postResult struct {
	Status   int
	Location string
}

// noRedirect returns a shallow copy of c.http that does not follow redirects,
// so POST responses keep their Location header (and a 302 is not turned into a GET).
func (c *Client) noRedirect() *http.Client {
	hc := *c.http
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &hc
}

// postForm POSTs form values (may be empty). On 403 it fetches a crumb once
// and retries with the crumb header and the crumb response's cookies. When
// the crumb cannot be fetched the original FORBIDDEN is returned.
//
// With idempotent=false, timeouts, network errors after connecting and 5xx
// responses are returned with Retryable=false: Jenkins may have acted on the
// request already, so a blind retry could queue a second build.
func (c *Client) postForm(ctx context.Context, escapedPath string, form url.Values, idempotent bool) (postResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	u, err := c.endpoint(escapedPath, nil)
	if err != nil {
		return postResult{}, err
	}
	hc := c.noRedirect()
	body := form.Encode()
	send := func(crumb *crumbInfo) (*http.Response, error) {
		req, err := c.newRequest(ctx, http.MethodPost, u, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if crumb != nil {
			req.Header.Set(crumb.Field, crumb.Crumb)
			for _, ck := range crumb.cookies {
				req.AddCookie(ck)
			}
		}
		resp, err := c.doRaw(hc, req)
		if err != nil {
			mapped := transportError(req, err)
			if !idempotent && !neverSent(err) {
				mapped = notRetryable(mapped)
			}
			return nil, mapped
		}
		return resp, nil
	}

	resp, err := send(nil)
	if err != nil {
		return postResult{}, err
	}
	if resp.StatusCode == http.StatusForbidden {
		forbidden := statusError(resp)
		resp.Body.Close()
		crumb, cerr := c.fetchCrumb(ctx)
		if cerr != nil {
			c.log.Debug("crumb fetch failed", "err", errString(cerr))
			return postResult{}, forbidden
		}
		if resp, err = send(crumb); err != nil {
			return postResult{}, err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		e := statusError(resp)
		if !idempotent {
			return postResult{}, notRetryable(e)
		}
		return postResult{}, e
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return postResult{Status: resp.StatusCode, Location: resp.Header.Get("Location")}, nil
}

type crumbInfo struct {
	Field   string `json:"crumbRequestField"`
	Crumb   string `json:"crumb"`
	cookies []*http.Cookie
}

func (c *Client) fetchCrumb(ctx context.Context) (*crumbInfo, error) {
	u, err := c.endpoint("/crumbIssuer/api/json", nil)
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.do(c.http, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, statusError(resp)
	}
	var ci crumbInfo
	if err := json.NewDecoder(resp.Body).Decode(&ci); err != nil {
		return nil, errs.Wrap(errs.CodeUpstream, err, "invalid crumb response")
	}
	if ci.Field == "" || ci.Crumb == "" {
		return nil, errs.New(errs.CodeUpstream, "empty crumb response")
	}
	ci.cookies = resp.Cookies()
	return &ci, nil
}

var queueItemRE = regexp.MustCompile(`/queue/item/(\d+)`)

// parseQueueID extracts the queue id from a Location header (absolute or relative).
func parseQueueID(location string) (int64, bool) {
	m := queueItemRE.FindStringSubmatch(location)
	if m == nil {
		return 0, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	return id, err == nil && id > 0
}

// buildPath returns the escaped path of a build: /job/a/job/b/<seg>/
func buildPath(job, seg string) string {
	return core.JobURLPath(job) + url.PathEscape(seg) + "/"
}

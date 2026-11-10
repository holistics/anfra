package cloudapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/holistics/anfra/shared/logkit"
)

// eventRetry is a call tried again because the server asked to wait: a delay
// the caller may not see, so it leaves a record.
var eventRetry = logkit.DefineEvent("cloudapi.retry",
	"A call to the org's server was tried again after the server asked to wait.",
	"op", "code", "wait_seconds", "attempt")

// Client calls the ops `anfra publish` uses on one org: one call each, which
// the caller batches within the server's Limits.
type Client struct {
	org  Org
	opts ClientOptions
}

// ClientOptions are what a Client sends with every call.
type ClientOptions struct {
	// HTTP is the client to send with; nil means http.DefaultClient.
	HTTP *http.Client
	// Authorize resolves current credentials immediately before each request.
	// An error prevents dispatch; authentication failures are never retried.
	Authorize func(*http.Request) error
	// UserAgent names the client and its version, for a server to tell which
	// clients still call it.
	UserAgent string
	// Waiting is told before a call waits to be tried again, after the server
	// asked it to (rate_limited, unavailable), so a person can see why nothing
	// moves. nil says nothing.
	Waiting func(op string, wait time.Duration)
	// Logger records each retry (cloudapi.retry), for a caller who sees no
	// progress, such as an agent. nil records nothing.
	Logger *slog.Logger
}

// Retries: how often a call is tried again when the server asks it to wait,
// and the longest wait taken. A longer wait is the server saying not to come
// back soon: the call fails with the server's answer instead.
const (
	maxRetries = 3
	maxWait    = time.Minute
	// defaultWait is for a rate_limited answer that doesn't say how long.
	defaultWait = 5 * time.Second
)

// NewClient is a client of org.
func NewClient(org Org, opts ClientOptions) *Client {
	if opts.HTTP == nil {
		opts.HTTP = http.DefaultClient
	}
	client := *opts.HTTP
	// Credentials must never follow redirects, even to related hosts. A cookie
	// jar could otherwise combine browser sessions with the selected bearer.
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	opts.HTTP = &client
	return &Client{org: org, opts: opts}
}

// Error is the server's answer to a call that failed: its error body, and the
// HTTP status.
type Error struct {
	Status   int
	Response apperr.Response
}

func (e *Error) Error() string { return e.Response.Code + ": " + e.Response.Message }

// IsCode reports whether err is the server's answer with code.
func IsCode(err error, code string) bool {
	e, ok := errors.AsType[*Error](err)
	return ok && e.Response.Code == code
}

// IsForbidden reports the server refusing the caller the op.
func IsForbidden(err error) bool { return IsCode(err, CodeForbidden) }

// Missing is the blob ids a blobs_missing answer names; ok is false for any
// other error.
func Missing(err error) (oids []string, ok bool) {
	e, isServers := errors.AsType[*Error](err)
	if !isServers || e.Response.Code != CodeBlobsMissing {
		return nil, false
	}
	raw, err := jsonkit.Marshal(e.Response.Details)
	if err != nil {
		return nil, false
	}
	var d MissingBlobs
	if jsonkit.Unmarshal(raw, &d) != nil {
		return nil, false
	}
	return d.OIDs, true
}

// Outdated is the revision a base_outdated answer names, published since
// the publish's base; ok is false for any other error.
func Outdated(err error) (live LiveRevision, ok bool) {
	e, isServers := errors.AsType[*Error](err)
	if !isServers || e.Response.Code != CodeBaseOutdated {
		return LiveRevision{}, false
	}
	return details[LiveRevision](e)
}

// retryAfter is how long the server asked to wait before trying the call
// again, and whether it asked: rate_limited always, unavailable when it knows.
func retryAfter(err error) (time.Duration, bool) {
	e, ok := errors.AsType[*Error](err)
	if !ok {
		return 0, false
	}
	d, _ := details[RetryAfter](e)
	wait := time.Duration(d.Seconds) * time.Second
	switch {
	case e.Response.Code == CodeRateLimited && wait <= 0:
		return defaultWait, true
	case e.Response.Code == CodeRateLimited, e.Response.Code == CodeUnavailable && wait > 0:
		return wait, wait <= maxWait
	}
	return 0, false
}

func details[T any](e *Error) (T, bool) {
	var d T
	raw, err := jsonkit.Marshal(e.Response.Details)
	if err != nil || jsonkit.Unmarshal(raw, &d) != nil {
		return d, false
	}
	return d, true
}

// Status is the org's published revision, and the server's limits.
func (c *Client) Status(ctx context.Context) (StatusOut, error) {
	return call[StatusOut](ctx, c, OpStatus, struct{}{})
}

// ListMissingBlobs is those of oids the org's repo lacks, in the order given.
func (c *Client) ListMissingBlobs(ctx context.Context, oids []string) ([]string, error) {
	out, err := call[ListMissingBlobsOut](ctx, c, OpListMissingBlobs, ListMissingBlobsIn{OIDs: oids})
	return out.OIDs, err
}

// CreateBlobs uploads blobs and returns their ids, in order.
func (c *Client) CreateBlobs(ctx context.Context, blobs [][]byte) ([]string, error) {
	out, err := call[CreateBlobsOut](ctx, c, OpCreateBlobs, CreateBlobsIn{Blobs: blobs})
	return out.OIDs, err
}

// Publish publishes in.Files as the whole of the org's repo.
func (c *Client) Publish(ctx context.Context, in PublishIn) (PublishOut, error) {
	return call[PublishOut](ctx, c, OpPublish, in)
}

// ListFiles is a revision's files, as paths and blob ids.
func (c *Client) ListFiles(ctx context.Context, revision string) ([]File, error) {
	out, err := call[ListFilesOut](ctx, c, OpListFiles, ListFilesIn{Revision: revision})
	return out.Files, err
}

// GetBlobs reads blobs' content: one call, within limits.DownloadBlobs. An
// answer past limits.DownloadBytes leaves some out, in Omitted.
func (c *Client) GetBlobs(ctx context.Context, oids []string) (GetBlobsOut, error) {
	return call[GetBlobsOut](ctx, c, OpGetBlobs, GetBlobsIn{OIDs: oids})
}

// ReadBlobs reads every blob in oids, in calls within limits, asking again
// for what an answer left out.
func (c *Client) ReadBlobs(ctx context.Context, oids []string, limits Limits) (map[string][]byte, error) {
	out := make(map[string][]byte, len(oids))
	for batch := range slices.Chunk(oids, max(limits.DownloadBlobs, 1)) {
		for len(batch) > 0 {
			got, err := c.GetBlobs(ctx, batch)
			if err != nil {
				return nil, err
			}
			if len(got.Blobs) == 0 && len(got.Omitted) > 0 {
				// The server always answers the first blob named: one that
				// answers none would have this ask forever.
				return nil, errors.New("the server answered none of the blobs asked for")
			}
			for _, b := range got.Blobs {
				out[b.OID] = b.Content
			}
			batch = got.Omitted
		}
	}
	return out, nil
}

// ListDataSources is the org's data sources, by name and type.
func (c *Client) ListDataSources(ctx context.Context) ([]DataSource, error) {
	out, err := call[ListDataSourcesOut](ctx, c, OpListDataSources, struct{}{})
	return out.DataSources, err
}

// CreateDataSource adds a data source to the org, its connection sent to be kept
// sealed.
func (c *Client) CreateDataSource(ctx context.Context, in CreateDataSourceIn) (DataSource, error) {
	return call[DataSource](ctx, c, OpCreateDataSource, in)
}

// TestDataSource checks the server can connect to a data source.
func (c *Client) TestDataSource(ctx context.Context, in TestDataSourceIn) (TestDataSourceOut, error) {
	return call[TestDataSourceOut](ctx, c, OpTestDataSource, in)
}

// ListAppOrigins is the origins the org's Data Apps may load from beyond the SDK's own.
func (c *Client) ListAppOrigins(ctx context.Context) ([]string, error) {
	out, err := call[AppOriginsOut](ctx, c, OpListAppOrigins, struct{}{})
	return out.Origins, err
}

// call posts in to op and decodes its answer, ignoring members it does not know:
// a server may answer with more than this client was built for.
//
// When the server asks the caller to wait (rate_limited, or unavailable saying
// how long), call waits and tries again, up to maxRetries times. The server
// answers those before doing anything, so any op is safe to try again.
func call[Out, In any](ctx context.Context, c *Client, op string, in In) (Out, error) {
	var out Out
	body, err := jsonkit.Marshal(in)
	if err != nil {
		return out, err
	}
	for attempt := 0; ; attempt++ {
		out, err = callOnce[Out](ctx, c, op, body)
		wait, ok := retryAfter(err)
		if !ok || attempt == maxRetries {
			return out, err
		}
		if c.opts.Waiting != nil {
			c.opts.Waiting(op, wait)
		}
		if c.opts.Logger != nil {
			code := ""
			if e, ok := errors.AsType[*Error](err); ok {
				code = e.Response.Code
			}
			c.opts.Logger.InfoContext(ctx, "Retrying a call the server asked to wait", logkit.EventKey, eventRetry,
				"op", op, "code", code, "wait_seconds", int(wait.Seconds()), "attempt", attempt+1)
		}
		if err := sleep(ctx, wait); err != nil {
			return out, err
		}
	}
}

// sleep waits d, or until ctx is done. Tests replace it, to retry at once.
var sleep = func(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// callOnce posts body to op once.
func callOnce[Out any](ctx context.Context, c *Client, op string, body []byte) (Out, error) {
	var out Out
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.org.Endpoint(op), bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.opts.UserAgent != "" {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}
	if c.opts.Authorize != nil {
		if err := c.opts.Authorize(req); err != nil {
			return out, err
		}
	}
	resp, err := c.opts.HTTP.Do(req)
	if err != nil {
		return out, clierr.Unreachablef(clierr.Org, err, "reach %s: %v", c.org, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return out, clierr.Unreachablef(clierr.Org, err, "The connection to %s broke before its reply arrived. Check your connection, and try again.", c.org)
	}
	if resp.StatusCode != http.StatusOK {
		var env apperr.Envelope
		if jsonkit.Unmarshal(raw, &env) != nil || env.Error.Code == "" {
			return out, clierr.Unreachablef(clierr.Org, nil, "%s returned %s. Try again in a moment, and if it keeps failing, contact your org's admin.", c.org, resp.Status)
		}
		return out, &Error{Status: resp.StatusCode, Response: env.Error}
	}
	if err := jsonkit.Unmarshal(raw, &out); err != nil {
		return out, clierr.Unreachablef(clierr.Org, err, "decode %s's answer to %s: %v", c.org, op, err)
	}
	return out, nil
}

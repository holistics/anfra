package remoteapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// Client calls the publish ops of one org on an anfra remote server: one call
// each, which the caller batches within the server's Limits.
type Client struct {
	remote Remote
	opts   ClientOptions
}

// ClientOptions are what a Client sends with every call.
type ClientOptions struct {
	// HTTP is the client to send with; nil means http.DefaultClient.
	HTTP *http.Client
	// Authorize adds the caller's credential to a request.
	Authorize func(*http.Request)
	// UserAgent names the client and its version, for a server to tell which
	// clients still call it.
	UserAgent string
}

// NewClient is a client of r.
func NewClient(r Remote, opts ClientOptions) *Client {
	if opts.HTTP == nil {
		opts.HTTP = http.DefaultClient
	}
	return &Client{remote: r, opts: opts}
}

// Error is an anfra remote server's answer to a call that failed: its error body, and the
// HTTP status.
type Error struct {
	Status   int
	Response apperr.Response
}

func (e *Error) Error() string { return e.Response.Code + ": " + e.Response.Message }

// IsCode reports whether err is an anfra remote server's answer with code c.
func IsCode(err error, c apperr.AnyCode) bool {
	e, ok := errors.AsType[*Error](err)
	return ok && e.Response.Code == c.Code().Public().String()
}

// IsForbidden reports a server refusing the caller the op: a server's own code,
// forbidden, which every anfra server answers a caller lacking the permission
// with.
func IsForbidden(err error) bool {
	e, ok := errors.AsType[*Error](err)
	return ok && e.Response.Code == "forbidden"
}

// Missing is the blob ids a blobs_missing answer names; ok is false for any
// other error.
func Missing(err error) (oids []string, ok bool) {
	e, isRemote := errors.AsType[*Error](err)
	if !isRemote || e.Response.Code != BlobsMissing.Code().String() {
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

// Status is the org's published revision, and the server's limits.
func (c *Client) Status(ctx context.Context) (StatusOut, error) {
	return call[StatusOut](ctx, c, StatusOp.Name, struct{}{})
}

// ListMissingBlobs is those of oids the org's repo lacks, in the order given.
func (c *Client) ListMissingBlobs(ctx context.Context, oids []string) ([]string, error) {
	out, err := call[ListMissingBlobsOut](ctx, c, ListMissingBlobsOp.Name, ListMissingBlobsIn{OIDs: oids})
	return out.OIDs, err
}

// CreateBlobs uploads blobs and returns their ids, in order.
func (c *Client) CreateBlobs(ctx context.Context, blobs [][]byte) ([]string, error) {
	out, err := call[CreateBlobsOut](ctx, c, CreateBlobsOp.Name, CreateBlobsIn{Blobs: blobs})
	return out.OIDs, err
}

// Publish publishes in.Files as the whole of the org's repo.
func (c *Client) Publish(ctx context.Context, in PublishIn) (PublishOut, error) {
	return call[PublishOut](ctx, c, PublishOp.Name, in)
}

// ListDataSources is the org's data sources, by name and type.
func (c *Client) ListDataSources(ctx context.Context) ([]DataSource, error) {
	out, err := call[ListDataSourcesOut](ctx, c, ListDataSourcesOp.Name, struct{}{})
	return out.DataSources, err
}

// CreateDataSource adds a data source to the org, its connection sent to be kept
// sealed.
func (c *Client) CreateDataSource(ctx context.Context, in CreateDataSourceIn) (DataSource, error) {
	return call[DataSource](ctx, c, CreateDataSourceOp.Name, in)
}

// TestDataSource checks the server can connect to a data source.
func (c *Client) TestDataSource(ctx context.Context, in TestDataSourceIn) (TestDataSourceOut, error) {
	return call[TestDataSourceOut](ctx, c, TestDataSourceOp.Name, in)
}

// call posts in to op and decodes its answer, ignoring members it does not know:
// a server may answer with more than this client was built for.
func call[Out, In any](ctx context.Context, c *Client, op string, in In) (Out, error) {
	var out Out
	body, err := jsonkit.Marshal(in)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.remote.Endpoint(op), bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.opts.UserAgent != "" {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}
	if c.opts.Authorize != nil {
		c.opts.Authorize(req)
	}
	resp, err := c.opts.HTTP.Do(req)
	if err != nil {
		return out, fmt.Errorf("reach %s: %w", c.remote, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return out, fmt.Errorf("read %s's answer to %s: %w", c.remote, op, err)
	}
	if resp.StatusCode != http.StatusOK {
		var env apperr.Envelope
		if jsonkit.Unmarshal(raw, &env) != nil || env.Error.Code == "" {
			return out, fmt.Errorf("%s answered %s with %s, not an anfra error: is it an anfra server?", c.remote, op, resp.Status)
		}
		return out, &Error{Status: resp.StatusCode, Response: env.Error}
	}
	if err := jsonkit.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode %s's answer to %s: %w", c.remote, op, err)
	}
	return out, nil
}

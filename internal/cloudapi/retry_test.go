package cloudapi_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/shared/jsonkit"
)

// newOrgServer serves h as an org's API, and answers a client of it.
func newOrgServer(t *testing.T, h http.HandlerFunc, opts cloudapi.ClientOptions) *cloudapi.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	org, err := cloudapi.ParseOrg(srv.URL + "/o/acme")
	if err != nil {
		t.Fatal(err)
	}
	return cloudapi.NewClient(org, opts)
}

func refuse(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// A call the server asks to wait is tried again after the wait asked for,
// and says so first; credentials are resolved again for each try.
func TestRetryWhenAskedToWait(t *testing.T) {
	waits := cloudapi.NoWait(t.Cleanup)
	calls, authorized := 0, 0
	var told []time.Duration
	var log bytes.Buffer
	c := newOrgServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			refuse(w, 429, `{"error":{"code":"rate_limited","scope":"client","status":429,"message":"Too many requests.","request_id":"r","details":{"retry_after_seconds":12}}}`)
		case 2:
			refuse(w, 503, `{"error":{"code":"unavailable","scope":"server","status":503,"message":"Another publish is in progress.","request_id":"r","details":{"retry_after_seconds":2}}}`)
		default:
			_, _ = w.Write([]byte(`{"files":[{"path":"a.aml","oid":"x"}]}`))
		}
	}, cloudapi.ClientOptions{
		Authorize: func(*http.Request) error { authorized++; return nil },
		Waiting:   func(_ string, d time.Duration) { told = append(told, d) },
		Logger:    slog.New(slog.NewJSONHandler(&log, nil)),
	})
	files, err := c.ListFiles(context.Background(), "acme@1")
	if err != nil || len(files) != 1 {
		t.Fatalf("%v, %v", files, err)
	}
	want := []time.Duration{12 * time.Second, 2 * time.Second}
	if !slices.Equal(*waits, want) || !slices.Equal(told, want) {
		t.Errorf("waited %v, told %v; want %v", *waits, told, want)
	}
	if authorized != 3 {
		t.Errorf("credentials resolved %d times, want 3", authorized)
	}
	// Each retry leaves a record, for whoever saw no progress.
	records := strings.Split(strings.TrimSpace(log.String()), "\n")
	if len(records) != 2 ||
		!strings.Contains(records[0], `"event":"cloudapi.retry"`) || !strings.Contains(records[0], `"code":"rate_limited"`) || !strings.Contains(records[0], `"wait_seconds":12`) ||
		!strings.Contains(records[1], `"code":"unavailable"`) || !strings.Contains(records[1], `"attempt":2`) {
		t.Errorf("log records:\n%s", log.String())
	}
}

// After maxRetries, or for a wait too long, or for an error that isn't the
// server asking to wait, the call fails with the server's answer.
func TestRetryGivesUp(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
		calls      int
	}{
		{"still rate limited", `{"error":{"code":"rate_limited","scope":"client","status":429,"message":"m","request_id":"r","details":{"retry_after_seconds":1}}}`, 429, 4},
		{"asked to wait too long", `{"error":{"code":"rate_limited","scope":"client","status":429,"message":"m","request_id":"r","details":{"retry_after_seconds":3600}}}`, 429, 1},
		{"unavailable without a wait", `{"error":{"code":"unavailable","scope":"server","status":503,"message":"m","request_id":"r"}}`, 503, 1},
		{"another error", `{"error":{"code":"forbidden","scope":"client","status":403,"message":"m","request_id":"r"}}`, 403, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cloudapi.NoWait(t.Cleanup)
			calls := 0
			c := newOrgServer(t, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				refuse(w, tc.status, tc.body)
			}, cloudapi.ClientOptions{})
			if _, err := c.Status(context.Background()); err == nil {
				t.Fatal("succeeded")
			}
			if calls != tc.calls {
				t.Errorf("%d calls, want %d", calls, tc.calls)
			}
		})
	}
}

// ReadBlobs batches by the download limit, and asks again for what an answer
// left out.
func TestReadBlobs(t *testing.T) {
	var asked [][]string
	c := newOrgServer(t, func(w http.ResponseWriter, r *http.Request) {
		var in cloudapi.GetBlobsIn
		decode(t, r, &in)
		asked = append(asked, in.OIDs)
		// Answer the first, and leave the rest out, as a server past its
		// byte limit does.
		body := `{"blobs":[{"oid":"` + in.OIDs[0] + `","content":"` + b64(in.OIDs[0]) + `"}],"omitted":` + jsonList(in.OIDs[1:]) + `}`
		_, _ = w.Write([]byte(body))
	}, cloudapi.ClientOptions{})
	got, err := c.ReadBlobs(context.Background(), []string{"a", "b", "c"}, cloudapi.Limits{DownloadBlobs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || string(got["c"]) != "c" {
		t.Errorf("got %q", got)
	}
	want := [][]string{{"a", "b"}, {"b"}, {"c"}}
	if !slices.EqualFunc(asked, want, slices.Equal) {
		t.Errorf("asked %v, want %v", asked, want)
	}
}

// Outdated reads the published revision from a base_outdated answer.
func TestOutdated(t *testing.T) {
	c := newOrgServer(t, func(w http.ResponseWriter, _ *http.Request) {
		refuse(w, 409, `{"error":{"code":"base_outdated","scope":"client","status":409,"message":"m","request_id":"r","details":{"revision":"acme@2","published_by":{"id":"u","email":"bob@acme.com"},"published_at":"2026-11-08T16:20:00Z"}}}`)
	}, cloudapi.ClientOptions{})
	_, err := c.Publish(context.Background(), cloudapi.PublishIn{Base: "acme@1"})
	live, ok := cloudapi.Outdated(err)
	if !ok || live.Revision != "acme@2" || live.PublishedBy == nil || live.PublishedBy.Email != "bob@acme.com" {
		t.Errorf("%+v, %v from %v", live, ok, err)
	}
	if _, ok := cloudapi.Outdated(nil); ok {
		t.Error("nil is outdated")
	}
}

func decode(t *testing.T, r *http.Request, v any) {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil || jsonkit.Unmarshal(raw, v) != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func jsonList(s []string) string {
	raw, _ := jsonkit.Marshal(s)
	return string(raw)
}

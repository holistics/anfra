package remoteapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/holistics/anfra/shared/remoteapi"
)

func TestParseRemote(t *testing.T) {
	for raw, want := range map[string]remoteapi.Remote{
		"https://bi.example.com/o/acme":      {Origin: "https://bi.example.com", Org: "acme"},
		"https://bi.example.com:8443/o/a-1/": {Origin: "https://bi.example.com:8443", Org: "a-1"},
		"http://localhost:8080/o/acme":       {Origin: "http://localhost:8080", Org: "acme"},
		"http://127.0.0.1:8080/o/acme":       {Origin: "http://127.0.0.1:8080", Org: "acme"},
	} {
		got, err := remoteapi.ParseRemote(raw)
		if err != nil || got != want {
			t.Errorf("%q: %+v, %v; want %+v", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"bi.example.com/o/acme",               // no scheme
		"ftp://bi.example.com/o/acme",         // not http(s)
		"https://bi.example.com",              // no org
		"https://bi.example.com/o/",           // empty org
		"https://bi.example.com/o/Acme",       // not a slug
		"https://bi.example.com/o/acme/x",     // more than the org's address
		"https://bi.example.com/o/acme?x=1",   // a query
		"https://u:p@bi.example.com/o/acme",   // a credential in the address
		"http://bi.example.com/o/acme",        // http, not loopback: the credential in clear
		"https://bi.example.com/api/o/acme/x", // an endpoint, not the org's address
	} {
		if _, err := remoteapi.ParseRemote(raw); err == nil {
			t.Errorf("%q: accepted", raw)
		}
	}
	if got, err := remoteapi.ParseRemote("  https://bi.example.com/o/acme\n"); err != nil || got.Org != "acme" {
		t.Errorf("an address pasted with spaces around it: %+v, %v", got, err)
	}
	r, _ := remoteapi.ParseRemote("https://bi.example.com/o/acme")
	if r.String() != "https://bi.example.com/o/acme" || r.Endpoint("repos.status") != "https://bi.example.com/api/o/acme/repos.status" {
		t.Errorf("%s, %s", r, r.Endpoint("repos.status"))
	}
}

// BlobID is git's own name for the content, as hash-object computes it with no
// filters.
func TestBlobIDIsGits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	for _, content := range []string{"", "Model orders {}\n", "a\r\nb\r\n", strings.Repeat("x", 100000)} {
		cmd := exec.Command("git", "hash-object", "--no-filters", "--stdin")
		cmd.Stdin = strings.NewReader(content)
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := remoteapi.BlobID([]byte(content)), strings.TrimSpace(string(out)); got != want {
			t.Errorf("BlobID(%d bytes) = %s, git says %s", len(content), got, want)
		}
	}
}

// The client posts to the org's endpoint with its credential and user agent,
// reads answers carrying members it does not know, and returns refusals as
// errors IsCode and Missing read.
func TestClient(t *testing.T) {
	var seen *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/o/acme/repos.list_missing_blobs":
			_, _ = w.Write([]byte(`{"oids":["b"],"added_later":true}`))
		case "/api/o/acme/repos.publish":
			w.WriteHeader(http.StatusConflict)
			_ = jsonkit.MarshalWrite(w, apperr.Envelope{Error: apperr.Response{Code: "blobs_missing", Message: "m", Details: remoteapi.MissingBlobs{OIDs: []string{"c"}}}})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<html>not found</html>"))
		}
	}))
	defer srv.Close()
	r, err := remoteapi.ParseRemote(srv.URL + "/o/acme")
	if err != nil {
		t.Fatal(err)
	}
	c := remoteapi.NewClient(r, remoteapi.ClientOptions{
		UserAgent: "anfra/test",
		Authorize: func(req *http.Request) { req.Header.Set("Authorization", "Bearer t") },
	})
	ctx := context.Background()

	missing, err := c.ListMissingBlobs(ctx, []string{"a", "b"})
	if err != nil || !slices.Equal(missing, []string{"b"}) {
		t.Errorf("missing = %v, %v: an unknown member must not fail an old client", missing, err)
	}
	if seen.Header.Get("Authorization") != "Bearer t" || seen.Header.Get("User-Agent") != "anfra/test" || seen.Method != http.MethodPost {
		t.Errorf("request: %s %v", seen.Method, seen.Header)
	}

	_, err = c.Publish(ctx, remoteapi.PublishIn{})
	if oids, ok := remoteapi.Missing(err); !ok || !slices.Equal(oids, []string{"c"}) || !remoteapi.IsCode(err, remoteapi.BlobsMissing) {
		t.Errorf("err = %v, missing %v %v", err, oids, ok)
	}
	if remoteapi.IsCode(err, remoteapi.RepoInvalid) {
		t.Error("blobs_missing read as repo_invalid")
	}

	_, err = c.Status(ctx)
	if err == nil || !strings.Contains(err.Error(), "not an anfra error") {
		t.Errorf("a non-anfra answer: %v", err)
	}
}

func TestSpec(t *testing.T) {
	spec, err := remoteapi.Spec()
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range remoteapi.Ops {
		if !strings.Contains(string(spec), "/"+o.Name+":") {
			t.Errorf("the spec has no %s", o.Name)
		}
	}
}

// A publish carries the repo's AML and Data Apps, and nothing else.
func TestPublishable(t *testing.T) {
	for path, want := range map[string]bool{
		"models/orders.aml":                 true,
		"datasets/sales.AML":                true,
		"orders.aml":                        true,
		"apps/dash/index.html":              true,
		"apps/dash/data/rows.json":          true,
		"apps":                              false, // a file named apps
		".anfra/data_sources.yml":           false,
		".anfra/context_sources.yml":        false,
		".git/config":                       false,
		"lib/.git":                          false,
		".hidden/x.aml":                     false,
		"models/.draft.aml":                 false,
		"apps/dash/node_modules/x/index.js": false,
		"apps/dash/.cache/x":                false,
		".env":                              false,
		"README.md":                         false,
		"scripts/build.sh":                  false,
		"models/orders.aml.bak":             false,
		"":                                  false,
		"models//a.aml":                     false,
	} {
		if got := remoteapi.Publishable(path); got != want {
			t.Errorf("Publishable(%q) = %v, want %v", path, got, want)
		}
	}
}

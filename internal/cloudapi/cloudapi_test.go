package cloudapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

func TestParseOrg(t *testing.T) {
	for raw, want := range map[string]cloudapi.Org{
		"https://bi.example.com/o/acme":      {Origin: "https://bi.example.com", Slug: "acme"},
		"https://bi.example.com:8443/o/a-1/": {Origin: "https://bi.example.com:8443", Slug: "a-1"},
		"https://BI.Example.com:443/o/acme":  {Origin: "https://bi.example.com", Slug: "acme"},
		"http://[::1]:80/o/acme":             {Origin: "http://[::1]", Slug: "acme"},
		"http://127.0.0.1:8080/o/acme":       {Origin: "http://127.0.0.1:8080", Slug: "acme"},
	} {
		got, err := cloudapi.ParseOrg(raw)
		if err != nil || got != want {
			t.Errorf("%q: %+v, %v; want %+v", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"http://localhost:8080/o/acme",
		"https://::1/o/acme",
		"https://[fe80::1%25eth0]/o/acme",
		"https://bad_host/o/acme",
		"http://127.0.0.2/o/acme",
		"https://bi.example.com/o/a-",
		"https://bi.example.com/o/" + strings.Repeat("a", 64),
		"https://bi.example.com/o/acme%2fx",
		"https://bi.example.com/o/%61cme",
		"https://bi.example.com:/o/acme",
		"https://bi.example.com:65536/o/acme",
		"https://bi.example.com/o/acme?",
		"https://bi.example.com/o/acme#",
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
		if _, err := cloudapi.ParseOrg(raw); err == nil {
			t.Errorf("%q: accepted", raw)
		}
	}
	if got, err := cloudapi.ParseOrg("  https://bi.example.com/o/acme\n"); err != nil || got.Slug != "acme" {
		t.Errorf("an address pasted with spaces around it: %+v, %v", got, err)
	}
	r, _ := cloudapi.ParseOrg("https://bi.example.com/o/acme")
	if r.String() != "https://bi.example.com/o/acme" || r.Endpoint("repos.status") != "https://bi.example.com/api/o/acme/repos.status" {
		t.Errorf("%s, %s", r, r.Endpoint("repos.status"))
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
			_ = jsonkit.MarshalWrite(w, apperr.Envelope{Error: apperr.Response{Code: "blobs_missing", Message: "m", Details: cloudapi.MissingBlobs{OIDs: []string{"c"}}}})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<html>not found</html>"))
		}
	}))
	defer srv.Close()
	r, err := cloudapi.ParseOrg(srv.URL + "/o/acme")
	if err != nil {
		t.Fatal(err)
	}
	c := cloudapi.NewClient(r, cloudapi.ClientOptions{
		UserAgent: "anfra/test",
		Authorize: func(req *http.Request) error { req.Header.Set("Authorization", "Bearer t"); return nil },
	})
	ctx := context.Background()

	missing, err := c.ListMissingBlobs(ctx, []string{"a", "b"})
	if err != nil || !slices.Equal(missing, []string{"b"}) {
		t.Errorf("missing = %v, %v: an unknown member must not fail an old client", missing, err)
	}
	if seen.Header.Get("Authorization") != "Bearer t" || seen.Header.Get("User-Agent") != "anfra/test" || seen.Method != http.MethodPost {
		t.Errorf("request: %s %v", seen.Method, seen.Header)
	}

	_, err = c.Publish(ctx, cloudapi.PublishIn{})
	if oids, ok := cloudapi.Missing(err); !ok || !slices.Equal(oids, []string{"c"}) || !cloudapi.IsCode(err, cloudapi.CodeBlobsMissing) {
		t.Errorf("err = %v, missing %v %v", err, oids, ok)
	}
	if cloudapi.IsCode(err, cloudapi.CodeRepoInvalid) {
		t.Error("blobs_missing read as repo_invalid")
	}

	_, err = c.Status(ctx)
	if err == nil || !strings.Contains(err.Error(), "not an anfra error") {
		t.Errorf("a non-anfra answer: %v", err)
	}
}

func TestClientDoesNotForwardBearerOnRedirect(t *testing.T) {
	var followed bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed = true }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-token" || r.Header.Get("Cookie") != "" {
			t.Error("wrong source credentials")
		}
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	org, err := cloudapi.ParseOrg(source.URL + "/o/acme")
	if err != nil {
		t.Fatal(err)
	}
	client := cloudapi.NewClient(org, cloudapi.ClientOptions{Authorize: func(r *http.Request) error { r.Header.Set("Authorization", "Bearer local-token"); return nil }})
	if _, err := client.Status(context.Background()); err == nil {
		t.Fatal("redirect accepted")
	}
	if followed {
		t.Fatal("bearer request followed redirect")
	}
}

func TestSlowPublishUsesCallerDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(31 * time.Second):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"revision":"repo@slow","changed":true}`))
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	org, err := cloudapi.ParseOrg(server.URL + "/o/acme")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	out, err := cloudapi.NewClient(org, cloudapi.ClientOptions{}).Publish(ctx, cloudapi.PublishIn{})
	if err != nil || out.Revision != "repo@slow" {
		t.Fatalf("slow publish: %v %v", out, err)
	}
}

func TestClientStopsBeforeDispatchOnCredentialFailure(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer server.Close()
	org, err := cloudapi.ParseOrg(server.URL + "/o/acme")
	if err != nil {
		t.Fatal(err)
	}
	expected := errors.New("credential refresh uncertain")
	client := cloudapi.NewClient(org, cloudapi.ClientOptions{Authorize: func(*http.Request) error { return expected }})
	if _, err := client.Status(context.Background()); !errors.Is(err, expected) {
		t.Fatalf("credential failure lost: %v", err)
	}
	if called {
		t.Fatal("API request dispatched despite unavailable credentials")
	}
}

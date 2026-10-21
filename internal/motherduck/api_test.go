package motherduck

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/jsonkit"
)

// The org's token is a read-scaling one, which never expires (no ttl), made
// with the person's own token; its secret is answered, and no error carries it.
func TestCreateReadToken(t *testing.T) {
	var body map[string]any
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = jsonkit.Unmarshal(b, &body)
		_, _ = io.WriteString(w, `{"id":"t1","token":"secret","token_type":"read_scaling","read_only":true}`)
	}))
	defer srv.Close()
	api := API{BaseURL: srv.URL}
	tok, err := api.CreateReadToken(context.Background(), "session", "ada b", "anfra acme", "for acme")
	if err != nil || tok != (Token{ID: "t1", Name: "anfra acme", Secret: "secret"}) {
		t.Fatalf("token %+v, %v", tok, err)
	}
	if auth != "Bearer session" || path != "/v1/users/ada b/tokens" {
		t.Errorf("auth %q, path %q", auth, path)
	}
	if body["token_type"] != "read_scaling" || body["name"] != "anfra acme" || body["description"] != "for acme" {
		t.Errorf("body %v", body)
	}
	if _, ok := body["ttl"]; ok {
		t.Errorf("an expiring token for the org: %v", body)
	}
}

// A token MotherDuck makes otherwise than asked, one that may write, is revoked
// at once and never answered.
func TestCreateReadTokenRevokesAWritingOne(t *testing.T) {
	var revoked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			revoked = r.URL.Path
			_, _ = io.WriteString(w, `{}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"t1","token":"secret","token_type":"read_write"}`)
	}))
	defer srv.Close()
	tok, err := API{BaseURL: srv.URL}.CreateReadToken(context.Background(), "session", "ada", "n", "d")
	if err == nil || tok.Secret != "" || revoked != "/v1/users/ada/tokens/t1" {
		t.Errorf("token %+v, err %v, revoked %q", tok, err, revoked)
	}
}

func TestAPIErrorsSayMotherDucksMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"Not allowed","code":"FORBIDDEN"}`)
	}))
	defer srv.Close()
	_, err := API{BaseURL: srv.URL}.CreateReadToken(context.Background(), "session-secret", "ada", "n", "d")
	if err == nil || !strings.Contains(err.Error(), "403: Not allowed") || strings.Contains(err.Error(), "session-secret") {
		t.Errorf("err %v", err)
	}
}

// A name another token has is told apart from other refusals.
func TestNameTaken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"message":"A token with that name already exists"}`)
	}))
	defer srv.Close()
	_, err := API{BaseURL: srv.URL}.CreateReadToken(context.Background(), "session", "ada", "anfra acme", "d")
	if !NameTaken(err) || !strings.Contains(err.Error(), "409: A token with that name already exists") {
		t.Errorf("err %v", err)
	}
	if NameTaken(&APIError{Status: http.StatusForbidden}) {
		t.Error("a refusal taken for a name taken")
	}
}

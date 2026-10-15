package cliauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"github.com/holistics/anfra/shared/jsonkit"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/cloudapi"
)

func protocolToken(origin string) tokenResponse {
	var response tokenResponse
	response.AccessToken = "new-access"
	response.RefreshToken = "new-refresh"
	response.TokenType = "Bearer"
	response.ExpiresIn = 3600
	response.Scope = "all"
	response.Resource = origin + "/api"
	response.Grant.ID = "11111111-1111-4111-8111-111111111111"
	response.Grant.Org.ID = "22222222-2222-4222-8222-222222222222"
	response.Grant.Org.Slug = "selected"
	response.Grant.Org.URL = origin + "/o/selected"
	response.Grant.ExpiresAt = time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	return response
}

func discoveryHandler(w http.ResponseWriter, r *http.Request, origin string) bool {
	switch r.URL.Path {
	case "/.well-known/oauth-protected-resource/api":
		_ = jsonkit.MarshalWrite(w, resourceMetadata{Resource: origin + "/api", Servers: []string{origin}, Scopes: []string{"all"}, BearerMethods: []string{"header"}})
	case "/.well-known/oauth-authorization-server":
		_ = jsonkit.MarshalWrite(w, serverMetadata{Issuer: origin, AuthorizationEndpoint: origin + "/oauth/authorize", TokenEndpoint: origin + "/oauth/token", RevocationEndpoint: origin + "/oauth/revoke", ResponseTypes: []string{"code"}, GrantTypes: []string{"authorization_code", "refresh_token"}, TokenAuth: []string{"none"}, RevocationAuth: []string{"none"}, Scopes: []string{"all"}, Challenges: []string{"S256"}, ResponseIssuer: true})
	default:
		return false
	}
	return true
}

func TestBrowserLoginProtocol(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	var origin string
	var exchanges atomic.Int32
	challenge := make(chan string, 1)
	exchangeObserved := make(chan struct{})
	var response tokenResponse
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("OAuth request carried credentials")
		}
		if discoveryHandler(w, r, origin) {
			return
		}
		if r.URL.Path != "/oauth/token" {
			t.Error("unexpected path")
			http.NotFound(w, r)
			return
		}
		exchanges.Add(1)
		close(exchangeObserved)
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Method != "POST" || r.Form.Get("client_id") != "anfra-cli" || r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("resource") != origin+"/api" || r.Form.Get("code") != "callback-code" || len(r.Form.Get("code_verifier")) < 43 {
			t.Error("invalid exchange")
		}
		digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(digest[:]) != <-challenge {
			t.Error("exchange verifier does not match S256 authorization challenge")
		}
		_ = jsonkit.MarshalWrite(w, response)
	}))
	defer server.Close()
	origin = server.URL
	response = protocolToken(origin)
	response.Grant.Org.Name = "Selected Analytics ทีม"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	org, err := (Manager{}).Login(ctx, origin, "hint", func(ctx context.Context, raw string) error {
		authorization, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := authorization.Query()
		challenge <- q.Get("code_challenge")
		if q.Get("org_hint") != "hint" || q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 || q.Get("resource") != origin+"/api" {
			t.Error("invalid authorization parameters")
		}
		callback := q.Get("redirect_uri")
		values := url.Values{"state": {q.Get("state")}, "iss": {origin}, "code": {"callback-code"}}
		invalid := []string{"state=wrong&iss=" + url.QueryEscape(origin) + "&code=callback-code", values.Encode() + "&state=duplicate", strings.Replace(values.Encode(), "iss=", "iss=wrong", 1)}
		for _, query := range invalid {
			res, err := http.Get(callback + "?" + query)
			if err != nil {
				return err
			}
			res.Body.Close()
			if res.StatusCode != 400 {
				t.Errorf("invalid callback status %d", res.StatusCode)
			}
		}
		res, err := http.Get(callback + "?" + values.Encode())
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("callback status %d", res.StatusCode)
		}
		// A browser launcher may stay alive after delivering the callback.
		// Code exchange must not wait for that process to exit.
		select {
		case <-exchangeObserved:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if org.String() != origin+"/o/selected" || exchanges.Load() != 1 {
		t.Fatalf("org=%v exchanges=%d", org, exchanges.Load())
	}
	logins, err := (Manager{}).StoredLogins(ctx)
	if err != nil || len(logins) != 1 || logins[0].Org != org || logins[0].Name != "Selected Analytics ทีม" || logins[0].LocalState != "stored" {
		t.Fatalf("inventory %v %v", logins, err)
	}
	info, err := os.Stat(filepath.Join(os.Getenv("ANFRA_HOME"), "credentials.json")) //nolint:gosec // G703: this test's isolated credential file
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
}

func TestBrowserLoginCancellation(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !discoveryHandler(w, r, origin) {
			t.Error("unexpected request after cancellation")
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	origin = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := (Manager{}).Login(ctx, origin, "", func(context.Context, string) error {
		cancel()
		return nil
	})
	if err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
	logins, err := (Manager{}).StoredLogins(context.Background())
	if err != nil || len(logins) != 0 {
		t.Fatalf("canceled login saved credentials: %v %v", logins, err)
	}
}

func TestCallbackBoundary(t *testing.T) {
	valid := "http://127.0.0.1:1234/oauth/callback?state=expected&iss=https%3A%2F%2Fexample.com&code=code"
	for _, test := range []struct{ name, method, address, host string }{
		{"post", "POST", valid, ""},
		{"wrong host", "GET", valid, "localhost:1234"},
		{"wrong path", "GET", strings.Replace(valid, "/oauth/callback", "/other", 1), ""},
		{"encoded path", "GET", strings.Replace(valid, "/oauth/callback", "/oauth/%63allback", 1), ""},
		{"duplicate code", "GET", valid + "&code=other", ""},
		{"both result kinds", "GET", valid + "&error=access_denied", ""},
		{"bad encoding", "GET", valid + "&bad=%zz", ""},
		{"query credential", "GET", valid + "&access_token=secret", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.address, nil)
			if test.host != "" {
				req.Host = test.host
			}
			if _, ok := validateOAuthCallback(req, "127.0.0.1:1234", "expected", "https://example.com"); ok {
				t.Fatal("accepted invalid callback")
			}
		})
	}
}

func TestGrantResponseBoundary(t *testing.T) {
	origin := "https://example.com"
	for _, test := range []struct {
		name   string
		change func(*tokenResponse)
	}{
		{"missing extension", func(r *tokenResponse) { r.Grant.ID = "" }},
		{"foreign org", func(r *tokenResponse) { r.Grant.Org.URL = "https://other.example/o/selected" }},
		{"slug mismatch", func(r *tokenResponse) { r.Grant.Org.Slug = "other" }},
		{"wrong resource", func(r *tokenResponse) { r.Resource = origin + "/mcp" }},
		{"wrong scope", func(r *tokenResponse) { r.Scope = "all extra" }},
		{"invalid UUID", func(r *tokenResponse) { r.Grant.Org.ID = "org" }},
		{"expired grant", func(r *tokenResponse) { r.Grant.ExpiresAt = time.Now().Add(-time.Second) }},
		{"missing refresh", func(r *tokenResponse) { r.RefreshToken = "" }},
		{"header injection", func(r *tokenResponse) { r.AccessToken = "bad\r\nheader" }},
		{"excess lifetime", func(r *tokenResponse) { r.ExpiresIn = 3601 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := protocolToken(origin)
			test.change(&r)
			if _, err := validateTokenResponse(r, origin, time.Now(), nil); err == nil {
				t.Fatal("accepted invalid token response")
			}
		})
	}
}

func TestDiscoveryRefusesRedirectAndForeignEndpoints(t *testing.T) {
	for _, redirect := range []bool{true, false} {
		t.Run(fmt.Sprint(redirect), func(t *testing.T) {
			var origin string
			var foreignCalls atomic.Int32
			foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignCalls.Add(1) }))
			defer foreign.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if redirect {
					http.Redirect(w, r, foreign.URL, http.StatusFound)
					return
				}
				if r.URL.Path == "/.well-known/oauth-authorization-server" {
					_ = jsonkit.MarshalWrite(w, serverMetadata{Issuer: origin, AuthorizationEndpoint: foreign.URL + "/authorize"})
					return
				}
				discoveryHandler(w, r, origin)
			}))
			defer server.Close()
			origin = server.URL
			if _, err := discoverOAuthServer(context.Background(), origin); err == nil {
				t.Fatal("accepted unsafe discovery")
			}
			if foreignCalls.Load() != 0 {
				t.Fatal("followed foreign URL")
			}
		})
	}
}

func seedCredential(t *testing.T, origin string, expired bool) storedCredential {
	t.Helper()
	response := protocolToken(origin)
	entry, err := validateTokenResponse(response, origin, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	entry.AccessToken = "old-access"
	entry.RefreshToken = "old-refresh"
	entry.TokenEndpoint = origin + "/oauth/token"
	entry.RevocationEndpoint = origin + "/oauth/revoke"
	if expired {
		entry.AccessExpiry = time.Now().Add(time.Minute)
	}
	err = withCredentialLock(context.Background(), func(inventory *credentialInventory, path string) error {
		inventory.Entries[credentialKey(entry.Org)] = entry
		return saveCredentialInventory(path, inventory)
	})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func assertAuthorizationToken(t *testing.T, authorize func(*http.Request) error, org cloudapi.Org, expected string) {
	t.Helper()
	req, _ := http.NewRequest("POST", org.Endpoint("repo.status"), nil) //nolint:gosec // G704: this test's validated local server
	if err := authorize(req); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "Bearer "+expected {
		t.Fatalf("header %q", req.Header.Get("Authorization"))
	}
}

func TestGrantLifetimeAllowsClockSkew(t *testing.T) {
	for _, skew := range []time.Duration{-5 * time.Second, 5 * time.Second, -2 * time.Minute, 2 * time.Minute, -time.Hour} {
		t.Run(skew.String(), func(t *testing.T) {
			now := time.Now().UTC()
			response := protocolToken("https://example.com")
			// Real new grants last 180 days on the server, not the 24 hours
			// used by the shorter protocol fixtures above.
			response.Grant.ExpiresAt = now.Add(180 * 24 * time.Hour)
			_, err := validateTokenResponse(response, "https://example.com", now.Add(skew), nil)
			if skew == -time.Hour {
				if err == nil {
					t.Fatal("accepted deadline far beyond clock-skew allowance")
				}
			} else if err != nil {
				t.Fatalf("valid production-length grant with clock offset %s: %v", skew, err)
			}
		})
	}
}

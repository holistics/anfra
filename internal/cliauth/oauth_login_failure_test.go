package cliauth

import (
	"context"
	"errors"
	"github.com/holistics/anfra/shared/apperr"
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

	"golang.org/x/oauth2"
)

func TestInvalidLoginGrantRevokesOnlyAtDiscoveredEndpoint(t *testing.T) {
	for _, invalid := range []string{"foreign org", "missing access token"} {
		t.Run(invalid, func(t *testing.T) {
			t.Setenv("ANFRA_HOME", t.TempDir())
			var origin string
			var revocations atomic.Int32
			var foreignCalls atomic.Int32
			foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignCalls.Add(1) }))
			defer foreign.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if discoveryHandler(w, r, origin) {
					return
				}
				switch r.URL.Path {
				case "/oauth/token":
					w.Header().Set("Content-Type", "application/json")
					response := protocolToken(origin)
					if invalid == "foreign org" {
						response.Grant.Org.URL = foreign.URL + "/o/selected"
					} else {
						response.AccessToken = ""
					}
					_ = jsonkit.MarshalWrite(w, response)
				case "/oauth/revoke":
					revocations.Add(1)
					_ = r.ParseForm()
					if r.Form.Get("token") != "new-refresh" || r.Form.Get("client_id") != "anfra-cli" {
						t.Error("wrong revocation credential")
					}
				default:
					t.Error("unexpected request")
				}
			}))
			defer server.Close()
			origin = server.URL
			_, err := (Manager{}).Login(context.Background(), origin, "", func(ctx context.Context, raw string) error {
				authorization, _ := url.Parse(raw)
				query := authorization.Query()
				callback := query.Get("redirect_uri") + "?" + url.Values{"state": {query.Get("state")}, "iss": {origin}, "code": {"code"}}.Encode()
				response, err := http.Get(callback) //nolint:gosec // G107: this test's literal-loopback callback listener
				if err != nil {
					return err
				}
				response.Body.Close()
				return nil
			})
			if err == nil || strings.Contains(err.Error(), "new-refresh") {
				t.Fatalf("unsafe login failure %v", err)
			}
			logins, inventoryErr := (Manager{}).StoredLogins(context.Background())
			if inventoryErr != nil || len(logins) != 0 || revocations.Load() != 1 || foreignCalls.Load() != 0 {
				t.Fatalf("logins=%v err=%v revocations=%d foreign=%d", logins, inventoryErr, revocations.Load(), foreignCalls.Load())
			}
		})
	}
}

func TestOAuthResponseCapAndSanitizedErrors(t *testing.T) {
	for _, body := range []string{strings.Repeat("x", (64<<10)+1), `{"error":"invalid_grant","error_description":"secret-access secret-refresh"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(body) < 64<<10 {
				w.WriteHeader(400)
			}
			_, _ = w.Write([]byte(body))
		}))
		var response tokenResponse
		err := oauthJSONRequest(context.Background(), http.MethodPost, server.URL, url.Values{"refresh_token": {"secret-refresh"}}, &response)
		server.Close()
		if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), server.URL) {
			t.Fatalf("unsafe OAuth error %v", err)
		}
	}
}

func TestLoginDiscoveryErrorsAreActionable(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"missing endpoint", "secret-response", "HTTP 404 (Not Found)", 404},
		{"unavailable", "secret-response", "HTTP 503 (Service Unavailable)", 503},
		{"invalid JSON", "secret-response", "didn't reply as an anfra server would", 200},
		{"incompatible server", `{}`, "API this version of anfra can use", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := (Manager{}).Login(context.Background(), server.URL, "", func(context.Context, string) error {
				t.Error("browser opened despite discovery failure")
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), server.URL) || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "contact your org's admin") || strings.Contains(err.Error(), "secret-response") {
				t.Fatalf("missing safe, actionable discovery error: %v", err)
			}
		})
	}
}

func TestLoginHandlesLauncherFailureAndCancellation(t *testing.T) {
	for _, failure := range []string{"launcher failed", "cancelled"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("ANFRA_HOME", t.TempDir())
			var origin string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !discoveryHandler(w, r, origin) {
					t.Error("unexpected token request")
				}
			}))
			defer server.Close()
			origin = server.URL
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			finished := make(chan struct{})
			_, err := (Manager{}).Login(ctx, origin, "", func(launchCtx context.Context, _ string) error {
				defer close(finished)
				if deadline, ok := launchCtx.Deadline(); !ok || time.Until(deadline) > 10*time.Minute {
					t.Error("launcher lacks login deadline")
				}
				if failure == "launcher failed" {
					return errors.New("secret-launcher-diagnostic")
				}
				cancel()
				<-launchCtx.Done()
				return launchCtx.Err()
			})
			if err == nil || strings.Contains(err.Error(), "secret-launcher-diagnostic") {
				t.Fatalf("unsafe launcher outcome: %v", err)
			}
			if failure == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("login lost cancellation: %v", err)
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("launcher ignored login cancellation")
			}
		})
	}
}

func TestCodeExchangeFailureDoesNotRetryOrExposeSecrets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"OAuth error", http.StatusBadRequest, `{"error":"invalid_grant","error_description":"secret-code secret-refresh"}`},
		{"oversized response", http.StatusOK, strings.Repeat("x", (64<<10)+1)},
		{"malformed JSON", http.StatusOK, `{"access_token":"secret-access",`},
		{"duplicate field", http.StatusOK, `{"access_token":"secret-access","access_token":"other"}`},
		{"redirect", http.StatusTemporaryRedirect, "secret-response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			config := oauth2.Config{ClientID: oauthClientID, Endpoint: oauth2.Endpoint{TokenURL: server.URL, AuthStyle: oauth2.AuthStyleInParams}}
			_, err := exchangeOAuthCode(context.Background(), &config, "secret-code", oauth2.GenerateVerifier(), server.URL+"/api")
			if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), server.URL) {
				t.Fatalf("unsafe exchange error: %v", err)
			}
			if attempts.Load() != 1 {
				t.Fatalf("code exchange made %d attempts", attempts.Load())
			}
		})
	}
}

// A failure in signing in keeps its own code and message, and says which
// server it was in its context: the public step, not words added to the
// message.
func TestLoginFailureSaysTheServerInItsContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	_, err := (Manager{}).Login(context.Background(), server.URL, "", func(context.Context, string) error { return nil })
	r := apperr.From(err).Response("")
	if r.Code != "unavailable" || strings.Contains(r.Message, server.URL) {
		t.Errorf("code %s, message %q: want unavailable, the failure's own message", r.Code, r.Message)
	}
	if len(r.Context) != 1 || r.Context[0].Step != "anfra_cli.sign_in" || r.Context[0].Text != "sign in to '"+server.URL+"'" {
		t.Errorf("context %+v, want the sign-in step", r.Context)
	}
}

func TestLoginChecksTheCredentialFileFirst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ANFRA_HOME", dir)
	path := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	// No server: the file is checked before anything is asked of one.
	_, err := (Manager{}).Login(context.Background(), "https://cloud.example.com", "", func(context.Context, string) error {
		t.Error("browser opened despite a damaged credential file")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "contents aren't valid") {
		t.Fatalf("damaged credential file not reported first: %v", err)
	}
}

func TestCredentialFileErrorsSayTheCause(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode os.FileMode
		want string
	}{
		{"permissions", 0o644, "`chmod 600 "},
		{"contents", 0o600, "contents aren't valid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("ANFRA_HOME", dir)
			path := filepath.Join(dir, "credentials.json")
			if err := os.WriteFile(path, []byte("not json"), tc.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, tc.mode); err != nil { // past the umask
				t.Fatal(err)
			}
			_, err := (Manager{}).StoredLogins(context.Background())
			if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q about %s, got %v", tc.want, path, err)
			}
		})
	}
}

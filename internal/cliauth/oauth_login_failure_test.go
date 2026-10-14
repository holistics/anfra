package cliauth

import (
	"context"
	"errors"
	"github.com/holistics/anfra/shared/jsonkit"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInvalidLoginGrantRevokesOnlyAtDiscoveredEndpoint(t *testing.T) {
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
			response := protocolToken(origin)
			response.Grant.Org.URL = foreign.URL + "/o/selected"
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
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("launcher ignored login cancellation")
			}
		})
	}
}

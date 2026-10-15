package cliauth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/holistics/anfra/internal/cloudapi"
)

type oauthCallbackResult struct {
	code   string
	denied bool
}

// LoginStage is a non-secret progress event, not evidence that a browser page opened.
type LoginStage string

const (
	LoginConnecting LoginStage = "Connecting to server"
	LoginWaiting    LoginStage = "Waiting for browser approval"
	LoginSaving     LoginStage = "Saving sign-in"
)

// Login performs browser loopback login, saving the authoritative returned org,
// which may differ from hint. openBrowser must honor its context and never log the URL.
func (m Manager) Login(ctx context.Context, origin, hint string, openBrowser func(context.Context, string) error) (cloudapi.Org, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	origin, err := cloudapi.ParseOrigin(origin)
	if err != nil {
		return cloudapi.Org{}, errors.New("cliauth: invalid server origin")
	}
	if hint != "" {
		org, parseErr := cloudapi.ParseOrg(origin + "/o/" + hint)
		if parseErr != nil || org.Slug != hint {
			return cloudapi.Org{}, errors.New("cliauth: invalid org hint")
		}
	}
	if openBrowser == nil {
		return cloudapi.Org{}, errors.New("cliauth: browser loopback login is required")
	}
	if m.LoginProgress != nil {
		m.LoginProgress(LoginConnecting)
	}
	metadata, err := discoverOAuthServer(ctx, origin)
	if err != nil {
		return cloudapi.Org{}, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return cloudapi.Org{}, errors.New("cliauth: cannot bind browser callback listener")
	}
	defer listener.Close()
	redirect := "http://" + listener.Addr().String() + "/oauth/callback"
	state := oauth2.GenerateVerifier()
	verifier := oauth2.GenerateVerifier()
	result := make(chan oauthCallbackResult, 1)
	var once sync.Once
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		callback, ok := validateOAuthCallback(r, listener.Addr().String(), state, origin)
		if !ok {
			http.Error(w, "Invalid OAuth callback.", http.StatusBadRequest)
			return
		}
		accepted := false
		once.Do(func() { accepted = true; result <- callback })
		if !accepted {
			http.Error(w, "OAuth callback already completed.", http.StatusConflict)
			return
		}
		_, _ = fmt.Fprintln(w, "You may close this window and return to anfra.")
	})
	defer server.Close()
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	config := oauth2.Config{ClientID: oauthClientID, RedirectURL: redirect, Scopes: []string{"all"}, Endpoint: oauth2.Endpoint{AuthURL: metadata.AuthorizationEndpoint}}
	options := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("resource", origin+"/api")}
	if hint != "" {
		options = append(options, oauth2.SetAuthURLParam("org_hint", hint))
	}
	launchFinished := make(chan error, 1)
	go func() { launchFinished <- openBrowser(ctx, config.AuthCodeURL(state, options...)) }()
	if m.LoginProgress != nil {
		m.LoginProgress(LoginWaiting)
	}
	var launchErrors <-chan error = launchFinished
	var callback oauthCallbackResult
waitCallback:
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return cloudapi.Org{}, errors.New("sign-in timed out; run anfra auth login to try again")
			}
			return cloudapi.Org{}, ctx.Err()
		case <-serveErrors:
			return cloudapi.Org{}, errors.New("cliauth: browser callback listener stopped")
		case err := <-launchErrors:
			if err != nil {
				return cloudapi.Org{}, errors.New("could not open your browser; check your default browser and try again")
			}
			launchErrors = nil
		case callback = <-result:
			break waitCallback
		}
	}
	if callback.denied {
		return cloudapi.Org{}, errors.New("sign-in was denied in the browser")
	}
	started := time.Now()
	var response tokenResponse
	err = oauthJSONRequest(ctx, http.MethodPost, metadata.TokenEndpoint, url.Values{"grant_type": {"authorization_code"}, "client_id": {oauthClientID}, "code": {callback.code}, "redirect_uri": {redirect}, "code_verifier": {verifier}, "resource": {origin + "/api"}}, &response)
	if err != nil && !safeOpaqueToken(response.RefreshToken) && !safeOpaqueToken(response.AccessToken) {
		return cloudapi.Org{}, err
	}
	var entry storedCredential
	if err == nil {
		entry, err = validateTokenResponse(response, origin, started, nil)
	}
	if err == nil {
		entry.TokenEndpoint = metadata.TokenEndpoint
		entry.RevocationEndpoint = metadata.RevocationEndpoint
		if m.LoginProgress != nil {
			m.LoginProgress(LoginSaving)
		}
		err = withCredentialLock(ctx, func(inventory *credentialInventory, path string) error {
			if !refreshRecoveryRequired(path, entry.Org) {
				if err := markRefreshRecovery(path, entry.Org); err != nil {
					return err
				}
			}
			inventory.Entries[credentialKey(entry.Org)] = entry
			if err := saveCredentialInventory(path, inventory); err != nil {
				return err
			}
			return clearRefreshRecovery(path, entry.Org)
		})
	}
	if err != nil {
		// The response is untrusted; use only the independently discovered endpoint.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cleanupCancel()
		revocationToken := response.RefreshToken
		if !safeOpaqueToken(revocationToken) {
			revocationToken = response.AccessToken
		}
		if !safeOpaqueToken(revocationToken) || revokeOAuthCredential(cleanupCtx, metadata.RevocationEndpoint, revocationToken) != nil {
			return cloudapi.Org{}, fmt.Errorf("sign-in failed; server access may still be active; remove it at %s/account/connected-apps before trying again", origin)
		}
		return cloudapi.Org{}, errors.New("sign-in failed; try signing in again")
	}
	return entry.Org, nil
}

func validateOAuthCallback(r *http.Request, host, state, issuer string) (oauthCallbackResult, bool) {
	if r.Method != http.MethodGet || r.Host != host || r.URL.Path != "/oauth/callback" || r.URL.RawPath != "" || len(r.URL.RawQuery) > 8192 {
		return oauthCallbackResult{}, false
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return oauthCallbackResult{}, false
	}
	for key, values := range query {
		if len(values) != 1 {
			return oauthCallbackResult{}, false
		}
		switch key {
		case "state", "iss", "code", "error", "error_description", "error_uri":
		default:
			return oauthCallbackResult{}, false
		}
	}
	if subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1 || query.Get("iss") != issuer {
		return oauthCallbackResult{}, false
	}
	_, hasCode := query["code"]
	_, hasError := query["error"]
	if hasCode == hasError {
		return oauthCallbackResult{}, false
	}
	if hasError {
		return oauthCallbackResult{denied: true}, query.Get("error") != ""
	}
	if _, ok := query["error_description"]; ok {
		return oauthCallbackResult{}, false
	}
	if _, ok := query["error_uri"]; ok {
		return oauthCallbackResult{}, false
	}
	return oauthCallbackResult{code: query.Get("code")}, safeOpaqueToken(query.Get("code"))
}

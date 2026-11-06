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

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"
	"github.com/holistics/anfra/shared/apptracing"
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
	LoginSaving     LoginStage = "Saving authentication"
)

// signInStep is signing in to an org's server: public, so a failure in it says
// which server it was, in its context, its message the failure's own.
var signInStep = appstep.Define(clierr.NS, "sign_in", appstep.Public("sign in to {origin}"))

// Login performs browser loopback login, saving the authoritative returned org,
// which may differ from hint. openBrowser must honor its context and never log the URL.
func (m Manager) Login(ctx context.Context, origin, hint string, openBrowser func(context.Context, string) error) (_ cloudapi.Org, err error) {
	ctx, span := apptracing.Start(ctx, signInStep, "origin", origin)
	defer span.End(&err)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	origin, err = cloudapi.ParseOrigin(origin)
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
		return cloudapi.Org{}, errors.New("cliauth: browser loopback authentication is required")
	}
	// Check the authentication file before the browser, so a problem with it
	// stops the login before the reader approves anything.
	if err := withCredentialLock(ctx, func(*credentialInventory, string) error { return nil }); err != nil {
		return cloudapi.Org{}, err
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
		return cloudapi.Org{}, clierr.Precondition("Couldn't open a local port to receive the browser's approval. Allow anfra to listen on 127.0.0.1, and try again.")
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
	config := oauth2.Config{ClientID: oauthClientID, RedirectURL: redirect, Scopes: []string{"all"}, Endpoint: oauth2.Endpoint{AuthURL: metadata.AuthorizationEndpoint, TokenURL: metadata.TokenEndpoint, AuthStyle: oauth2.AuthStyleInParams}}
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
				return cloudapi.Org{}, clierr.Precondition("Authentication timed out before it was approved in the browser. Try again.")
			}
			return cloudapi.Org{}, ctx.Err()
		case <-serveErrors:
			return cloudapi.Org{}, clierr.Precondition("Couldn't receive the browser's approval, because anfra's local server stopped unexpectedly. Try again.")
		case err := <-launchErrors:
			if err != nil {
				if ctx.Err() != nil {
					return cloudapi.Org{}, ctx.Err()
				}
				// The opener may say what to do instead, as the CLI does.
				var public *apperr.Error
				if errors.As(err, &public) {
					return cloudapi.Org{}, err
				}
				return cloudapi.Org{}, clierr.Precondition("Couldn't open a browser to approve the authentication.")
			}
			launchErrors = nil
		case callback = <-result:
			break waitCallback
		}
	}
	if callback.denied {
		return cloudapi.Org{}, apperr.New(clierr.Interrupted, "The authentication was denied or failed in the browser. If you meant to approve it, try again.")
	}
	started := time.Now()
	response, err := exchangeOAuthCode(ctx, &config, callback.code, verifier, origin+"/api")
	if err != nil && !safeOpaqueToken(response.RefreshToken) && !safeOpaqueToken(response.AccessToken) {
		return cloudapi.Org{}, err
	}
	var entry storedCredential
	// Why it failed, and what to do next, by cause.
	failure := "the authentication server returned an unexpected reply"
	next := "Run `anfra auth login` again, and if this happens again, contact your org's admin."
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
		if err != nil {
			failure = "couldn't save the authentication on this device: " + err.Error()
			next = "Once the authentication can be saved, run `anfra auth login` again."
		}
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
			return cloudapi.Org{}, clierr.Precondition("%s. This device may still have access on the server, so remove it at %s/account/connected-apps. %s", failure, origin, next)
		}
		return cloudapi.Org{}, clierr.Precondition("%s. anfra removed this device's access on the server. %s", failure, next)
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

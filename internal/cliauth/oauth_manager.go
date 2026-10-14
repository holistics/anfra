package cliauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/holistics/anfra/internal/cloudapi"
)

// Manager uses the machine's home directory; its zero value is ready for use.
// It never uses an automatic token source or writes repo configuration.
type Manager struct {
	// LoginProgress receives bounded, non-secret stages, synchronously during login.
	LoginProgress func(LoginStage)
}

// StoredLogin exposes only local display metadata, never credential material.
type StoredLogin struct {
	Org        cloudapi.Org
	Name       string
	LocalState string
}

func canonicalCredentialOrg(org cloudapi.Org) (cloudapi.Org, error) {
	parsed, err := cloudapi.ParseOrg(org.String())
	if err != nil {
		return cloudapi.Org{}, errors.New("cliauth: invalid org address")
	}
	return parsed, nil
}

func loginRequired(org cloudapi.Org) error {
	return fmt.Errorf("sign-in required; run anfra auth login %s", org.String())
}

// StoredLogins is local inventory only, not proof of a usable remote grant.
func (Manager) StoredLogins(ctx context.Context) ([]StoredLogin, error) {
	orgs := []StoredLogin{}
	err := withCredentialLock(ctx, func(inventory *credentialInventory, path string) error {
		for key, entry := range inventory.Entries {
			org, err := canonicalCredentialOrg(entry.Org)
			if err != nil || key != credentialKey(org) {
				return errors.New("cliauth: invalid stored org address")
			}
			name := entry.OrgName
			if name == "" {
				name = org.Slug
			}
			state := "stored"
			if entry.Pending || refreshRecoveryRequired(path, org) || !entry.GrantExpiry.After(time.Now()) || !validGrantUUID(entry.GrantID) || !validGrantUUID(entry.OrgID) || !safeOpaqueToken(entry.AccessToken) || !safeOpaqueToken(entry.RefreshToken) || !safeOAuthEndpoint(entry.TokenEndpoint, org.Origin) || !safeOAuthEndpoint(entry.RevocationEndpoint, org.Origin) {
				state = "sign_in_again"
			}
			orgs = append(orgs, StoredLogin{Org: org, Name: name, LocalState: state})
		}
		return nil
	})
	sort.Slice(orgs, func(i, j int) bool {
		if orgs[i].Name == orgs[j].Name {
			return orgs[i].Org.String() < orgs[j].Org.String()
		}
		return orgs[i].Name < orgs[j].Name
	})
	return orgs, err
}

// Authorize checks credentials now and resolves them again before every request,
// so prompts or another process's refresh cannot leave a captured access token stale.
func (Manager) Authorize(ctx context.Context, org cloudapi.Org) (func(*http.Request) error, error) {
	org, err := canonicalCredentialOrg(org)
	if err != nil {
		return nil, err
	}
	if _, err := loadAccessToken(ctx, org); err != nil {
		return nil, err
	}
	return func(req *http.Request) error {
		if req == nil || req.URL == nil {
			return errors.New("cliauth: missing API request")
		}
		if req.URL.Scheme+"://"+req.URL.Host != org.Origin || req.URL.User != nil || !apiOrgRequestPath(req.URL, org) || req.Header.Get("Cookie") != "" {
			return errors.New("cliauth: refusing credentials for a different destination or mixed credentials")
		}
		token, err := loadAccessToken(req.Context(), org)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}, nil
}

// loadAccessToken serializes on-demand refresh with durable pending state.
func loadAccessToken(ctx context.Context, org cloudapi.Org) (string, error) {
	var token string
	err := withCredentialLock(ctx, func(inventory *credentialInventory, path string) error {
		entry, ok := inventory.Entries[credentialKey(org)]
		if !ok || entry.Pending || refreshRecoveryRequired(path, org) || entry.Org != org || !entry.GrantExpiry.After(time.Now()) || !validGrantUUID(entry.GrantID) || !validGrantUUID(entry.OrgID) || !safeOpaqueToken(entry.AccessToken) || !safeOpaqueToken(entry.RefreshToken) || !safeOAuthEndpoint(entry.TokenEndpoint, org.Origin) || !safeOAuthEndpoint(entry.RevocationEndpoint, org.Origin) {
			return loginRequired(org)
		}
		if time.Until(entry.AccessExpiry) < 5*time.Minute {
			if err := markRefreshRecovery(path, org); err != nil {
				return err
			}
			entry.Pending = true
			inventory.Entries[credentialKey(org)] = entry
			if err := saveCredentialInventory(path, inventory); err != nil {
				return err
			}
			started := time.Now()
			var response tokenResponse
			err := oauthJSONRequest(ctx, http.MethodPost, entry.TokenEndpoint, url.Values{"grant_type": {"refresh_token"}, "client_id": {oauthClientID}, "refresh_token": {entry.RefreshToken}, "resource": {org.Origin + "/api"}}, &response)
			if err != nil {
				return loginRequired(org)
			}
			successor, err := validateTokenResponse(response, org.Origin, started, &entry)
			if err != nil {
				return loginRequired(org)
			}
			successor.TokenEndpoint = entry.TokenEndpoint
			successor.RevocationEndpoint = entry.RevocationEndpoint
			inventory.Entries[credentialKey(org)] = successor
			if err := saveCredentialInventory(path, inventory); err != nil {
				return err
			}
			if err := clearRefreshRecovery(path, org); err != nil {
				return err
			}
			entry = successor
		}
		token = entry.AccessToken
		return nil
	})
	return token, err
}

func apiOrgRequestPath(u *url.URL, org cloudapi.Org) bool {
	return u.RawPath == "" && len(u.Path) > len("/api/o/"+org.Slug+"/") && u.Path[:len("/api/o/"+org.Slug+"/")] == "/api/o/"+org.Slug+"/"
}

// LogoutResult distinguishes local removal from confirmed server revocation.
type LogoutResult struct {
	LocalRemoved              bool `json:"local_removed"`
	RemoteRevocationConfirmed bool `json:"remote_revocation_confirmed"`
}

// Logout removes only the selected local entry even when remote revocation fails.
func (Manager) Logout(ctx context.Context, org cloudapi.Org) (LogoutResult, error) {
	var result LogoutResult
	org, err := canonicalCredentialOrg(org)
	if err != nil {
		return result, err
	}
	// Cancellation can stop revocation, not the promised local credential removal.
	localCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	err = withCredentialLock(localCtx, func(inventory *credentialInventory, path string) error {
		entry, ok := inventory.Entries[credentialKey(org)]
		if !ok {
			result.LocalRemoved = true
			return nil
		}
		confirmed := false
		if entry.Org == org && safeOAuthEndpoint(entry.RevocationEndpoint, org.Origin) && safeOpaqueToken(entry.RefreshToken) {
			confirmed = revokeOAuthCredential(ctx, entry.RevocationEndpoint, entry.RefreshToken) == nil
		}
		delete(inventory.Entries, credentialKey(org))
		if err := saveCredentialInventory(path, inventory); err != nil {
			return err
		}
		result.LocalRemoved = true
		result.RemoteRevocationConfirmed = confirmed
		if err := clearRefreshRecovery(path, org); err != nil {
			return err
		}
		if !confirmed {
			return fmt.Errorf("signed out on this device, but server access may still be active.\nRemove access at %s/account/connected-apps", org.Origin)
		}
		return nil
	})
	return result, err
}

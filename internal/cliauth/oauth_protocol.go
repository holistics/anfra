package cliauth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/shared/jsonkit"
)

const oauthClientID = "anfra-cli"

// oauthClockSkew bounds tolerance between client and server wall clocks. Access
// expiry remains conservative and the absolute deadline must stay exact on refresh.
const oauthClockSkew = 5 * time.Minute

// A fresh client has no cookie jar, credentials, redirects, tracing or retry wrapper.
func oauthHTTPClient() *http.Client {
	// No proxy: OAuth secrets must not enter a configured proxy's logs.
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		// HTTP/2 may transparently retry REFUSED_STREAM/GOAWAY requests. A fresh
		// HTTP/1 connection prevents transport retries of these single-use POSTs.
		TLSNextProto:      map[string]func(string, *tls.Conn) http.RoundTripper{},
		DisableKeepAlives: true,
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type serverMetadata struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RevocationEndpoint    string   `json:"revocation_endpoint"`
	ResponseTypes         []string `json:"response_types_supported"`
	GrantTypes            []string `json:"grant_types_supported"`
	TokenAuth             []string `json:"token_endpoint_auth_methods_supported"`
	RevocationAuth        []string `json:"revocation_endpoint_auth_methods_supported"`
	Scopes                []string `json:"scopes_supported"`
	Challenges            []string `json:"code_challenge_methods_supported"`
	ResponseIssuer        bool     `json:"authorization_response_iss_parameter_supported"`
}

type resourceMetadata struct {
	Resource      string   `json:"resource"`
	Servers       []string `json:"authorization_servers"`
	Scopes        []string `json:"scopes_supported"`
	BearerMethods []string `json:"bearer_methods_supported"`
}

func containsCapability(values []string, required string) bool {
	for _, value := range values {
		if value == required {
			return true
		}
	}
	return false
}

func safeOAuthEndpoint(raw, origin string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.Path == "" || u.RawPath != "" {
		return false
	}
	if u.Scheme+"://"+u.Host != origin || !strings.HasPrefix(u.Path, "/") || u.EscapedPath() != u.Path {
		return false
	}
	for _, segment := range strings.Split(strings.TrimPrefix(u.Path, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func discoverOAuthServer(ctx context.Context, origin string) (serverMetadata, error) {
	var resource resourceMetadata
	if err := oauthJSONRequest(ctx, http.MethodGet, origin+"/.well-known/oauth-protected-resource/api", nil, &resource); err != nil {
		return serverMetadata{}, err
	}
	if resource.Resource != origin+"/api" || len(resource.Servers) != 1 || resource.Servers[0] != origin || !containsCapability(resource.Scopes, "all") || !containsCapability(resource.BearerMethods, "header") {
		return serverMetadata{}, errors.New("server does not advertise a compatible anfra API; check the organization URL or contact your server administrator")
	}
	var server serverMetadata
	if err := oauthJSONRequest(ctx, http.MethodGet, origin+"/.well-known/oauth-authorization-server", nil, &server); err != nil {
		return serverMetadata{}, err
	}
	if server.Issuer != origin || !safeOAuthEndpoint(server.AuthorizationEndpoint, origin) || !safeOAuthEndpoint(server.TokenEndpoint, origin) || !safeOAuthEndpoint(server.RevocationEndpoint, origin) || !server.ResponseIssuer || !containsCapability(server.ResponseTypes, "code") || !containsCapability(server.GrantTypes, "authorization_code") || !containsCapability(server.GrantTypes, "refresh_token") || !containsCapability(server.TokenAuth, "none") || !containsCapability(server.RevocationAuth, "none") || !containsCapability(server.Scopes, "all") || !containsCapability(server.Challenges, "S256") {
		return serverMetadata{}, errors.New("server does not advertise compatible browser authentication; check the organization URL or contact your server administrator")
	}
	return server, nil
}

func oauthJSONRequest(ctx context.Context, method, endpoint string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return errors.New("cliauth: cannot construct OAuth request")
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Accept", "application/json")
	client := oauthHTTPClient()
	defer client.CloseIdleConnections()
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("authentication server did not respond in time; check your connection and try again")
		}
		return errors.New("could not connect securely to the authentication server; check your connection, server address and HTTPS certificate")
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<10+1))
	if err != nil || len(raw) > 64<<10 {
		return errors.New("could not read the authentication server's response, or it exceeded the size limit; try again or contact your server administrator")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication server returned HTTP %d (%s); check the organization URL or contact your server administrator", res.StatusCode, http.StatusText(res.StatusCode))
	}
	if out == nil {
		return nil
	}
	if jsonkit.Unmarshal(raw, out) != nil {
		return errors.New("authentication server returned invalid JSON; check that the URL points to an anfra server or contact your server administrator")
	}
	return nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	Resource     string `json:"resource"`
	Grant        struct {
		ID  string `json:"id"`
		Org struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"org"`
		ExpiresAt time.Time `json:"expires_at"`
	} `json:"anfra_grant"`
}

var oauthUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validGrantUUID(value string) bool {
	return oauthUUIDPattern.MatchString(value) && value != "00000000-0000-0000-0000-000000000000"
}
func safeOpaqueToken(value string) bool {
	if len(value) == 0 || len(value) > 8192 {
		return false
	}
	for _, c := range value {
		if c <= 32 || c >= 127 {
			return false
		}
	}
	return true
}

func validateTokenResponse(response tokenResponse, origin string, started time.Time, predecessor *storedCredential) (storedCredential, error) {
	invalid := errors.New("cliauth: invalid OAuth grant response; authenticate again")
	org, err := cloudapi.ParseOrg(response.Grant.Org.URL)
	deadline := response.Grant.ExpiresAt
	_, offset := deadline.Zone()
	if offset != 0 {
		return storedCredential{}, invalid
	}
	if err != nil || org.Origin != origin || org.Slug != response.Grant.Org.Slug || response.Grant.Org.URL != org.String() || !validGrantUUID(response.Grant.ID) || !validGrantUUID(response.Grant.Org.ID) || !safeOpaqueToken(response.AccessToken) || !safeOpaqueToken(response.RefreshToken) || !strings.EqualFold(response.TokenType, "Bearer") || response.Scope != "all" || response.Resource != origin+"/api" || response.ExpiresIn <= 0 || response.ExpiresIn > 3600 || !deadline.After(time.Now()) || deadline.Sub(started) > 180*24*time.Hour+oauthClockSkew {
		return storedCredential{}, invalid
	}
	// Starting before the HTTP request is conservative about network latency.
	expiry := started.Add(time.Duration(response.ExpiresIn) * time.Second)
	if expiry.After(deadline.Add(oauthClockSkew)) || !expiry.After(time.Now()) {
		return storedCredential{}, invalid
	}
	if expiry.After(deadline) {
		expiry = deadline
	}
	result := storedCredential{Org: org, OrgName: response.Grant.Org.Name, GrantID: response.Grant.ID, OrgID: response.Grant.Org.ID, GrantExpiry: deadline, AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, AccessExpiry: expiry}
	if predecessor != nil && (result.GrantID != predecessor.GrantID || result.OrgID != predecessor.OrgID || result.Org != predecessor.Org || !result.GrantExpiry.Equal(predecessor.GrantExpiry) || result.RefreshToken == predecessor.RefreshToken || result.AccessToken == predecessor.AccessToken) {
		return storedCredential{}, invalid
	}
	if predecessor != nil && result.OrgName == "" {
		result.OrgName = predecessor.OrgName
	}
	return result, nil
}

func revokeOAuthCredential(ctx context.Context, endpoint, token string) error {
	return oauthJSONRequest(ctx, http.MethodPost, endpoint, url.Values{"client_id": {oauthClientID}, "token": {token}}, nil)
}

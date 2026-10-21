package motherduck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/holistics/anfra/shared/jsonkit"
)

// API is MotherDuck's REST API, for the tokens anfra makes an org.
type API struct {
	// BaseURL is the API's; "" is MotherDuck's, https://api.motherduck.com.
	BaseURL   string
	HTTP      *http.Client
	UserAgent string
}

// Token is a token made through the API: its ID, which revokes it, its name,
// and its secret, which the API answers once.
type Token struct {
	ID, Name, Secret string
}

// APIError is an answer of the API's that is not a success.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("MotherDuck answered %d", e.Status)
	}
	return fmt.Sprintf("MotherDuck answered %d: %s", e.Status, e.Message)
}

// NameTaken reports whether err is MotherDuck refusing a token's name, which
// another of the user's tokens has.
func NameTaken(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == http.StatusConflict
}

// CreateReadToken makes the user a read-scaling token, which reads their
// databases and writes none, and does not expire. session is the user's own
// token (CLI.SessionToken), which authorizes it.
func (a API) CreateReadToken(ctx context.Context, session, username, name, description string) (Token, error) {
	body, err := jsonkit.Marshal(map[string]any{"name": name, "description": description, "token_type": "read_scaling"})
	if err != nil {
		return Token{}, err
	}
	var out struct {
		ID        string `json:"id"`
		Token     string `json:"token"`
		TokenType string `json:"token_type"`
	}
	if err := a.do(ctx, http.MethodPost, "/v1/users/"+url.PathEscape(username)+"/tokens", session, body, &out); err != nil {
		return Token{}, fmt.Errorf("create a MotherDuck token: %w", err)
	}
	if out.ID == "" || out.Token == "" {
		return Token{}, errors.New("create a MotherDuck token: the answer has no token")
	}
	if out.TokenType != "read_scaling" {
		// Not what was asked: one that may write is not given to an org.
		t := Token{ID: out.ID}
		if err := a.RevokeToken(ctx, session, username, t.ID); err != nil {
			return Token{}, fmt.Errorf("MotherDuck made a %q token, not a read-only one, and revoking it failed (%v): revoke it in MotherDuck's settings", out.TokenType, err)
		}
		return Token{}, fmt.Errorf("MotherDuck made a %q token, not a read-only one; it is revoked", out.TokenType)
	}
	return Token{ID: out.ID, Name: name, Secret: out.Token}, nil
}

// RevokeToken revokes the user's token id. anfra revokes only a token it made in
// the same publish, which nothing uses yet.
func (a API) RevokeToken(ctx context.Context, session, username, id string) error {
	return a.do(ctx, http.MethodDelete, "/v1/users/"+url.PathEscape(username)+"/tokens/"+url.PathEscape(id), session, nil, nil)
}

// do calls the API. An error carries the API's message, never the request.
func (a API) do(ctx context.Context, method, path, session string, body []byte, out any) error {
	base := a.BaseURL
	if base == "" {
		base = "https://api.motherduck.com"
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+session)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.UserAgent != "" {
		req.Header.Set("User-Agent", a.UserAgent)
	}
	client := a.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	answer, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode/100 != 2 {
		var failed struct {
			Message string `json:"message"`
		}
		_ = jsonkit.Unmarshal(answer, &failed)
		return &APIError{Status: res.StatusCode, Message: failed.Message}
	}
	if out == nil {
		return nil
	}
	return jsonkit.Unmarshal(answer, out)
}

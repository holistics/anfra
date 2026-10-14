package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Until `anfra login`, a publish carries the session in sessionEnv, as
// anfra-cloud's session cookie; without one, it says how to set it.
func TestSessionCredential(t *testing.T) {
	t.Setenv(sessionEnv, "")
	if _, err := sessionCredential(); err == nil || !strings.Contains(err.Error(), sessionEnv) {
		t.Errorf("no session: %v, want one naming %s", err, sessionEnv)
	}
	t.Setenv(sessionEnv, "dev-session")
	authorize, err := sessionCredential()
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/", nil)
	authorize(r)
	if c, err := r.Cookie("anfra_session"); err != nil || c.Value != "dev-session" {
		t.Errorf("cookie = %v, %v", c, err)
	}
}

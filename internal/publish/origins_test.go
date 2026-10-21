package publish

import (
	"strings"
	"testing"
)

const repoOrigins = ".anfra/config.yml"

// The repo's allowed origins the org lacks are said, the ones it has are not, and
// the publish goes on.
func TestPublishWarnsOfOriginsTheOrgLacks(t *testing.T) {
	s, url := newServer(t)
	s.origins = []string{"https://cdn.acme.com"}
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a", repoOrigins: "apps:\n  allow_origins: [https://cdn.acme.com, https://fonts.acme.com, https://maps.acme.com]\n"})

	_, errOut, err := run(t, dir, Options{SetOrg: url}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "The org does not allow https://fonts.acme.com, https://maps.acme.com, which this repo's Data Apps may load from (.anfra/config.yml apps.allow_origins): apps loading from them are blocked there.") {
		t.Errorf("the lacking origins are not said: %q", errOut)
	}
	if strings.Contains(errOut, "allow https://cdn.acme.com") {
		t.Errorf("an origin the org has is said: %q", errOut)
	}
	if len(s.deployed) != 1 {
		t.Errorf("deployed %v, want the publish to go on", s.deployed)
	}
}

// Nothing is said when the org allows every one, or the repo lists none.
func TestPublishSaysNothingOfOriginsTheOrgHas(t *testing.T) {
	for name, config := range map[string]string{
		"all allowed": "apps:\n  allow_origins: [https://cdn.acme.com]\n",
		"none listed": "apps: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			s, url := newServer(t)
			s.origins = []string{"https://cdn.acme.com"}
			dir := t.TempDir()
			write(t, dir, map[string]string{"a.aml": "a", repoOrigins: config})
			_, errOut, err := run(t, dir, Options{SetOrg: url}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(errOut, "origins") || strings.Contains(errOut, "does not allow") {
				t.Errorf("said %q", errOut)
			}
		})
	}
}

// A server without allowed origins is said not checked, and the publish goes on.
func TestPublishToAServerWithoutAllowedOrigins(t *testing.T) {
	s, url := newServer(t)
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a", repoOrigins: "apps:\n  allow_origins: [https://cdn.acme.com]\n"})
	_, errOut, err := run(t, dir, Options{SetOrg: url}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "Not checking the org's allowed origins") || len(s.deployed) != 1 {
		t.Errorf("said %q; deployed %v", errOut, s.deployed)
	}
}

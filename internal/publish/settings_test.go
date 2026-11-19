package publish

import (
	"slices"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/shared/repoformat"
)

// deployedPaths are the paths the fake server has published.
func (s *server) deployedPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, f := range s.deployed {
		out = append(out, f.Path)
	}
	return out
}

const cap24h = "query:\n  cache:\n    max_duration: 24h\n"

// To a server that takes the repo's settings, the settings file is published
// with the repo's other files.
func TestPublishSendsTheSettingsFile(t *testing.T) {
	srv, url := newServer(t)
	srv.repoConfig = &cloudapi.RepoConfigOut{}
	dir := newRepo(t, map[string]string{"a.aml": "a", repoformat.ConfigFile: cap24h})
	if _, said, err := run(t, dir, Options{SetOrg: url}, never(t)); err != nil {
		t.Fatalf("%v\n%s", err, said)
	}
	if got := srv.deployedPaths(); !slices.Contains(got, repoformat.ConfigFile) {
		t.Errorf("deployed %v, want the settings file among them", got)
	}
}

// To a server from before the repo's settings, the settings file stays here,
// with a warning, and the rest is published.
func TestPublishToAnOlderServer(t *testing.T) {
	srv, url := newServer(t)
	dir := newRepo(t, map[string]string{"a.aml": "a", repoformat.ConfigFile: cap24h})
	_, said, err := run(t, dir, Options{SetOrg: url}, never(t))
	if err != nil {
		t.Fatalf("%v\n%s", err, said)
	}
	if got := srv.deployedPaths(); !slices.Equal(got, []string{"a.aml"}) {
		t.Errorf("deployed %v, want a.aml alone", got)
	}
	if !strings.Contains(said, "settings_not_published: ") || !strings.Contains(said, "doesn't take the repo's settings yet") {
		t.Errorf("said:\n%s", said)
	}
}

// A publish warns of each change it makes to a setting the org overrides,
// which takes no effect there, and not of an overridden setting it leaves as
// published. It warns of a setting this machine's own file gives a value the
// org doesn't use.
func TestPublishWarnsOfOverriddenChangesAndLocalSettings(t *testing.T) {
	srv, url := newServer(t)
	srv.repoConfig = &cloudapi.RepoConfigOut{Settings: []cloudapi.RepoSetting{
		{Key: "query.cache.enabled", Value: true, Source: "override", Override: &cloudapi.RepoOverride{Value: true, UpdatedBy: "ada@acme.com"}, Editable: true},
		{Key: "query.cache.max_duration", Value: "1h", Source: "override", Repo: "24h", Override: &cloudapi.RepoOverride{Value: "1h"}, Editable: true},
		{Key: "aql.compiler.version", Source: "default", Editable: true},
		{Key: "apps.plugins.export.enabled", Value: true, Source: "override", Override: &cloudapi.RepoOverride{Value: true, UpdatedBy: "ada@acme.com"}, Editable: true},
		{Key: "query.default.input.timezone", Value: "Etc/UTC", Source: "repo", Repo: "Etc/UTC", Editable: true},
	}}
	const published = "query:\n  default:\n    input:\n      timezone: Etc/UTC\n  cache:\n    max_duration: 24h\n"
	dir := newRepo(t, map[string]string{
		"a.aml":                   "a",
		repoformat.ConfigFile:     published,
		".anfra/config.local.yml": "aql:\n  compiler:\n    version: v2\nquery:\n  default:\n    input:\n      timezone: Etc/UTC\n",
	})
	if _, said, err := run(t, dir, Options{SetOrg: url}, never(t)); err != nil {
		t.Fatalf("%v\n%s", err, said)
	}
	// Then this publish turns the cache and the export plugin off, which the
	// org overrides, and leaves the cap as published, which it overrides too.
	write(t, dir, map[string]string{repoformat.ConfigFile: published + "    enabled: false\norg:\n  url: " + url + "\napps:\n  plugins:\n    export:\n      enabled: false\n"})
	_, said, err := run(t, dir, Options{}, never(t))
	if err != nil {
		t.Fatalf("%v\n%s", err, said)
	}
	for _, want := range []string{
		"settings_overridden: This publish changes query.cache.enabled, but " + url + " keeps it at true, because ada@acme.com overrides it there.",
		"settings_overridden: This publish changes apps.plugins.export.enabled, but " + url + " keeps it at true, because ada@acme.com overrides it there.",
		"settings_local: .anfra/config.local.yml sets aql.compiler.version to v2 on this machine, but the org uses anfra's default.",
	} {
		if !strings.Contains(said, want) {
			t.Errorf("said:\n%s\nwant %q", said, want)
		}
	}
	for _, unsaid := range []string{"max_duration", "timezone"} {
		if strings.Contains(said, unsaid) {
			t.Errorf("said:\n%s\nwant nothing of %s", said, unsaid)
		}
	}
}

// A folder fresh from `anfra init`, whose settings file says nothing but
// anfra's defaults and its org, takes the org's settings file without a
// conflict, keeping its org.
func TestPullAdoptsTheOrgsSettingsIntoAFreshFolder(t *testing.T) {
	srv, url := newServer(t)
	srv.repoConfig = &cloudapi.RepoConfigOut{}
	srv.publishAs("bob@acme.com", map[string]string{"models/a.aml": "a\n", repoformat.ConfigFile: "# the team's\n" + cap24h})
	dir := newRepo(t, map[string]string{repoformat.ConfigFile: "# init's\nquery:\n  default:\n    input:\n      weekStartDay: monday\n"})
	res := mustPull(t, dir, PullOptions{SetOrg: url})
	if len(res.Conflicts) != 0 {
		t.Errorf("conflicts %+v", res.Conflicts)
	}
	got := read(t, dir, repoformat.ConfigFile)
	if !strings.HasPrefix(got, "# the team's\n"+cap24h) || !strings.Contains(got, "url: "+url) {
		t.Errorf("settings file:\n%s", got)
	}
}

// A folder whose settings file sets something, with no record of the org,
// can't tell whose is newer: the two are a conflict, as any file is.
func TestPullConflictsOnChangedSettings(t *testing.T) {
	srv, url := newServer(t)
	srv.repoConfig = &cloudapi.RepoConfigOut{}
	srv.publishAs("bob@acme.com", map[string]string{"models/a.aml": "a\n", repoformat.ConfigFile: cap24h})
	dir := newRepo(t, map[string]string{repoformat.ConfigFile: "query:\n  cache:\n    max_duration: 1h\n"})
	res := mustPull(t, dir, PullOptions{SetOrg: url})
	if got := res.Conflicts["add_add"]; len(got) != 1 || !slices.Contains(got[0].Paths, repoformat.ConfigFile) {
		t.Errorf("conflicts %+v", res.Conflicts)
	}
}

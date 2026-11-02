package agents

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseStamp(t *testing.T) {
	for _, tc := range []struct {
		name, skill, want string
		ok                bool
	}{
		{"stamped", "# Skill\n\nDo this.\n\n---\nanfra skills 0.16.0\n", "0.16.0", true},
		{"no final newline", "Do this.\n\n---\nanfra skills 0.16.0", "0.16.0", true},
		{"trailing blank lines", "Do this.\n\n---\nanfra skills 0.16.0\n\n", "0.16.0", true},
		{"CRLF", "Do this.\r\n\r\n---\r\nanfra skills 0.16.0\r\n", "0.16.0", true},
		{"none", "# Skill\n\nDo this.\n", "", false},
		{"not at the end", "---\nanfra skills 0.16.0\n\nMore text.\n", "", false},
		{"without the rule", "Do this.\nanfra skills 0.16.0\n", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseStamp([]byte(tc.skill))
			if got != tc.want || ok != tc.ok {
				t.Errorf("got %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

// Copies finds anfra's skills by their prefix and reads each one's stamp; it
// finds a copy of the old names by their marker, and skips another vendor's.
func TestCopies(t *testing.T) {
	env := newFakeEnv(t)
	repoDir := t.TempDir()
	agents := filepath.Join(env.Home, ".agents/skills")
	writeSkill(t, agents, "anfra-aql", stamped("0.15.0"))
	writeSkill(t, agents, "anfra-write-aql", stamped("0.14.0"))
	writeSkill(t, agents, "aql", "Holistics' AQL.\n") // another vendor's, of a name both use
	claude := filepath.Join(env.Home, ".claude/skills")
	writeSkill(t, claude, "setup-repo", "Run `anfra version`.\n")                                    // the old names
	writeSkill(t, filepath.Join(env.Home, ".cursor/skills"), "setup-repo", "Set up a Rails repo.\n") // another vendor's
	writeSkill(t, filepath.Join(repoDir, ".agents/skills"), "anfra-aml", "Write AML.\n")             // no stamp

	got := Copies(env.Env, repoDir)
	for i := range got {
		got[i].path = ""
	}
	want := []Copy{
		{Dir: "~/.claude/skills", Versions: map[string][]string{}, OldNames: true},
		{Dir: "~/.agents/skills", Versions: map[string][]string{"0.15.0": {"anfra-aql"}, "0.14.0": {"anfra-write-aql"}}},
		{Dir: ".agents/skills", Versions: map[string][]string{"": {"anfra-aml"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if o := got[1].Oldest(); o != "0.14.0" {
		t.Errorf("oldest: %q", o)
	}
	if o := got[2].Oldest(); o != "" {
		t.Errorf("oldest without a stamp: %q", o)
	}
}

func TestStatus(t *testing.T) {
	for _, tc := range []struct {
		name          string
		versions      map[string][]string
		known         bool
		status, about string
	}{
		{"latest", map[string][]string{"0.16.0": {"anfra-aml", "anfra-aql"}}, true, "latest", "0.16.0"},
		{"newer than the check", map[string][]string{"0.16.1": {"anfra-aml"}}, true, "latest", "0.16.1"},
		{"behind", map[string][]string{"0.15.0": {"anfra-aml"}}, true, "behind", "0.15.0"},
		{"no stamp", map[string][]string{"": {"anfra-aml"}}, true, "behind", "no version"},
		{"mixed, oldest first", map[string][]string{
			"0.16.0": {"anfra-aml", "anfra-aql", "anfra-build-data-app", "anfra-setup-repo"},
			"0.15.0": {"anfra-write-aql", "anfra-run-aql"},
			"":       {"anfra-lookup-values"},
		}, true, "mixed", "anfra-lookup-values at no version; anfra-run-aql, anfra-write-aql at 0.15.0; 4 skills at 0.16.0"},
		{"the latest unknown", map[string][]string{"0.15.0": {"anfra-aml"}}, false, "unknown", "0.15.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, about := status(tc.versions, "0.16.0", tc.known)
			if st != tc.status || about != tc.about {
				t.Errorf("got %q, %q; want %q, %q", st, about, tc.status, tc.about)
			}
		})
	}
}

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"0.16.0", "0.15.9", true},
		{"0.10.0", "0.9.0", true},
		{"0.16.1", "0.16.0", true},
		{"0.16.0", "0.16.0", false},
		{"0.15.0", "0.16.0", false},
		{"v0.16.0", "0.15.0", false},
		{"0.16", "0.15.0", false},
		{"unknown", "0.15.0", false},
	} {
		if got := Newer(tc.a, tc.b); got != tc.want {
			t.Errorf("Newer(%s, %s) = %v", tc.a, tc.b, got)
		}
	}
}

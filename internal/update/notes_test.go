package update

import (
	"slices"
	"testing"
)

// releaseBody is a release's notes as scripts/release-notes.sh writes them.
const releaseBody = `## Changelog

### ⚠ BREAKING CHANGES

* **sdk.host:** MountOptions.baseHref is removed and no <base> is
injected: a Data App is one self-contained file.

### Features

* **cli:** warn about Data App references a host cannot serve ([04e7f2b](https://github.com/holistics/anfra/commit/04e7f2baedeaa4ccfdee2bbd9eb15ff912122335))
* formalize background routines ([bcfecd4](https://github.com/holistics/anfra/commit/bcfecd4e1c3e6ba90bf12e6fd27c1cd317fbbca0))

## Pull requests
* Disable aql v2 by @datbth in https://github.com/holistics/anfra/pull/54


---

**Full Changelog**: https://github.com/holistics/anfra/compare/v0.11.1...v0.12.0
`

func TestChangelog(t *testing.T) {
	want := []Line{
		{Heading: true, Text: "⚠ BREAKING CHANGES"},
		// A change that wraps onto a second line is one change.
		{Text: "sdk.host: MountOptions.baseHref is removed and no <base> is injected: a Data App is one self-contained file."},
		{Heading: true, Text: "Features"},
		{Text: "cli: warn about Data App references a host cannot serve"},
		{Text: "formalize background routines"},
	}
	if got := changelog(releaseBody); !slices.Equal(got, want) {
		t.Errorf("changelog:\n got  %+v\n want %+v", got, want)
	}
}

func TestChangelogWithoutSection(t *testing.T) {
	// Notes written by hand, or by GitHub alone, have no changelog to show.
	if got := changelog("## What's Changed\n* a change by @someone\n"); len(got) != 0 {
		t.Errorf("changelog without a section = %+v, want none", got)
	}
}

func TestNotesBetween(t *testing.T) {
	rels := []ghRelease{
		{TagName: "v0.12.0"},
		{TagName: "v0.13.0-rc.1", Prerelease: true},
		{TagName: "v0.11.1"},
		{TagName: "v0.11.0", Draft: true},
		{TagName: "v0.10.2"},
		{TagName: "v0.10.0"},
		{TagName: "anfra-v0.8.0"},
		{TagName: "nightly"},
	}
	versions := func(from, to string) []string {
		var out []string
		for _, n := range notesBetween(rels, from, to) {
			out = append(out, n.Version)
		}
		return out
	}

	// After from, up to and including to, newest first; drafts and prereleases skipped.
	if got, want := versions("0.10.0", "0.11.1"), []string{"0.11.1", "0.10.2"}; !slices.Equal(got, want) {
		t.Errorf("0.10.0 -> 0.11.1: %q, want %q", got, want)
	}
	// Legacy anfra-v tags count too.
	if got, want := versions("0.7.0", "0.10.0"), []string{"0.10.0", "0.8.0"}; !slices.Equal(got, want) {
		t.Errorf("0.7.0 -> 0.10.0: %q, want %q", got, want)
	}
	if got := versions("0.12.0", "0.12.0"); len(got) != 0 {
		t.Errorf("0.12.0 -> 0.12.0: %q, want none", got)
	}
}

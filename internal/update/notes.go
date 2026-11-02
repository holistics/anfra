package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/holistics/anfra/internal/cliui"
	"golang.org/x/mod/semver"
)

// ReleasesURL is the page listing every release and its notes.
const ReleasesURL = "https://github.com/" + repoSlug + "/releases"

// Notes is what one release changed, from its notes on GitHub.
type Notes struct {
	Version string
	URL     string // the release's page
	// Changelog is the release's changelog as plain text, empty when its notes
	// have none.
	Changelog []Line
}

// Line is a line of a changelog: a section's heading ("Features"), or a change
// and, when it names one, the part of anfra it touches, its scope ("cli").
type Line struct {
	Heading bool
	Scope   string
	Text    string
}

// maxChangelog bounds how much of CHANGELOG.md ReleaseNotes reads.
const maxChangelog = 4 << 20

// ReleaseNotes fetches the notes of the releases after from, up to and
// including to, newest first: everything an update from one to the other
// brings, the releases it skips included.
//
// It reads them from CHANGELOG.md at to's tag, of which each release's notes
// are a section (scripts/release-notes.sh), rather than from GitHub's API,
// which allows 60 anonymous calls an hour (see Latest).
func ReleaseNotes(ctx context.Context, from, to string) ([]Notes, error) {
	resp, err := get(ctx, fmt.Sprintf("%s/%s/v%s/CHANGELOG.md", rawBase, repoSlug, url.PathEscape(tagVersion(to))), lookupTimeout)
	if err != nil {
		return nil, fmt.Errorf("fetch the changelog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch the changelog: GitHub returned %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxChangelog))
	if err != nil {
		return nil, fmt.Errorf("read the changelog: %w", err)
	}
	return notesBetween(string(b), from, to), nil
}

// versionHeading is a release's heading in CHANGELOG.md, as in
// "## [0.15.0](https://github.com/...) (2026-10-30)".
var versionHeading = regexp.MustCompile(`^## \[([^\]]+)\]`)

// notesBetween keeps the sections of CHANGELOG.md for the releases after from,
// up to and including to, newest first.
func notesBetween(changes, from, to string) []Notes {
	lo, hi := canonicalVersion(from), canonicalVersion(to)
	var out []Notes
	// Each "## " heading starts a section; the text before the first is none.
	for _, section := range strings.Split("\n"+changes, "\n## ")[1:] {
		heading, body, _ := strings.Cut(section, "\n")
		m := versionHeading.FindStringSubmatch("## " + heading)
		if m == nil {
			continue
		}
		v := canonicalVersion(m[1])
		if !semver.IsValid(v) || semver.Prerelease(v) != "" || semver.Compare(v, lo) <= 0 || semver.Compare(v, hi) > 0 {
			continue
		}
		ver := tagVersion(m[1])
		out = append(out, Notes{
			Version:   ver,
			URL:       fmt.Sprintf("%s/%s/releases/tag/v%s", webBase, repoSlug, url.PathEscape(ver)),
			Changelog: changelog(body),
		})
	}
	slices.SortFunc(out, func(a, b Notes) int {
		return semver.Compare(canonicalVersion(b.Version), canonicalVersion(a.Version))
	})
	return out
}

// commitLink is the link to a change's commit at the end of its changelog line.
var commitLink = regexp.MustCompile(`\s*\(\[[0-9a-f]+\]\([^)]*\)\)`)

// scopePrefix is a change's bullet and its bold scope, as in "* **cli:** ...".
var scopePrefix = regexp.MustCompile(`^([*-] )\*\*([^*]+):\*\* `)

// changelog reads a release's section of CHANGELOG.md, without its heading,
// into plain lines: its "###" headings, and a line per change with its scope
// apart, without the commit's link and with the lines a long change wraps onto
// joined back.
func changelog(section string) []Line {
	var out []Line
	for l := range strings.SplitSeq(section, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		l = commitLink.ReplaceAllString(l, "")
		var scope string
		if m := scopePrefix.FindStringSubmatch(l); m != nil {
			scope, l = m[2], m[1]+l[len(m[0]):]
		}
		l = strings.ReplaceAll(l, "**", "")
		switch {
		case strings.HasPrefix(l, "### "):
			out = append(out, Line{Heading: true, Text: strings.TrimPrefix(l, "### ")})
		case strings.HasPrefix(l, "* "), strings.HasPrefix(l, "- "):
			out = append(out, Line{Scope: scope, Text: l[2:]})
		case len(out) > 0 && !out[len(out)-1].Heading:
			out[len(out)-1].Text += " " + l
		default:
			out = append(out, Line{Text: l})
		}
	}
	return out
}

// RenderNotes writes what the newest limit releases in notes changed, then a
// link to the rest: per release, a heading and its page, then its changelog's
// sections, a change per line wrapped under its text to width (0 doesn't wrap).
// The zero styles write plain text. The notes come from GitHub: they are written
// as text, never as escapes.
func RenderNotes(w io.Writer, notes []Notes, limit int, s cliui.TerminalStyles, width int) {
	for _, n := range notes[:min(len(notes), limit)] {
		fmt.Fprintf(w, "\n%s %s\n%s\n", s.Heading.Render("What's new in anfra"),
			s.Success.Inherit(s.Heading).Render(cliui.SafeText(n.Version)), s.Muted.Render(n.URL))
		for _, l := range n.Changelog {
			text := cliui.SafeText(l.Text)
			switch {
			case !l.Heading:
				if l.Scope != "" {
					text = s.Accent.Render(cliui.SafeText(l.Scope)+":") + " " + text
				}
				writeWrapped(w, "    "+s.Muted.Render("•")+" ", text, width)
			case strings.Contains(strings.ToLower(text), "breaking"):
				fmt.Fprintf(w, "\n  %s\n", s.Warning.Inherit(s.Heading).Render(text))
			default:
				fmt.Fprintf(w, "\n  %s\n", s.Heading.Render(text))
			}
		}
	}
	if more := len(notes) - limit; more > 0 {
		noun := "releases"
		if more == 1 {
			noun = "release"
		}
		fmt.Fprintf(w, "\n%s\n", s.Muted.Render(fmt.Sprintf("And %d earlier %s: %s", more, noun, ReleasesURL)))
	}
}

// writeWrapped writes text after mark, wrapped to width under the text rather
// than the mark; 0 doesn't wrap.
func writeWrapped(w io.Writer, mark, text string, width int) {
	indent := ansi.StringWidth(mark)
	for i, l := range cliui.Wrap(text, width, indent) {
		if i > 0 {
			mark = strings.Repeat(" ", indent)
		}
		fmt.Fprintln(w, mark+l)
	}
}

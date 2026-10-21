package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/shared/jsonkit"
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

// ghRelease is the part of a GitHub release the notes are read from.
type ghRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	HTMLURL    string `json:"html_url"`
	Body       string `json:"body"`
}

// ReleaseNotes fetches the notes of the releases after from, up to and
// including to, newest first: everything an update from one to the other
// brings, the releases it skips included.
func ReleaseNotes(ctx context.Context, from, to string) ([]Notes, error) {
	// The newest 100 releases: more than an update ever skips.
	resp, err := get(ctx, fmt.Sprintf("%s/repos/%s/releases?per_page=100", apiBase, repoSlug), "application/vnd.github+json", lookupTimeout)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list releases: GitHub returned %s", resp.Status)
	}
	var rels []ghRelease
	if err := jsonkit.UnmarshalRead(resp.Body, &rels); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}
	return notesBetween(rels, from, to), nil
}

// notesBetween keeps the published releases after from, up to and including
// to, newest first.
func notesBetween(rels []ghRelease, from, to string) []Notes {
	lo, hi := canonicalVersion(from), canonicalVersion(to)
	var out []Notes
	for _, r := range rels {
		v := canonicalVersion(r.TagName)
		if r.Draft || r.Prerelease || !semver.IsValid(v) || semver.Compare(v, lo) <= 0 || semver.Compare(v, hi) > 0 {
			continue
		}
		out = append(out, Notes{Version: tagVersion(r.TagName), URL: r.HTMLURL, Changelog: changelog(r.Body)})
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

// changelog reads the "## Changelog" section of a release's notes, as
// scripts/release-notes.sh writes it, into plain lines: its "###" headings, and
// a line per change with its scope apart, without the commit's link and with
// the lines a long change wraps onto joined back. The pull requests and the
// rest that follow the section are left out.
func changelog(body string) []Line {
	var out []Line
	in := false
	for l := range strings.SplitSeq(body, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "## ") {
			if in {
				break
			}
			in = l == "## Changelog"
			continue
		}
		if !in || l == "" {
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
	if width > indent+20 {
		text = ansi.Wrap(text, width-indent, "")
	}
	for i, l := range strings.Split(text, "\n") {
		if i > 0 {
			mark = strings.Repeat(" ", indent)
		}
		fmt.Fprintln(w, mark+l)
	}
}

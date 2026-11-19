package telemetry

import (
	"regexp"
	"strings"
)

// StableSpan is a span users can rely on: it keeps its name, and each of its
// Attributes keeps its name and meaning. Every other span and attribute is
// development, and may change in any release
// (docs/designs/telemetry.md, stable span names and attributes).
type StableSpan struct {
	// Name is the span's name, where <...> stands for any part: anfra.op.<op>.
	Name string
	// Attributes are its stable attributes. A span whose name matches several
	// stable spans has the attributes of each.
	Attributes []Key
}

// StableSpans are the stable spans. The usage page lists them, and a test
// checks it lists these.
var StableSpans = []StableSpan{
	{Name: "anfra <command path>", Attributes: []Key{RepoID}},
	{Name: "anfra.op.<op>", Attributes: []Key{Op, ErrorType, RepoID}},
	{Name: "anfra.op.core.query", Attributes: []Key{QueryID, QueryKind, Dataset, DataSource, DBSystem, QueryText}},
}

// Matches reports whether a span named name is s.
func (s StableSpan) Matches(name string) bool {
	return stablePattern(s.Name).MatchString(name)
}

// StableAttributes are the stable attributes of a span named name, each once;
// none when it isn't stable.
func StableAttributes(name string) []Key {
	var keys []Key
	seen := map[Key]bool{}
	for _, s := range StableSpans {
		if !s.Matches(name) {
			continue
		}
		for _, k := range s.Attributes {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	return keys
}

// stablePattern is name as a regexp, each <...> standing for one or more
// characters.
func stablePattern(name string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for {
		i := strings.Index(name, "<")
		j := strings.Index(name, ">")
		if i < 0 || j < i {
			b.WriteString(regexp.QuoteMeta(name))
			break
		}
		b.WriteString(regexp.QuoteMeta(name[:i]) + ".+")
		name = name[j+1:]
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

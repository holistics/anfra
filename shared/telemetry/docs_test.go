package telemetry_test

import (
	"maps"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/telemetry"
)

// The docs' tables of categories say what the registry does: each category's
// variable, values and default, in the design, the usage page and configs.
func TestTheDocsTablesMatchTheRegistry(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		b, err := os.ReadFile("../../" + path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	tableRow := func(doc, v string) string {
		t.Helper()
		m := regexp.MustCompile("(?m)^\\|.*`" + v + "`.*\\|$").FindString(doc)
		if m == "" {
			t.Errorf("no row for %s", v)
		}
		return m
	}
	quoted := func(levels []telemetry.Level, sep string) string {
		var q []string
		for _, l := range levels {
			q = append(q, "`"+string(l)+"`")
		}
		return strings.Join(q, sep)
	}
	orQuoted := func(levels []telemetry.Level) string {
		q := quoted(levels, ", ")
		if i := strings.LastIndex(q, ", "); i >= 0 {
			q = q[:i] + " or " + q[i+2:]
		}
		return q
	}
	for _, path := range []string{"docs/designs/telemetry.md", "docs/usage/telemetry.md"} {
		doc := read(path)
		for _, d := range telemetry.Categories {
			want := "| " + quoted(d.Levels, ", ") + " | `" + string(d.Default) + "` |"
			if row := tableRow(doc, d.Var); !strings.HasSuffix(row, want) {
				t.Errorf("%s: %s's row = %q, want it to end %q", path, d.Var, row, want)
			}
		}
	}
	configs := read("docs/usage/configs.md")
	for _, d := range telemetry.Categories {
		want := orQuoted(d.Levels) + ". Default: `" + string(d.Default) + "`. |"
		if row := tableRow(configs, d.Var); !strings.HasSuffix(row, want) {
			t.Errorf("configs.md: %s's row = %q, want it to end %q", d.Var, row, want)
		}
	}
}

// The tables of stable spans, in the design and the usage page, list the
// registry's: each span's name and its stable attributes, no more, no fewer.
func TestTheStableSpansTablesMatchTheRegistry(t *testing.T) {
	want := map[string]string{}
	for _, s := range telemetry.StableSpans {
		var q []string
		for _, k := range s.Attributes {
			q = append(q, "`"+k.String()+"`")
		}
		want[s.Name] = strings.Join(q, ", ")
	}
	firstCode := regexp.MustCompile("`([^`]+)`")
	for _, path := range []string{"docs/designs/telemetry.md", "docs/usage/telemetry.md"} {
		b, err := os.ReadFile("../../" + path)
		if err != nil {
			t.Fatal(err)
		}
		_, table, ok := strings.Cut(string(b), "| Span | Stable attributes |\n|---|---|\n")
		if !ok {
			t.Errorf("%s: no table of stable spans", path)
			continue
		}
		table, _, _ = strings.Cut(table, "\n\n")
		got := map[string]string{}
		for row := range strings.SplitSeq(table, "\n") {
			cells := strings.Split(strings.Trim(row, "| "), " | ")
			m := firstCode.FindStringSubmatch(cells[0])
			if len(cells) != 2 || m == nil {
				t.Errorf("%s: can't read the row %q", path, row)
				continue
			}
			got[m[1]] = cells[1]
		}
		if !maps.Equal(got, want) {
			t.Errorf("%s: stable spans = %v, want %v (telemetry.StableSpans)", path, got, want)
		}
	}
}

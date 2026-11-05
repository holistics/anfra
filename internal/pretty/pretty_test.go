package pretty

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// result is a command's result, as a CLI-only command's is shaped.
type result struct {
	Message string   `json:"message"`
	Name    string   `json:"name"`
	Note    string   `json:"note,omitempty"`
	Owner   *owner   `json:"owner"`
	Tags    []string `json:"tags"`
	Script  string   `json:"script" pretty:",block"`
	Files   []file   `json:"files"`
	Hints   []string `json:"hints,omitempty"`
}

type owner struct {
	Email string `json:"email"`
	Admin bool   `json:"admin"`
}

type file struct {
	Path    string `json:"path"`
	Changed bool   `json:"changed" pretty:",tone=true:good"`
}

// The conventions: the message first, the hints last; scalars as aligned
// key/values, an empty one marked omitempty left out; a list of scalars one
// a line; a nil object as <none>; text as a block; a list of objects
// as a table.
func TestConventions(t *testing.T) {
	var out bytes.Buffer
	err := Render(&out, result{
		Message: "Set up sales.",
		Name:    "sales",
		Tags:    []string{"a", "b, c"},
		Script:  "run",
		Files:   []file{{"models/.gitkeep", true}, {".gitignore", false}},
		Hints:   []string{"Run `anfra validate`."},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := `Set up sales.

Name:   sales
Owner:  <none>
Tags:   a
        b, c

Script:
  run

Files:
  PATH              CHANGED
  models/.gitkeep   true
  .gitignore        false

Hints:
  Run ` + "`anfra validate`" + `.
`
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

// A list joined in a cell is a CSV record, quoted only where CSV needs it.
func TestCSVField(t *testing.T) {
	for in, want := range map[string]string{
		"Laptop":          "Laptop",
		"Smith, John":     `"Smith, John"`,
		`a "quoted" word`: `"a ""quoted"" word"`,
		" leading":        `" leading"`,
	} {
		if got := csvField(in, ','); got != want {
			t.Errorf("csvField(%q) = %s, want %s", in, got, want)
		}
	}
	if got := csvField("behind by 2", ' '); got != `"behind by 2"` {
		t.Errorf("space-separated: %s", got)
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local)
	for d, want := range map[time.Duration]string{
		time.Hour:           "today",
		24 * time.Hour:      "yesterday",
		5 * 24 * time.Hour:  "5 days ago",
		60 * 24 * time.Hour: "4 Sep 2026",
	} {
		if got := relativeTime(now.Add(-d), now); got != want {
			t.Errorf("%v ago: %q, want %q", d, got, want)
		}
	}
}

// The day before a DST change is still yesterday, though its midnight is 23
// hours back.
func TestRelativeTimeAcrossDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	local := time.Local
	time.Local = ny
	t.Cleanup(func() { time.Local = local })
	now := time.Date(2026, 3, 9, 10, 0, 0, 0, ny) // spring forward on 8 March
	if got := relativeTime(time.Date(2026, 3, 8, 10, 0, 0, 0, ny), now); got != "yesterday" {
		t.Errorf("got %q, want yesterday", got)
	}
}

// A list of paths with the tree hint is drawn as tree draws it: folders first,
// each sorted, a folder of one entry on its parent's line; in ASCII without
// UTF-8.
func TestTree(t *testing.T) {
	paths := []string{"models/orders.aml", "apps/sales/index.html", "apps/sales/chart.js", "datasets/ecommerce.aml", "models/customers.aml", "README.aml"}
	draw := func(utf8 bool) string {
		var b strings.Builder
		for _, l := range treeLines(paths, utf8) {
			b.WriteString(l.plain() + "\n")
		}
		return b.String()
	}
	want := `├── apps/sales/
│   ├── chart.js
│   └── index.html
├── datasets/ecommerce.aml
├── models/
│   ├── customers.aml
│   └── orders.aml
└── README.aml
`
	if got := draw(true); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	ascii := "|-- apps/sales/\n|   |-- chart.js\n|   `-- index.html\n"
	if got := draw(false); !strings.HasPrefix(got, ascii) {
		t.Errorf("ASCII:\n%s", got)
	}
}

func TestTitleCase(t *testing.T) {
	for name, want := range map[string]string{"checked_at": "Checked At", "url": "URL", "org_id": "Org ID", "deprecated_names": "Deprecated Names"} {
		if got := titleCase(name); got != want {
			t.Errorf("titleCase(%q) = %q, want %q", name, got, want)
		}
	}
}

package pretty

import (
	"bytes"
	"encoding/json/jsontext"
	"reflect"
	"strings"
	"testing"
)

// A result shaped as a query's: rows of raw JSON, inlined from a field of
// their own, labelled from a path from the top; timings by their view.
type rowsResult struct {
	SQL     string      `json:"sql" pretty:"-"`
	Columns []rowColumn `json:"columns" pretty:"-"`
	Result  rowsOf      `json:"result" pretty:",inline"`
	HasMore bool        `json:"has_more"`
	Timings timings     `json:"timings" pretty:",view=timings"`
}

type rowColumn struct {
	Label string `json:"label"`
}

type rowsOf struct {
	Fields  []string         `json:"fields" pretty:"-"`
	Records []jsontext.Value `json:"records" pretty:"Result,labels=columns.label"`
}

type timings struct {
	Steps []struct {
		Step    string `json:"step"`
		StartMs int64  `json:"start_ms"`
	} `json:"steps"`
	EndMs int64 `json:"end_ms"`
}

func sampleRows() rowsResult {
	r := rowsResult{SQL: "select", Columns: []rowColumn{{"Status"}, {"Count of Id"}},
		Result:  rowsOf{Fields: []string{"s", "c"}, Records: []jsontext.Value{jsontext.Value(`["delivered","1204"]`), jsontext.Value(`["New York","87"]`), jsontext.Value(`["refunded",null]`)}},
		HasMore: true, Timings: timings{EndMs: 64}}
	for _, s := range []struct {
		name  string
		start int64
	}{{"compiling_query", 0}, {"executing_query", 12}, {"executing_query.waiting", 15}, {"fetching_query_result", 52}} {
		r.Timings.Steps = append(r.Timings.Steps, struct {
			Step    string `json:"step"`
			StartMs int64  `json:"start_ms"`
		}{s.name, s.start})
	}
	return r
}

func TestRowsGrid(t *testing.T) {
	t.Setenv("LANG", "en_US.UTF-8")
	var b bytes.Buffer
	if err := Render(&b, sampleRows(), 0); err != nil {
		t.Fatal(err)
	}
	want := `Result:
  ┌───────────┬─────────────┐
  │ Status    │ Count of Id │
  ├───────────┼─────────────┤
  │ delivered │        1204 │
  │ New York  │          87 │
  │ refunded  │      <none> │
  └───────────┴─────────────┘

Has More:  true

Timings:
  compiling_query         12 ms
  executing_query         40 ms
    waiting               37 ms
  fetching_query_result   12 ms
  total                   64 ms
`
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", &b, want)
	}
}

// Without a UTF-8 terminal, the grid is drawn in ASCII; wider than the
// terminal, it's one block a row.
func TestRowsFallbacks(t *testing.T) {
	t.Setenv("LANG", "C")
	var b bytes.Buffer
	_ = Render(&b, sampleRows(), 0)
	if !strings.Contains(b.String(), "+-----------+-------------+\n  | Status    | Count of Id |") {
		t.Errorf("ASCII:\n%s", &b)
	}
	b.Reset()
	_ = Render(&b, sampleRows(), 20)
	if !strings.Contains(b.String(), "Result:\n  -[ ROW 1 ]-\n  Status        delivered\n  Count of Id   1204\n  -[ ROW 2 ]-") {
		t.Errorf("narrow:\n%s", &b)
	}
}

// A column of numbers is right-aligned in a listing's table too.
func TestNumericColumns(t *testing.T) {
	got := numericColumns([][]prettyCell{
		{{{text: "a"}}, {{text: "1"}}, {{text: "1"}}},
		{{{text: "b"}}, {{text: "22.5"}}, {{text: "x"}}},
		{{{text: "c"}}, noneCell, {{text: "3"}}},
	})
	if len(got) != 3 || got[0] || !got[1] || got[2] {
		t.Errorf("numeric %v", got)
	}
}

// Timings' steps are faint, the detail; their total plain, read first.
func TestTimingsStyles(t *testing.T) {
	pv, ok := timingsView(reflect.ValueOf(sampleRows().Timings))
	if !ok {
		t.Fatal("no view")
	}
	last := pv.lines[len(pv.lines)-1]
	if !strings.HasPrefix(last.plain(), "total") || last[0].style != stylePlain {
		t.Errorf("total %q styled %v", last.plain(), last[0].style)
	}
	for _, l := range pv.lines[:len(pv.lines)-1] {
		if l[0].style != styleMuted {
			t.Errorf("step %q styled %v", l.plain(), l[0].style)
		}
	}
}

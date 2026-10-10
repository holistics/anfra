package query

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
)

// memStore is an ExportStore in memory: the file it was given, and how it ended.
type memStore struct {
	filename, contentType string
	buf                   bytes.Buffer
	done, aborted         bool
}

func (s *memStore) Create(_ context.Context, filename, contentType string) (command.Export, error) {
	s.filename, s.contentType = filename, contentType
	return memExport{s}, nil
}

type memExport struct{ s *memStore }

func (e memExport) Write(p []byte) (int, error) { return e.s.buf.Write(p) }
func (e memExport) Done(context.Context) (string, time.Time, error) {
	e.s.done = true
	return "https://exports.example/x", time.Date(2026, 10, 9, 13, 0, 0, 0, time.UTC), nil
}
func (e memExport) Abort(context.Context) error { e.s.aborted = true; return nil }

// canalRows is a run that answers rows, then fields, as canal streams them.
func canalRows(fields []string, rows ...string) rowsFunc {
	return func(each func(canalquery.Row) error) ([]string, error) {
		for _, r := range rows {
			if err := each(canalquery.Row(r)); err != nil {
				return nil, err
			}
		}
		return fields, nil
	}
}

var exportColumns = []anfranode.ExploreColumn{
	{Name: "id", FieldName: "id", Label: "Order ID"},
	{Name: "note", FieldName: "note", Label: "Note"},
}

// A CSV holds the values as the query answers them: a number with every digit,
// a string as itself (quoted where CSV needs it), null as an empty cell, an
// object as its JSON; under a header of labels, names, or none.
func TestExportCSV(t *testing.T) {
	fields := []string{"id", "note", "attrs", "flag"}
	rows := canalRows(fields,
		`[9007199254740993,"a, \"quoted\"\nline",{"b":1},true]`,
		`[2,null,null,false]`,
	)
	body := "9007199254740993,\"a, \"\"quoted\"\"\nline\",\"{\"\"b\"\":1}\",true\n2,,,false\n"
	for _, tc := range []struct {
		name        string
		opts        ExportFormatOptions
		want        string
		contentType string
	}{
		{"labels, by default", ExportFormatOptions{}, "Order ID,Note,attrs,flag\n" + body, "text/csv; charset=utf-8; header=present"},
		{"names", ExportFormatOptions{Header: "names"}, "id,note,attrs,flag\n" + body, "text/csv; charset=utf-8; header=present"},
		{"no header", ExportFormatOptions{Header: "none"}, body, "text/csv; charset=utf-8; header=absent"},
		{"a byte-order mark, before the header", ExportFormatOptions{BOM: true}, "\xEF\xBB\xBFOrder ID,Note,attrs,flag\n" + body, "text/csv; charset=utf-8; header=present"},
		{"a byte-order mark, no header", ExportFormatOptions{Header: "none", BOM: true}, "\xEF\xBB\xBF" + body, "text/csv; charset=utf-8; header=absent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &memStore{}
			link, err := writeCSV(context.Background(), store, "sales.csv", tc.opts, exportColumns, rows)
			if err != nil {
				t.Fatal(err)
			}
			if store.buf.String() != tc.want {
				t.Errorf("file =\n%q\nwant\n%q", store.buf.String(), tc.want)
			}
			if store.contentType != tc.contentType || store.filename != "sales.csv" || !store.done || store.aborted {
				t.Errorf("store = %q %q done %v aborted %v", store.filename, store.contentType, store.done, store.aborted)
			}
			want := command.ExportLink{URL: "https://exports.example/x", Filename: "sales.csv", Format: "csv", RowCount: 2,
				ExpiresAt: time.Date(2026, 10, 9, 13, 0, 0, 0, time.UTC)}
			if link != want {
				t.Errorf("link = %+v, want %+v", link, want)
			}
		})
	}
}

// A query that fails while its rows arrive fails the export, as it would fail
// query, and its file is discarded, never finished.
func TestExportFailingQueryAbortsTheFile(t *testing.T) {
	store := &memStore{}
	failing := func(each func(canalquery.Row) error) ([]string, error) {
		_ = each(canalquery.Row(`[1]`))
		return nil, &canalquery.Error{Message: "relation does not exist", Scope: "User"}
	}
	_, err := writeCSV(context.Background(), store, "sales.csv", ExportFormatOptions{}, nil, failing)
	if !errors.Is(err, errcode.QueryFailed) {
		t.Errorf("err = %v, want query_failed", err)
	}
	if !store.aborted || store.done {
		t.Errorf("the file was not discarded: done %v aborted %v", store.done, store.aborted)
	}
}

// An export is refused as query refuses the same query; and without a store,
// before it runs.
func TestExportRefusals(t *testing.T) {
	sql := ExportInput{QueryInput: QueryInput{Query: "select 1", Lang: "sql", DataSource: "demo"}, Timezone: "Asia/Ho_Chi_Minh"}
	if err := sql.check(); field(err) != "timezone" {
		t.Errorf("a time zone on SQL: %v, want refused on timezone", err)
	}
	if err := (ExportInput{QueryInput: QueryInput{Query: "q"}}).check(); field(err) != "dataset" {
		t.Errorf("no dataset: %v, want refused on dataset", err)
	}
	if err := command.RequireExports(command.CommandContext{}); !errors.Is(err, errcode.ExportsUnavailable) {
		t.Errorf("no store: %v, want exports_unavailable", err)
	}
}

func field(err error) string {
	if v, ok := apperr.DetailsOf(err, apperr.ValidationFailed); ok && len(v.Violations) == 1 {
		return v.Violations[0].Field
	}
	return ""
}

// A file is named as asked, or for what it exports, with its format's
// extension. A name is one name, not a path; any script is welcome, and what a
// system forbids in a name is the browser's to replace. A blank one is unset.
func TestExportFilename(t *testing.T) {
	for _, tc := range []struct{ asked, exported, want string }{
		{"", "sales", "sales.csv"},
		{"   ", "sales", "sales.csv"},
		{"", "ecommerce.orders", "ecommerce.orders.csv"},
		{"", "doanh thu/quý 1", "doanh thu_quý 1.csv"},
		{"", "../..", "export.csv"},
		{"", "", "export.csv"},
		{"Báo cáo tháng 10", "sales", "Báo cáo tháng 10.csv"},
		{"  report  ", "sales", "report.csv"},
		{"Q1: what?", "sales", "Q1: what?.csv"},
		{"orders.CSV", "sales", "orders.CSV"},
		{"orders.tsv", "sales", "orders.tsv.csv"},
	} {
		if got := (ExportInput{Filename: tc.asked}).filename(tc.exported, "csv"); got != tc.want {
			t.Errorf("filename %q for %q = %q, want %q", tc.asked, tc.exported, got, tc.want)
		}
	}

	in := ExportInput{QueryInput: QueryInput{Query: "q", Dataset: "d"}}
	for _, bad := range []string{"..", "a/b", `a\b`, "tab\there", strings.Repeat("x", 256)} {
		in.Filename = bad
		if err := in.check(); field(err) != "filename" {
			t.Errorf("filename %q: %v, want refused on filename", bad, err)
		}
	}
	for _, good := range []string{"", " ", "Báo cáo", "Q1: what?"} {
		in.Filename = good
		if err := in.check(); err != nil {
			t.Errorf("filename %q was refused: %v", good, err)
		}
	}
}

// The spool a header row needs is no file on disk, even while the export runs:
// nothing is left of it, even by a crash.
func TestExportSpoolLeavesNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("an open file cannot be removed on Windows")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	empty := func(when string) {
		if es, _ := os.ReadDir(tmp); len(es) != 0 {
			t.Errorf("%s: %d files in the temp directory, want none", when, len(es))
		}
	}
	rows := func(each func(canalquery.Row) error) ([]string, error) {
		if err := each(canalquery.Row(`[1]`)); err != nil {
			return nil, err
		}
		empty("while rows arrive")
		return []string{"id"}, nil
	}
	if _, err := writeCSV(context.Background(), &memStore{}, "sales.csv", ExportFormatOptions{}, nil, rows); err != nil {
		t.Fatal(err)
	}
	empty("after the export")
}

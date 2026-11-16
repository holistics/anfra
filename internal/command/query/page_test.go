package query

import (
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// read streams header and rows through a page's reading, as canal-query would,
// and answers what was passed on.
func read(p *pageReading, header canalquery.Header, rows ...string) (canalquery.Header, []string) {
	var got canalquery.Header
	var passed []string
	h := p.handler(canalquery.Handler{
		Header: func(hd canalquery.Header) error { got = hd; return nil },
		Row:    func(r canalquery.Row) error { passed = append(passed, string(r)); return nil },
	})
	_ = h.Header(header)
	for _, r := range rows {
		_ = h.Row(canalquery.Row(r))
	}
	return got, passed
}

// A query's page keeps its rows, and the row past them says there is a next
// page; a page that ends the result has none.
func TestAQuerysPage(t *testing.T) {
	page := &Pagination{Page: 1, PageSize: 2}
	compiled := &anfranode.CompileToSQLResult{RowPage: &anfranode.RowPage{Rows: 2}}
	header := canalquery.Header{Fields: []string{"name"}}

	p := newPageReading(page, compiled)
	if _, rows := read(p, header, `["a"]`, `["b"]`, `["c"]`); strings.Join(rows, ",") != `["a"],["b"]` || !p.hasMore() || p.pivot() != nil {
		t.Errorf("a page with more: %v, more %v", rows, p.hasMore())
	}
	p = newPageReading(page, compiled)
	if _, rows := read(p, header, `["a"]`, `["b"]`); len(rows) != 2 || p.hasMore() {
		t.Errorf("the last page, full: %v, more %v", rows, p.hasMore())
	}
}

// A pivot's page keeps every row, without its two count columns, which say
// how big its grid is: whether there are rows after it, and columns after its
// page of them.
func TestAPivotsPage(t *testing.T) {
	compiled := &anfranode.CompileToSQLResult{PivotPage: &anfranode.PivotPage{RowCountColumn: "__pivot_row_count__", ColumnCountColumn: "__pivot_column_count__"}}
	header := canalquery.Header{
		Fields: []string{"__pivot_row__", "__pivot_row_count__", "region", "__pivot_column_count__"},
		Types:  []string{"INT8", "INT8", "TEXT", "INT8"},
	}
	rows := []string{`["0","5","n","3"]`, `["1","5","EU","3"]`, `["1","5","US","3"]`, `["2","5","EU","3"]`}

	p := newPageReading(&Pagination{Page: 1, PageSize: 2, Columns: &ColumnPage{Page: 2, PageSize: 2}}, compiled)
	got, passed := read(p, header, rows...)
	if strings.Join(got.Fields, ",") != "__pivot_row__,region" || strings.Join(got.Types, ",") != "INT8,TEXT" {
		t.Errorf("header: %+v", got)
	}
	if len(passed) != 4 || passed[1] != `["1","EU"]` {
		t.Errorf("rows: %v", passed)
	}
	if !p.hasMore() || *p.pivot() != (PivotPage{Rows: 5, Columns: 3, HasMoreColumns: false}) {
		t.Errorf("page 1 of 5 rows by 2, columns 2 of 3 by 2: more %v, %+v", p.hasMore(), *p.pivot())
	}

	p = newPageReading(&Pagination{Page: 3, PageSize: 2}, compiled)
	if read(p, header, rows[0]); p.hasMore() || p.pivot().Rows != 5 || p.pivot().HasMoreColumns {
		t.Errorf("page 3 of 5 rows by 2, every column: more %v, %+v", p.hasMore(), *p.pivot())
	}

	p = newPageReading(&Pagination{Page: 1, PageSize: 2}, compiled)
	if read(p, header); p.hasMore() || *p.pivot() != (PivotPage{}) {
		t.Errorf("a pivot with no rows: more %v, %+v", p.hasMore(), *p.pivot())
	}
}

// From an anfra-node that compiles a page as pageSize rows and says nothing
// of it, a full page may have more; and with no page, there's none.
func TestAPageFromAnOlderAnfraNode(t *testing.T) {
	header := canalquery.Header{Fields: []string{"name"}}
	p := newPageReading(&Pagination{Page: 1, PageSize: 2}, &anfranode.CompileToSQLResult{})
	if _, rows := read(p, header, `["a"]`, `["b"]`); len(rows) != 2 || !p.hasMore() {
		t.Errorf("a full page: %v, more %v", rows, p.hasMore())
	}
	p = newPageReading(nil, nil)
	if read(p, header, `["a"]`); p.hasMore() || p.pivot() != nil {
		t.Error("every row: more")
	}
}

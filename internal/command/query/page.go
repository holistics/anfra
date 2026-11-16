package query

import (
	"encoding/json/jsontext"
	"fmt"
	"slices"
	"strconv"

	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/jsonkit"
)

// PivotPage is a page of a pivot, as its answer says it: how big the pivot's
// grid is, so a caller can number its pages, and whether a page of columns
// has more after it.
type PivotPage struct {
	Rows           int  `json:"rows" doc:"how many rows the pivot's grid has, in all of its pages, the grand total not counted"`
	Columns        int  `json:"columns" doc:"how many columns the pivot's grid has, the total column not counted"`
	HasMoreColumns bool `json:"has_more_columns,omitempty" doc:"with input.pagination.columns: whether there are columns after this page's"`
}

// pageReading reads a page's rows as they stream, as anfra-node compiled the
// page, and records what it found: a query's page answers one row past it,
// which says there is a next page and isn't passed on; a pivot's page answers
// every row of its grid's rows, each with the grid's counts in two columns,
// which are read, and taken out of the header and of every row. A state local
// to a query's run: hasMore and pivot are derived from it once the rows end.
type pageReading struct {
	asked     *Pagination          // the page asked for; nil: every row
	rowPage   *anfranode.RowPage   // how anfra-node compiled a query's page
	pivotPage *anfranode.PivotPage // how anfra-node compiled a pivot's page

	kept          int    // the rows passed on
	past          bool   // a query's page answered the row past it
	countsAt      [2]int // a pivot's count columns in the header: its rows', its columns'
	rows, columns int    // a pivot's grid's counts, from its first row
	countsRead    bool
}

// newPageReading reads the page asked for as compiled says to: nil compiled
// for SQL, whose page anfra-node doesn't compile.
func newPageReading(asked *Pagination, compiled *anfranode.CompileToSQLResult) *pageReading {
	p := &pageReading{asked: asked, countsAt: [2]int{-1, -1}}
	if compiled != nil && asked != nil {
		p.rowPage, p.pivotPage = compiled.RowPage, compiled.PivotPage
	}
	return p
}

// handler is h, passed only the page's rows, and for a pivot, without its
// count columns.
func (p *pageReading) handler(h canalquery.Handler) canalquery.Handler {
	header, row := h.Header, h.Row
	switch {
	case p.rowPage != nil:
		h.Row = func(r canalquery.Row) error {
			if p.kept == p.rowPage.Rows {
				p.past = true
				return nil
			}
			p.kept++
			return row(r)
		}
	case p.pivotPage != nil:
		h.Header = func(hd canalquery.Header) error {
			p.countsAt = [2]int{slices.Index(hd.Fields, p.pivotPage.RowCountColumn), slices.Index(hd.Fields, p.pivotPage.ColumnCountColumn)}
			return header(withoutColumns(hd, p.countsAt))
		}
		h.Row = func(r canalquery.Row) error {
			r, counts, err := takeCounts(r, p.countsAt)
			if err != nil {
				return err
			}
			if !p.countsRead {
				p.rows, p.columns, p.countsRead = counts[0], counts[1], true
			}
			p.kept++
			return row(r)
		}
	default:
		h.Row = func(r canalquery.Row) error {
			p.kept++
			return row(r)
		}
	}
	return h
}

// hasMore says whether a page has more after it: a query's, when it answered
// the row past it; a pivot's, when its grid has rows past it. A page from an
// anfra-node that says neither is full: there may be more. Every row, unpaged:
// none.
func (p *pageReading) hasMore() bool {
	switch {
	case p.asked == nil:
		return false
	case p.rowPage != nil:
		return p.past
	case p.pivotPage != nil:
		return max(p.asked.Page, 1)*p.asked.PageSize < p.rows
	default:
		return p.kept == p.asked.PageSize
	}
}

// pivot is a pivot's page as its answer says it; nil for any other.
func (p *pageReading) pivot() *PivotPage {
	if p.pivotPage == nil {
		return nil
	}
	out := &PivotPage{Rows: p.rows, Columns: p.columns}
	if c := p.asked.Columns; c != nil {
		out.HasMoreColumns = max(c.Page, 1)*c.PageSize < p.columns
	}
	return out
}

// withoutColumns is hd without the columns at the indexes (-1: none).
func withoutColumns(hd canalquery.Header, at [2]int) canalquery.Header {
	return canalquery.Header{Fields: withoutAt(hd.Fields, at), Types: withoutAt(hd.Types, at), ArrowTypes: withoutAt(hd.ArrowTypes, at)}
}

// withoutAt is s without its items at the indexes (-1: none).
func withoutAt[T any](s []T, at [2]int) []T {
	out := make([]T, 0, len(s))
	for i, v := range s {
		if i != at[0] && i != at[1] {
			out = append(out, v)
		}
	}
	return out
}

// takeCounts is r without the count columns at the indexes, and their values
// as numbers: 0 for one that's absent or null.
func takeCounts(r canalquery.Row, at [2]int) (canalquery.Row, [2]int, error) {
	var counts [2]int
	if at[0] < 0 && at[1] < 0 {
		return r, counts, nil
	}
	var values []jsontext.Value
	if err := jsonkit.Unmarshal(r, &values); err != nil {
		return nil, counts, fmt.Errorf("read a pivot's row: %w", err)
	}
	kept := make([]jsontext.Value, 0, len(values))
	for i, v := range values {
		switch i {
		case at[0]:
			counts[0] = countOf(v)
		case at[1]:
			counts[1] = countOf(v)
		default:
			kept = append(kept, v)
		}
	}
	b, err := jsonkit.Marshal(kept)
	if err != nil {
		return nil, counts, err
	}
	return canalquery.Row(b), counts, nil
}

// countOf is a count as a row holds it: a number, or a string of its digits,
// as canal-query writes a big integer.
func countOf(v jsontext.Value) int {
	s := string(v)
	if v.Kind() == '"' {
		_ = jsonkit.Unmarshal(v, &s)
	}
	n, _ := strconv.Atoi(s)
	return n
}

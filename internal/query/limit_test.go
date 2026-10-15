package query

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// The query's own limit is asked for when it is within the maximum; otherwise
// one row past the maximum, which shows a result over it rather than cutting it.
func TestRowCap(t *testing.T) {
	for _, c := range []struct {
		limit, maxRows, truncate int
		capped                   bool
	}{
		{NoLimit, 10, 11, true},
		{5, 10, 5, false},
		{10, 10, 10, false},
		{11, 10, 11, true},
		{NoLimit, 0, NoLimit, false},
		{5, 0, 5, false},
	} {
		truncate, capped := rowCap(c.limit, c.maxRows)
		if truncate != c.truncate || capped != c.capped {
			t.Errorf("rowCap(%d, %d) = %d, %v; want %d, %v", c.limit, c.maxRows, truncate, capped, c.truncate, c.capped)
		}
	}
}

// canalAnswering answers every query with rows rows of one column, as
// canal-query streams a result, and keeps the truncate_rows it was sent.
func canalAnswering(t *testing.T, rows int) (*canalquery.Client, *float64) {
	t.Helper()
	var truncate float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var sent map[string]any
		_ = jsonkit.Unmarshal(body, &sent)
		truncate, _ = sent["truncate_rows"].(float64)
		lines := []string{`{"__holistics_header__":true,"fields":["id"],"types":["int8"],` +
			`"arrow_types":[{"type":"int","bit_width":64,"is_signed":true,"label":"int64"}]}`}
		for range rows {
			lines = append(lines, `["1"]`)
		}
		lines = append(lines, `{"__holistics_trailer__":true,"metadata":{},"http_code":200}`)
		_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
	}))
	t.Cleanup(srv.Close)
	return canalquery.NewClient(srv.URL, false), &truncate
}

var sqlSource = datasource.DataSource{Name: "warehouse", DBType: "postgresql", Connection: map[string]any{"password": "secret"}}

// A result over the maximum fails with ResultTooLarge, naming the maximum,
// rather than coming back cut short.
func TestResultOverTheMaximumFails(t *testing.T) {
	canal, truncate := canalAnswering(t, 3)
	_, err := ExecuteSQL(context.Background(), canal, sqlSource, "select 1", Limits{MaxResultRows: 2})
	if !errors.Is(err, ResultTooLarge) {
		t.Fatalf("err = %v, want result_too_large", err)
	}
	if d, ok := apperr.DetailsOf(err, ResultTooLarge); !ok || d.MaxRows != 2 {
		t.Errorf("details = %+v, %v; want max_rows 2", d, ok)
	}
	if *truncate != 3 {
		t.Errorf("asked canal for %v rows, want one past the maximum: 3", *truncate)
	}
}

// A result at the maximum is answered whole.
func TestResultAtTheMaximumIsAnswered(t *testing.T) {
	canal, _ := canalAnswering(t, 2)
	r, err := ExecuteSQL(context.Background(), canal, sqlSource, "select 1", Limits{MaxResultRows: 2})
	if err != nil || len(r.Records) != 2 {
		t.Fatalf("got %v rows, %v; want 2 rows", r, err)
	}
}

// A query whose own limit is within the maximum is cut at its limit, as it
// asked, and does not fail.
func TestOwnLimitWithinTheMaximum(t *testing.T) {
	canal, truncate := canalAnswering(t, 2)
	compiled := anfranode.CompileToSQLResult{SQL: "select 1", DataSource: anfranode.CompileDataSource{Name: "warehouse", DBType: "postgresql"}}
	r, err := Execute(context.Background(), canal, datasource.Set{"warehouse": sqlSource}, compiled, 2, Limits{MaxResultRows: 5})
	if err != nil || len(r.Records) != 2 {
		t.Fatalf("got %v, %v; want 2 rows", r, err)
	}
	if *truncate != 2 {
		t.Errorf("asked canal for %v rows, want the query's own limit: 2", *truncate)
	}
}

package catalog

import (
	"testing"

	"github.com/holistics/anfra/shared/apperr"
)

// A blank query is the caller's to fix, on the query arg, before any sidecar
// starts; a query with words in it runs.
func TestSearchRefusesABlankQuery(t *testing.T) {
	for _, input := range []string{`{}`, `{"query":[]}`, `{"query":["  ", ""]}`} {
		err := Search.Check([]byte(input))
		v, ok := apperr.DetailsOf(err, apperr.ValidationFailed)
		if !ok || len(v.Violations) != 1 || v.Violations[0].Field != "query" || v.Violations[0].Message != "name what to search for" {
			t.Errorf("%s: %v, want a violation on query", input, err)
		}
		if needs := Search.Needs([]byte(input)); needs.Node || needs.CanalQuery {
			t.Errorf("%s: needs %+v, want no sidecar", input, needs)
		}
	}
	if err := Search.Check([]byte(`{"query":["orders"]}`)); err != nil {
		t.Errorf("a query: %v", err)
	}
}

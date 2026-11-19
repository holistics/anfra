package query

import (
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apptracing"
	"github.com/holistics/anfra/shared/jsonkit"
)

// A query's answer is recorded, with data on, without its rows: their fields
// and count only.
func TestAQueryAnswersPayloadHasNoRows(t *testing.T) {
	r := QueryResult{SQL: "SELECT 1", Result: QueryRows{Fields: []string{"n"}, Records: []canalquery.Row{canalquery.Row(`["secret-value"]`), canalquery.Row(`["other"]`)}}}
	var p apptracing.Payloader = r
	b, err := jsonkit.Marshal(p.TracePayload())
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Contains(got, "secret-value") || strings.Contains(got, "records") {
		t.Errorf("rows recorded: %s", got)
	}
	if !strings.Contains(got, `"result":{"fields":["n"],"count":2}`) || !strings.Contains(got, `"sql":"SELECT 1"`) {
		t.Errorf("got %s", got)
	}
}

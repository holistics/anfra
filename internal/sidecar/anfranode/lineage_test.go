package anfranode

import (
	"testing"

	"github.com/holistics/anfra/shared/jsonkit"
)

// A diagram as anfra-node formats it reads and writes back unchanged, so the
// core API passes it through whole: a link has props only when its edge has
// some.
func TestDiagramRoundTrips(t *testing.T) {
	const diagram = `{"results":[{"target":{"parent":"ecommerce","child":"revenue"},"query":"needed_by:(fqn:ecommerce.revenue)","diagram":{` +
		`"version":2,"query":"needed_by:(fqn:ecommerce.revenue)","total":2,"matched":["aml.dimension:orders.amount","aml.model:orders"],` +
		`"schema":{"edges":{"contains":"A contains B: B is a child of A, listed in A.sections.","refers":"A refers B: A uses B. Data flows from B to A. Stored on A in A.links with type refers."},` +
		`"types":{"aml.dimension":{"top_level":false,"edges":{}},"aml.metric":{"top_level":true,"edges":{"refers":["aml.dimension","aml.model"]}},"aml.model":{"top_level":true,"edges":{"contains":["aml.dimension"]}}}},` +
		`"layers":[{"index":0,"types":["aml.model"],"card_ids":["aml.model:orders"]},{"index":1,"types":["aml.metric"],"card_ids":["aml.metric:revenue"]}],` +
		`"cards":[` +
		`{"id":"aml.model:orders","type":"aml.model","label":"Orders","tags":["Endorsed"],"props":{"table_name":"public.orders"},` +
		`"sections":[{"type":"aml.dimension","nodes":[{"id":"aml.dimension:orders.amount","type":"aml.dimension","label":"Amount"}]}],"layer":0},` +
		`{"id":"aml.metric:revenue","type":"aml.metric","label":"Revenue","links":[{"type":"refers","to":"aml.dimension:orders.amount"},{"type":"refers","to":"aml.model:orders","props":{"via":"join"}}],"layer":1,"detached":true}` +
		`]}},{"target":{"parent":"ecommerce","child":"nosuch"},"query":"needed_by:(fqn:ecommerce.nosuch)","error":"no such field"}]}`

	var res CatalogLineageResult
	if err := jsonkit.Unmarshal([]byte(diagram), &res); err != nil {
		t.Fatal(err)
	}
	links := res.Results[0].Diagram.Cards[1].Links
	if len(links) != 2 || links[0].To != "aml.dimension:orders.amount" || links[0].Props != nil || links[1].Props["via"] != "join" {
		t.Errorf("links = %+v", links)
	}
	b, err := jsonkit.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	// Through a map, so the comparison is of values, not of key order.
	var got, want any
	if err := jsonkit.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if err := jsonkit.Unmarshal([]byte(diagram), &want); err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := jsonkit.Marshal(got)
	wantJSON, _ := jsonkit.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("round trip changed the diagram:\n got %s\nwant %s", gotJSON, wantJSON)
	}
}

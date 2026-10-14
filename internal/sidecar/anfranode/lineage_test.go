package anfranode

import (
	"testing"

	"github.com/holistics/anfra/shared/jsonkit"
)

// A diagram as anfra-node formats it reads and writes back unchanged, so the
// core API passes it through whole: a link with properties stays an object,
// one without stays an id.
func TestDiagramRoundTrips(t *testing.T) {
	const diagram = `{"results":[{"target":{"parent":"ecommerce","child":"revenue"},"query":"needed_by:(fqn:ecommerce.revenue)","diagram":{` +
		`"version":1,"query":"needed_by:(fqn:ecommerce.revenue)","total":2,"matched":["aml.dimension:orders.amount","aml.model:orders"],` +
		`"schema":{"edges":{"contains":"A contains B: B is a child of A, listed in A.sections.","refers":"A refers B: A uses B. Data flows from B to A. Stored on A in A.links.refers."},` +
		`"types":{"aml.dimension":{"contains":[]},"aml.metric":{"refers":["aml.dimension"],"top_level":true},"aml.model":{"contains":["aml.dimension"],"top_level":true}}},` +
		`"layers":[{"index":0,"types":["aml.model"],"card_ids":["aml.model:orders"]},{"index":1,"types":["aml.metric"],"card_ids":["aml.metric:revenue"]}],` +
		`"cards":[` +
		`{"id":"aml.model:orders","type":"aml.model","label":"Orders","tags":["Endorsed"],"props":{"table_name":"public.orders"},` +
		`"sections":[{"type":"aml.dimension","nodes":[{"id":"aml.dimension:orders.amount","type":"aml.dimension","label":"Amount"}]}],"layer":0},` +
		`{"id":"aml.metric:revenue","type":"aml.metric","label":"Revenue","links":{"refers":["aml.dimension:orders.amount",{"id":"aml.model:orders","props":{"via":"join"}}]},"layer":1,"detached":true}` +
		`]}},{"target":{"parent":"ecommerce","child":"nosuch"},"query":"needed_by:(fqn:ecommerce.nosuch)","error":"no such field"}]}`

	var res CatalogLineageResult
	if err := jsonkit.Unmarshal([]byte(diagram), &res); err != nil {
		t.Fatal(err)
	}
	links := res.Results[0].Diagram.Cards[1].Links["refers"]
	if len(links) != 2 || links[0].ID != "aml.dimension:orders.amount" || links[0].Props != nil || links[1].Props["via"] != "join" {
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

// A link keeps the form it came in: an object stays an object even with no
// properties, and what is neither an id nor an object is refused.
func TestDiagramLinkKeepsItsForm(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`"x"`, `"x"`},
		{`{"id":"x","props":{}}`, `{"id":"x","props":{}}`},
		{`{"id":"x"}`, `{"id":"x","props":{}}`},
		{`{"id":"x","props":{"via":"join"}}`, `{"id":"x","props":{"via":"join"}}`},
	} {
		var l DiagramLink
		if err := jsonkit.Unmarshal([]byte(tc.in), &l); err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if b, err := jsonkit.Marshal(l); err != nil || string(b) != tc.want {
			t.Errorf("%s: wrote %s, %v; want %s", tc.in, b, err, tc.want)
		}
	}
	for _, in := range []string{`null`, `1`, `["x"]`} {
		var l DiagramLink
		if err := jsonkit.Unmarshal([]byte(in), &l); err == nil {
			t.Errorf("%s: read as %+v, want an error", in, l)
		}
	}
}

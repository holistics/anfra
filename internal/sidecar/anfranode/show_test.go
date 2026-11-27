package anfranode

import (
	"reflect"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/jsonkit"
)

// A shown object encodes within the answer's encoder, so the answer's encoding
// reaches inside it: a repo with no datasets has [], not null.
func TestShowObjectEncodesAsTheAnswer(t *testing.T) {
	b, err := jsonkit.Marshal(ShowResult{Object: ShowObject{Repo: &ShowRepo{Kind: "repo"}}})
	if want := `{"object":{"kind":"repo","datasets":[]},"diagnostics":[]}`; err != nil || string(b) != want {
		t.Errorf("got %s %v, want %s", b, err, want)
	}
}

// A full dataset carries its relationships as aml.show answers them, inactive
// ones too, and passes them through as they came.
func TestShowDatasetCarriesItsRelationships(t *testing.T) {
	answer := `{"kind":"dataset","fqn":"shop","name":"shop","models":[],"metrics":[],"relationships":[
		{"name":"buyer","from":"orders.user_id","to":"users.id","type":"many_to_one","active":true,"direction":"one_way"},
		{"from":"orders.id","to":"refunds.order_id","type":"one_to_one","active":false,"direction":"two_way"}]}`
	var o ShowObject
	if err := jsonkit.Unmarshal([]byte(answer), &o); err != nil || o.Dataset == nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []ShowRelationship{
		{Name: "buyer", From: "orders.user_id", To: "users.id", Type: "many_to_one", Active: true, Direction: "one_way"},
		{From: "orders.id", To: "refunds.order_id", Type: "one_to_one", Direction: "two_way"},
	}
	if !reflect.DeepEqual(o.Dataset.Relationships, want) {
		t.Errorf("relationships = %+v, want %+v", o.Dataset.Relationships, want)
	}
	b, err := jsonkit.Marshal(o)
	if err != nil || !strings.Contains(string(b), `{"from":"orders.id","to":"refunds.order_id","type":"one_to_one","active":false,"direction":"two_way"}`) {
		t.Errorf("encoded %s %v: want the inactive relationship with active false and no name", b, err)
	}
}

// A dataset carries the @tags of itself and its models as aml.show answers
// them, and passes them through as they came. What has no @tag, and its
// fields and metrics, have no tags.
func TestShowDatasetCarriesItsTags(t *testing.T) {
	answer := `{"kind":"dataset","fqn":"shop","name":"shop",
		"tags":[{"name":"Endorsed","color":"#00ff00","description":"Recommended."}],
		"models":[
			{"fqn":"orders","name":"orders","tags":[{"name":"marketing"}],"fields":[
				{"fqn":"orders.id","name":"id","role":"dimension","type":"number","hidden":false}]},
			{"fqn":"users","name":"users","fields":[]}],
		"metrics":[{"fqn":"shop.revenue","name":"revenue","role":"metric","type":"number","hidden":false}]}`
	var o ShowObject
	if err := jsonkit.Unmarshal([]byte(answer), &o); err != nil || o.Dataset == nil {
		t.Fatalf("unmarshal: %v", err)
	}
	d := o.Dataset
	if want := []ShowTag{{Name: "Endorsed", Color: "#00ff00", Description: "Recommended."}}; !reflect.DeepEqual(d.Tags, want) {
		t.Errorf("dataset tags = %+v, want %+v", d.Tags, want)
	}
	if want := []ShowTag{{Name: "marketing"}}; !reflect.DeepEqual(d.Models[0].Tags, want) {
		t.Errorf("model tags = %+v, want %+v", d.Models[0].Tags, want)
	}
	if d.Models[1].Tags != nil {
		t.Errorf("untagged model tags = %+v, want none", d.Models[1].Tags)
	}
	b, err := jsonkit.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"tags":[{"name":"Endorsed","color":"#00ff00","description":"Recommended."}]`,
		`{"fqn":"orders","name":"orders","tags":[{"name":"marketing"}],"fields":[`,
		`{"fqn":"users","name":"users","fields":[]}`,
		`{"fqn":"orders.id","name":"id","role":"dimension","type":"number","hidden":false}`,
		`{"fqn":"shop.revenue","name":"revenue","role":"metric","type":"number","hidden":false}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("encoded %s: want %s", b, want)
		}
	}
}

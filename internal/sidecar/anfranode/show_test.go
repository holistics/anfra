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

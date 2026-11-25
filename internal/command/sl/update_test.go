package sl

import "testing"

func TestUpdateValidatesBeforeStartingNode(t *testing.T) {
	for _, input := range []string{`{}`, `{"model_name":" "}`, `{"model_name":"\u0000"}`} {
		if err := Update.Check([]byte(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
		if needs := Update.Needs([]byte(input)); needs.Node || needs.CanalQuery {
			t.Errorf("invalid input starts sidecars: %s", input)
		}
	}

	for _, input := range []string{`{"model_name":"sales.orders"}`, `{"model_name":"sales::orders"}`} {
		if err := Update.Check([]byte(input)); err != nil {
			t.Fatal(err)
		}
		if needs := Update.Needs([]byte(input)); !needs.Node || needs.CanalQuery {
			t.Fatalf("needs = %+v", needs)
		}
	}
}

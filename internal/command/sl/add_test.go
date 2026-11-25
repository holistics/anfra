package sl

import (
	"testing"
)

func TestAddValidatesBeforeStartingNode(t *testing.T) {
	for _, input := range []string{`{}`, `{"table":"orders","data_source":"pg"}`, `{"table":"a.b.c","data_source":"pg"}`, `{"table":"public.","data_source":"pg"}`, `{"table":"public.orders"}`, `{"table":"public.orders","data_source":" "}`, `{"table":"\"public\".orders","data_source":"pg"}`} {
		if err := Add.Check([]byte(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
		if needs := Add.Needs([]byte(input)); needs.Node || needs.CanalQuery {
			t.Errorf("invalid input starts sidecars: %s", input)
		}
	}

	input := []byte(`{"table":"Sales.Orders","data_source":"pg"}`)
	if err := Add.Check(input); err != nil {
		t.Fatal(err)
	}
	if needs := Add.Needs(input); !needs.Node || needs.CanalQuery {
		t.Fatalf("needs = %+v", needs)
	}
}

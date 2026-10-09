package show

import (
	"errors"
	"testing"

	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// anfra-node refusing what to show lands on the arg at fault; anything else
// stays as it is.
func TestRefused(t *testing.T) {
	refusal := func(path, msg string) error {
		data, _ := jsonkit.Marshal(map[string]string{"path": path})
		return &anfranode.RPCError{Method: "aml.show", Code: anfranode.RPCInvalidParams, Message: msg, Data: data}
	}
	for _, tc := range []struct {
		name string
		err  error
		want *apperr.Violation
	}{
		{"an fqn found nothing at", refusal("fqn", `No dataset "x" in the repo.`),
			&apperr.Violation{Field: "fqn", Code: "invalid", Message: `No dataset "x" in the repo.`}},
		{"a type not shown", refusal("type", "Showing a aml.model isn't supported yet; only aml.dataset."),
			&apperr.Violation{Field: "type", Code: "invalid", Message: "Showing a aml.model isn't supported yet; only aml.dataset."}},
		{"another path", refusal("dataSources", "bad"), nil},
		{"invalid params with no path", &anfranode.RPCError{Code: anfranode.RPCInvalidParams, Message: "bad"}, nil},
		{"not an RPC error", errors.New("boom"), nil},
		{"no error", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := refused(tc.err)
			switch {
			case tc.want == nil && ok:
				t.Errorf("got %+v, want it left as it is", got)
			case tc.want != nil && (!ok || got != *tc.want):
				t.Errorf("got %+v (%v), want %+v", got, ok, *tc.want)
			}
		})
	}
}

// A ShowObject travels as the object it holds, tagged by kind, both ways; a
// kind this anfra does not know is refused rather than dropped.
func TestShowObjectJSON(t *testing.T) {
	for _, raw := range []string{
		`{"kind":"repo","datasets":[{"kind":"dataset","fqn":"sales","name":"sales"}]}`,
		`{"kind":"dataset","fqn":"sales","name":"sales","models":[{"fqn":"orders","name":"orders","fields":[]}]}`,
	} {
		var o anfranode.ShowObject
		if err := jsonkit.Unmarshal([]byte(raw), &o); err != nil {
			t.Fatal(err)
		}
		back, err := jsonkit.Marshal(o)
		if err != nil || string(back) != raw {
			t.Errorf("round trip: %s, %v; want %s", back, err, raw)
		}
	}
	var o anfranode.ShowObject
	if err := jsonkit.Unmarshal([]byte(`{"kind":"model","fqn":"orders"}`), &o); err == nil {
		t.Error("an unknown kind was accepted")
	}
}

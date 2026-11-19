package telemetry_test

import (
	"slices"
	"testing"

	"github.com/holistics/anfra/shared/telemetry"
)

// A span's stable attributes are those of every stable span its name matches,
// a part in <...> standing for any text.
func TestStableAttributesOfASpan(t *testing.T) {
	names := func(keys []telemetry.Key) []string {
		var n []string
		for _, k := range keys {
			n = append(n, k.String())
		}
		return n
	}
	for name, want := range map[string][]string{
		"anfra query":          {"anfra.repo.id"},
		"anfra telemetry init": {"anfra.repo.id"},
		"anfra.op.core.show":   {"anfra.op", "error.type", "anfra.repo.id"},
		"anfra.op.core.query": {"anfra.op", "error.type", "anfra.repo.id",
			"anfra.query.id", "anfra.query.kind", "anfra.dataset", "anfra.data_source", "db.system.name", "db.query.text"},
		"anfra.op.":           nil,
		"anfra":               nil,
		"HTTP GET":            nil,
		"anfra_node.op.query": nil,
	} {
		if got := names(telemetry.StableAttributes(name)); !slices.Equal(got, want) {
			t.Errorf("StableAttributes(%q) = %v, want %v", name, got, want)
		}
	}
}

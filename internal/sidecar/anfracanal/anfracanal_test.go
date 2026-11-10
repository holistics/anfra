//go:build !embed_sidecar

package anfracanal

import (
	"slices"
	"strings"
	"testing"
)

// In a build from source: a service's own standalone binary runs bare, and wins over anfra-canal, which runs with the
// service's mode.
func TestCommand(t *testing.T) {
	cases := []struct {
		name, serviceBin, canalBin string
		path                       string
		args                       []string
	}{
		{name: "anfra-canal", canalBin: "/b/anfra-canal", path: "/b/anfra-canal", args: []string{"query"}},
		{name: "a standalone canal-query", serviceBin: "/b/canal-query", path: "/b/canal-query"},
		{name: "both", serviceBin: "/b/canal-query", canalBin: "/b/anfra-canal", path: "/b/canal-query"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("ANFRA_CANAL_QUERY_BIN", c.serviceBin)
			t.Setenv(BinEnv, c.canalBin)
			path, args, err := Command("canal-query", "ANFRA_CANAL_QUERY_BIN", "query")
			if err != nil || path != c.path || !slices.Equal(args, c.args) {
				t.Errorf("got %q %q, %v; want %q %q", path, args, err, c.path, c.args)
			}
		})
	}

	t.Run("neither", func(t *testing.T) {
		t.Setenv("ANFRA_CANAL_QUERY_BIN", "")
		t.Setenv(BinEnv, "")
		_, _, err := Command("canal-query", "ANFRA_CANAL_QUERY_BIN", "query")
		want := "this build of anfra doesn't include canal-query. Set ANFRA_CANAL_BIN to the path of an anfra-canal binary, or ANFRA_CANAL_QUERY_BIN to a canal-query one"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("got %v, want %q", err, want)
		}
	})
}

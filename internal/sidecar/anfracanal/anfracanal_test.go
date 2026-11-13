package anfracanal

import (
	"slices"
	"strings"
	"testing"
)

// A build from source runs a service alone when its own variable names one,
// else anfra-canal with the service's mode; and with neither, says both.
func TestCommand(t *testing.T) {
	for _, c := range []struct {
		name          string
		service, both string // the service's variable, and ANFRA_CANAL_BIN
		want          Command
	}{
		{name: "the service alone", service: "/bin/canal-query", want: Command{Path: "/bin/canal-query"}},
		{name: "anfra-canal", both: "/bin/anfra-canal", want: Command{Path: "/bin/anfra-canal", Args: []string{"query"}}},
		{name: "both", service: "/bin/canal-query", both: "/bin/anfra-canal", want: Command{Path: "/bin/canal-query"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(Query.BinEnv, c.service)
			t.Setenv(BinEnv, c.both)
			got, err := Query.Command()
			if err != nil || got.Path != c.want.Path || !slices.Equal(got.Args, c.want.Args) {
				t.Errorf("Command() = %+v, %v; want %+v", got, err, c.want)
			}
		})
	}

	t.Setenv(Query.BinEnv, "")
	t.Setenv(BinEnv, "")
	_, err := Query.Command()
	if err == nil || !strings.Contains(err.Error(), "ANFRA_CANAL_BIN") || !strings.Contains(err.Error(), "ANFRA_CANAL_QUERY_BIN") {
		t.Errorf("neither: %v, want an error naming both variables", err)
	}
}

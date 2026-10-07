package app

import (
	"context"
	"errors"
	"testing"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/errcode"
)

func TestIngestDeclaresRequiredSidecars(t *testing.T) {
	cmd, ok := Find("ingest")
	if !ok {
		t.Fatal("ingest command was not registered")
	}
	needs := cmd.Needs(nil)
	if !needs.Node || !needs.CanalQuery {
		t.Fatalf("ingest sidecars = %+v, want both anfra-node and canal-query", needs)
	}
}

func TestSearchDeclaresRequiredSidecars(t *testing.T) {
	cmd, ok := Find("search")
	if !ok {
		t.Fatal("search command was not registered")
	}
	needs := cmd.Needs(nil)
	if !needs.Node || !needs.CanalQuery {
		t.Fatalf("search sidecars = %+v, want both anfra-node and canal-query", needs)
	}
}

// A command that needs a sidecar refuses to run without it, as
// sidecar_unavailable, rather than calling a client that is not there.
func TestCommandsRefuseWithoutTheirSidecars(t *testing.T) {
	cc := command.CommandContext{DataPerms: dataperm.Unrestricted()}
	for cmd, input := range map[string]string{
		"ingest":   `{}`,
		"search":   `{"query":["revenue"]}`,
		"validate": `{}`,
		"query":    `{"query":"q","dataset":"d"}`,
		"show":     `{"fqn":"ecommerce"}`,
	} {
		if _, err := Invoke(context.Background(), cc, cmd, []byte(input)); !errors.Is(err, errcode.SidecarUnavailable) {
			t.Errorf("%s: got %v, want sidecar_unavailable", cmd, err)
		}
	}
}

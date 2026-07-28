package app

import (
	"testing"
)

func TestIngestDeclaresRequiredSidecars(t *testing.T) {
	cmd, ok := Find("ingest")
	if !ok {
		t.Fatal("ingest command was not registered")
	}
	needs := cmd.Needs(map[string]any{})
	if !needs.Node || !needs.CanalQuery {
		t.Fatalf("ingest sidecars = %+v, want both anfra-node and canal-query", needs)
	}
}

func TestSearchDeclaresRequiredSidecars(t *testing.T) {
	cmd, ok := Find("search")
	if !ok {
		t.Fatal("search command was not registered")
	}
	needs := cmd.Needs(map[string]any{})
	if !needs.Node || !needs.CanalQuery {
		t.Fatalf("search sidecars = %+v, want both anfra-node and canal-query", needs)
	}
}

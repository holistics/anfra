package app

import (
	"testing"
)

func TestLineageNeedsOnlyAnfraNode(t *testing.T) {
	cmd, ok := Find("lineage")
	if !ok {
		t.Fatal("lineage command was not registered")
	}
	needs := cmd.Needs(map[string]any{})
	if !needs.Node || needs.CanalQuery {
		t.Fatalf("lineage sidecars = %+v, want anfra-node only", needs)
	}
}

func TestLineageReadsTargetsAndOptions(t *testing.T) {
	targets, opts, err := lineageRun(map[string]any{
		"targets": `[{"model_name":"o","measure_name":"s"}]`,
		"limit":   "5",
		"offset":  float64(0),
	})
	if err != nil {
		t.Fatalf("lineageRun returned error: %v", err)
	}
	if len(targets) != 1 || targets[0].ModelName != "o" || targets[0].MeasureName != "s" {
		t.Fatalf("targets = %+v", targets)
	}
	if *opts.Limit != 5 || *opts.Offset != 0 {
		t.Fatalf("opts = %+v", opts)
	}

	targets, _, err = lineageRun(map[string]any{"dataset": "d", "metric": "m"})
	if err != nil || len(targets) != 1 || targets[0].DatasetName != "d" || targets[0].MetricName != "m" {
		t.Fatalf("flag target = %+v, %v", targets, err)
	}

	if _, _, err := lineageRun(map[string]any{"limit": "-1"}); err == nil {
		t.Fatal("lineageRun accepted a negative limit")
	}
}

func TestLineageRejectsTargetsWithFlagTarget(t *testing.T) {
	cmd, _ := Find("lineage")
	err := checkExclusiveArgs(cmd, map[string]any{"targets": "[]", "dataset": "d"})
	if err == nil {
		t.Fatal("lineage accepted --targets with --dataset")
	}
}

// Package lineage is the catalog lineage use-case layer: it builds the request
// anfra-node needs to run the lineage of AML fields.
package lineage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
)

// Options page the lineage's matched entities.
type Options struct {
	Limit  *int
	Offset *int
}

// Run asks anfra-node for the lineage diagram of each target. Go owns
// orchestration (sidecar, repo identity, sources file); anfra-node builds the
// catalog from the repo on each call, so no prior ingest is needed.
func Run(ctx context.Context, node *sidecar.AnfraNodeClient, r repo.Repo, targets []sidecar.LineageTarget, opts Options) (sidecar.CatalogLineageResult, error) {
	if len(targets) == 0 {
		return sidecar.CatalogLineageResult{}, fmt.Errorf("lineage needs a target: --dataset with --metric or --dimension, --model with --measure or --dimension, or --targets")
	}
	for _, t := range targets {
		if err := ValidateTarget(t); err != nil {
			return sidecar.CatalogLineageResult{}, err
		}
	}
	if node == nil {
		return sidecar.CatalogLineageResult{}, fmt.Errorf("lineage requires the anfra-node sidecar")
	}

	req := sidecar.CatalogLineageRequest{
		RepoPath: r.Dir,
		RepoID:   r.ID,
		Targets:  targets,
		Limit:    opts.Limit,
		Offset:   opts.Offset,
	}
	// The sources file (dbt, postgres, ...) is optional: without it the lineage
	// covers the AML repo only. anfra-node rejects a sourcesPath that doesn't exist.
	if p := filepath.Join(r.ConfigDir, "context_sources.yml"); fileExists(p) {
		req.SourcesPath = p
	}
	return node.LineageCatalog(ctx, req)
}

// ValidateTarget checks a target names one dataset metric or dimension, or one
// model measure or dimension.
func ValidateTarget(t sidecar.LineageTarget) error {
	fields := 0
	for _, f := range []string{t.MetricName, t.MeasureName, t.DimensionName} {
		if f != "" {
			fields++
		}
	}
	switch {
	case t.DatasetName != "" && t.ModelName != "":
		return fmt.Errorf("lineage target %s: set a dataset or a model, not both", describe(t))
	case t.DatasetName == "" && t.ModelName == "":
		return fmt.Errorf("lineage target %s: needs a dataset or a model", describe(t))
	case fields != 1:
		return fmt.Errorf("lineage target %s: needs exactly one metric, measure or dimension", describe(t))
	case t.DatasetName != "" && t.MeasureName != "":
		return fmt.Errorf("lineage target %s: a dataset has metrics, not measures", describe(t))
	case t.ModelName != "" && t.MetricName != "":
		return fmt.Errorf("lineage target %s: a model has measures, not metrics", describe(t))
	}
	return nil
}

func describe(t sidecar.LineageTarget) string {
	b, _ := json.Marshal(t)
	return string(b)
}

// ParseTargets reads a list of targets from a JSON array, given as a string (the
// CLI) or as a decoded array (/call). Absent or empty is nil.
func ParseTargets(v any) ([]sidecar.LineageTarget, error) {
	var raw []byte
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		if strings.TrimSpace(t) == "" {
			return nil, nil
		}
		raw = []byte(t)
	case []any:
		b, err := json.Marshal(t)
		if err != nil {
			return nil, fmt.Errorf("invalid targets: %w", err)
		}
		raw = b
	default:
		return nil, fmt.Errorf("invalid targets: expected a JSON array of targets")
	}
	var targets []sidecar.LineageTarget
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&targets); err != nil {
		return nil, fmt.Errorf("invalid targets: expected a JSON array of {dataset_name, metric_name | dimension_name} or {model_name, measure_name | dimension_name}: %w", err)
	}
	return targets, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

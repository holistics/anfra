package lineage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
)

// fakeNode answers catalog.lineage and hands back the raw params it got.
func fakeNode(t *testing.T, params *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode rpc request: %v", err)
		}
		if req.Method != "catalog.lineage" {
			t.Fatalf("method = %q, want catalog.lineage", req.Method)
		}
		*params = req.Params
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{"results": []any{
				map[string]any{
					"target":  map[string]any{"dataset_name": "d", "metric_name": "m"},
					"query":   "needed_by:(type:metric fqn:d.m)",
					"diagram": map[string]any{"version": 5, "total": 1},
				},
			}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testRepo(t *testing.T) repo.Repo {
	dir := t.TempDir()
	return repo.Repo{Dir: dir, ID: "repo-123", ConfigDir: filepath.Join(dir, ".anfra")}
}

func TestRunPassesTargetsAndOptionsToRPC(t *testing.T) {
	var params map[string]any
	node := fakeNode(t, &params)
	r := testRepo(t)
	limit, offset := 10, 0

	res, err := Run(context.Background(), sidecar.NewAnfraNodeClientHTTP(node.URL), r,
		[]sidecar.LineageTarget{{DatasetName: "d", MetricName: "m"}},
		Options{Limit: &limit, Offset: &offset})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	want := map[string]any{
		"repoPath": r.Dir,
		"repoId":   "repo-123",
		"targets":  []any{map[string]any{"dataset_name": "d", "metric_name": "m"}},
		"limit":    float64(10),
		"offset":   float64(0),
	}
	if !reflect.DeepEqual(params, want) {
		t.Fatalf("params = %v, want %v", params, want)
	}
	if len(res.Results) != 1 || res.Results[0].Query != "needed_by:(type:metric fqn:d.m)" || len(res.Results[0].Diagram) == 0 {
		t.Fatalf("results = %+v", res.Results)
	}
}

func TestRunOmitsUnsetOptions(t *testing.T) {
	var params map[string]any
	node := fakeNode(t, &params)

	if _, err := Run(context.Background(), sidecar.NewAnfraNodeClientHTTP(node.URL), testRepo(t),
		[]sidecar.LineageTarget{{ModelName: "o", DimensionName: "x"}}, Options{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for _, key := range []string{"limit", "offset", "sourcesPath"} {
		if _, ok := params[key]; ok {
			t.Fatalf("params has %q, want it omitted: %v", key, params)
		}
	}
}

func TestRunPassesSourcesFileWhenPresent(t *testing.T) {
	var params map[string]any
	node := fakeNode(t, &params)
	r := testRepo(t)
	sources := filepath.Join(r.ConfigDir, "context_sources.yml")
	if err := os.MkdirAll(r.ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sources, []byte("sources: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(context.Background(), sidecar.NewAnfraNodeClientHTTP(node.URL), r,
		[]sidecar.LineageTarget{{DatasetName: "d", DimensionName: "x"}}, Options{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if params["sourcesPath"] != sources {
		t.Fatalf("sourcesPath = %v, want %s", params["sourcesPath"], sources)
	}
}

func TestRunRequiresATarget(t *testing.T) {
	_, err := Run(context.Background(), nil, repo.Repo{}, nil, Options{})
	if err == nil || !strings.Contains(err.Error(), "lineage needs a target") {
		t.Fatalf("error = %v, want missing target error", err)
	}
}

func TestValidateTarget(t *testing.T) {
	cases := []struct {
		target  sidecar.LineageTarget
		wantErr string
	}{
		{sidecar.LineageTarget{DatasetName: "d", MetricName: "m"}, ""},
		{sidecar.LineageTarget{DatasetName: "d", DimensionName: "x"}, ""},
		{sidecar.LineageTarget{ModelName: "o", MeasureName: "s"}, ""},
		{sidecar.LineageTarget{ModelName: "o", DimensionName: "x"}, ""},
		{sidecar.LineageTarget{MetricName: "m"}, "needs a dataset or a model"},
		{sidecar.LineageTarget{DatasetName: "d", ModelName: "o", MetricName: "m"}, "not both"},
		{sidecar.LineageTarget{DatasetName: "d"}, "exactly one"},
		{sidecar.LineageTarget{DatasetName: "d", MetricName: "m", DimensionName: "x"}, "exactly one"},
		{sidecar.LineageTarget{DatasetName: "d", MeasureName: "s"}, "not measures"},
		{sidecar.LineageTarget{ModelName: "o", MetricName: "m"}, "not metrics"},
	}
	for _, c := range cases {
		err := ValidateTarget(c.target)
		if c.wantErr == "" && err != nil {
			t.Errorf("ValidateTarget(%+v) = %v, want nil", c.target, err)
		}
		if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("ValidateTarget(%+v) = %v, want %q", c.target, err, c.wantErr)
		}
	}
}

func TestParseTargets(t *testing.T) {
	want := []sidecar.LineageTarget{{DatasetName: "d", MetricName: "m"}, {ModelName: "o", DimensionName: "x"}}
	fromCLI, err := ParseTargets(`[{"dataset_name":"d","metric_name":"m"},{"model_name":"o","dimension_name":"x"}]`)
	if err != nil || !reflect.DeepEqual(fromCLI, want) {
		t.Fatalf("ParseTargets(string) = %+v, %v", fromCLI, err)
	}
	fromCall, err := ParseTargets([]any{
		map[string]any{"dataset_name": "d", "metric_name": "m"},
		map[string]any{"model_name": "o", "dimension_name": "x"},
	})
	if err != nil || !reflect.DeepEqual(fromCall, want) {
		t.Fatalf("ParseTargets([]any) = %+v, %v", fromCall, err)
	}
	if got, err := ParseTargets(" "); got != nil || err != nil {
		t.Fatalf("ParseTargets(blank) = %+v, %v; want nil, nil", got, err)
	}
	if _, err := ParseTargets(`[{"dataset":"d"}]`); err == nil {
		t.Fatal("ParseTargets accepted an unknown key")
	}
	if _, err := ParseTargets(`{"dataset_name":"d"}`); err == nil {
		t.Fatal("ParseTargets accepted an object, want an array")
	}
}

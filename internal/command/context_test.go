package command

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/shared/telemetry"
)

// The repo's data sources: the host's when it supplies them; a local repo's
// data_sources.yml otherwise; and for a managed repo without the host's, a
// refusal naming where they must come from, never an empty directory read.
func TestLoadDataSources(t *testing.T) {
	dir := t.TempDir()
	local := repo.Resolve(dir)
	if err := os.MkdirAll(local.ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local.ConfigDir, datasource.FileName),
		[]byte("data_sources:\n  file:\n    type: postgresql\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rev, err := repo.Managed("0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b",
		"0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b@0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	host := datasource.Set{"host": {Name: "host", DBType: "bigquery"}}

	for name, tc := range map[string]struct {
		cc   CommandContext
		want string
	}{
		"local, from its file":   {CommandContext{Repo: local.Repo()}, "file"},
		"local, the host's":      {CommandContext{Repo: local.Repo(), DataSources: host}, "host"},
		"managed, the host's":    {CommandContext{Repo: rev, DataSources: host}, "host"},
		"managed, none supplied": {CommandContext{Repo: rev}, ""},
		"no repo, none supplied": {CommandContext{}, ""},
	} {
		got, err := tc.cc.ResolveDataSources()
		if tc.want == "" {
			if err == nil || !strings.Contains(err.Error(), "Invocation.DataSources") {
				t.Errorf("%s: %v, %v; want a refusal naming Invocation.DataSources", name, got, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		dialects, err := got.Dialects(context.Background())
		if err != nil || len(dialects) != 1 || dialects[0].Name != tc.want {
			t.Errorf("%s: dialects %v, %v; want %s", name, dialects, err, tc.want)
		}
	}
}

// status's data has one field per category of the registry, in its order, and
// each level's enum is every level a category takes.
func TestTelemetryDataMatchesTheRegistry(t *testing.T) {
	typ := reflect.TypeFor[TelemetryData]()
	if typ.NumField() != len(telemetry.Categories) {
		t.Fatalf("TelemetryData has %d fields, the registry %d categories", typ.NumField(), len(telemetry.Categories))
	}
	levels := map[string]bool{}
	for i, d := range telemetry.Categories {
		if tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]; tag != string(d.Name) {
			t.Errorf("field %d is %q, want %q", i, tag, d.Name)
		}
		for _, l := range d.Levels {
			levels[string(l)] = true
		}
	}
	enum, _ := reflect.TypeFor[TelemetryLevel]().FieldByName("Value")
	for _, l := range strings.Split(enum.Tag.Get("enum"), ",") {
		if !levels[l] {
			t.Errorf("enum has %q, which no category takes", l)
		}
		delete(levels, l)
	}
	if len(levels) > 0 {
		t.Errorf("enum lacks %v", levels)
	}
}

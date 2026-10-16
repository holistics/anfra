package datasource

import (
	"os"
	"path/filepath"
	"testing"
)

// A data source's settings are read as written, beside its connection; one
// without settings has none.
func TestLoadSettings(t *testing.T) {
	dir := t.TempDir()
	yml := `data_sources:
  warehouse:
    type: postgresql
    connection:
      host: localhost
    settings:
      statement_timeout_ms: 60000
      require_ssl: true
  archive:
    type: bigquery
    connection:
      project_id: p
`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	set, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s := set["warehouse"].Settings; s["statement_timeout_ms"] != 60000 || s["require_ssl"] != true || len(s) != 2 {
		t.Errorf("warehouse settings = %v", s)
	}
	if s := set["archive"].Settings; s != nil {
		t.Errorf("archive settings = %v, want none", s)
	}
}

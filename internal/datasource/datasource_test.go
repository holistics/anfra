package datasource

import (
	"os"
	"path/filepath"
	"strings"
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

// Without data_sources.yml, Load says how to make it.
func TestLoadMissing(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil || !strings.Contains(err.Error(), "Run `anfra init --data-sources`") {
		t.Errorf("err = %v", err)
	}
}

// A loaded data source's identity is its connection hashed: another user,
// password, host or project is another identity, while settings and the order
// the connection is written in are not. The credentials are only in the hash.
func TestLoadIdentity(t *testing.T) {
	load := func(connection, settings string) string {
		t.Helper()
		dir := t.TempDir()
		yml := "data_sources:\n  w:\n    type: postgresql\n    connection:\n" + connection + settings
		if err := os.WriteFile(filepath.Join(dir, FileName), []byte(yml), 0o600); err != nil {
			t.Fatal(err)
		}
		set, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		return set["w"].Identity
	}
	const conn = "      host: db\n      user: anfra\n      password: secret\n      dbname: shop\n"
	base := load(conn, "")
	if !strings.HasPrefix(base, "conn1:") || strings.Contains(base, "secret") {
		t.Fatalf("identity %q", base)
	}
	if got := load("      dbname: shop\n      password: secret\n      user: anfra\n      host: db\n", ""); got != base {
		t.Error("the connection written in another order is another identity")
	}
	if got := load(conn, "    settings:\n      statement_timeout_ms: 1000\n"); got != base {
		t.Error("settings changed the identity")
	}
	for name, other := range map[string]string{
		"another password": strings.Replace(conn, "secret", "rotated", 1),
		"another user":     strings.Replace(conn, "user: anfra", "user: admin", 1),
		"another host":     strings.Replace(conn, "host: db", "host: replica", 1),
		"another database": strings.Replace(conn, "dbname: shop", "dbname: hr", 1),
	} {
		if load(other, "") == base {
			t.Errorf("%s: the same identity", name)
		}
	}
}

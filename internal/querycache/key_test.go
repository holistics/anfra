package querycache

import (
	"errors"
	"strings"
	"testing"
)

// The access test: whatever decides access is in the key, so changing any one
// of them gives another key. A new access input must join this list before it
// ships (docs/designs/query-cache.md#keys).
func TestKeyHoldsAccess(t *testing.T) {
	base := KeyInput{CompilerKey: "v1:abc", Tenant: "acme", DataSource: "conn1:111", DBType: "postgresql", TruncateRows: 10000}
	k, err := NewKey(base)
	if err != nil || !strings.HasPrefix(string(k), keyFormat) {
		t.Fatalf("NewKey(base) = %q, %v", k, err)
	}
	if again, _ := NewKey(base); again != k {
		t.Errorf("the same input gave %q, then %q", k, again)
	}

	for name, change := range map[string]func(*KeyInput){
		// Access, guaranteed.
		"another compiled query":       func(in *KeyInput) { in.CompilerKey = "v1:abd" },
		"another tenant":               func(in *KeyInput) { in.Tenant = "globex" },
		"another data source identity": func(in *KeyInput) { in.DataSource = "conn1:112" },
		"a SQL text in place of the compiler key": func(in *KeyInput) {
			in.SQL, in.CompilerKey = in.CompilerKey, ""
		},
		// The rows, best effort.
		"another dbtype":         func(in *KeyInput) { in.DBType = "mysql" },
		"another row truncation": func(in *KeyInput) { in.TruncateRows = 5000 },
	} {
		in := base
		change(&in)
		other, err := NewKey(in)
		if err != nil || other == k {
			t.Errorf("%s: the key %q (%v), want another than %q", name, other, err, k)
		}
	}
}

// An input missing what decides access makes no key, and says what's missing.
func TestKeyRefusesIncompleteInput(t *testing.T) {
	ok := KeyInput{CompilerKey: "v1:abc", Tenant: "acme", DataSource: "conn1:111", DBType: "postgresql"}
	for want, change := range map[error]func(*KeyInput){
		ErrNoQuery:              func(in *KeyInput) { in.CompilerKey = "" },
		ErrTwoQueries:           func(in *KeyInput) { in.SQL = "select 1" },
		ErrNoTenant:             func(in *KeyInput) { in.Tenant = "" },
		ErrNoDataSourceIdentity: func(in *KeyInput) { in.DataSource = "" },
	} {
		in := ok
		change(&in)
		if k, err := NewKey(in); !errors.Is(err, want) || k != "" {
			t.Errorf("NewKey = %q, %v; want %v", k, err, want)
		}
	}
}

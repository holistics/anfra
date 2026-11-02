package validate

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
)

func TestAppReports(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"apps/sales.html":         `<script src="chart.js"></script>`,
		"apps/team/overview.html": `<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/a@1/a.css">`,
		"apps/clean.html":         `<title>Clean</title>`,
		"apps/logo.png":           "png",
		"apps/.hidden/x.html":     `<img src="x.png">`,
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	files := func(globs ...string) map[string]int {
		got := map[string]int{}
		for _, r := range appReports(dir, dataApps(dir), globs) {
			if r.Severity != "warning" || r.Validator != "data_app" {
				t.Errorf("report %+v: want a data_app warning", r)
			}
			got[r.FilePath]++
		}
		return got
	}
	if got := files(); len(got) != 2 || got["apps/sales.html"] != 1 || got["apps/team/overview.html"] != 1 {
		t.Errorf("every app: %v", got)
	}
	if got := files("apps/team"); len(got) != 1 || got["apps/team/overview.html"] != 1 {
		t.Errorf("a folder: %v", got)
	}
	if got := files("apps/*.html"); len(got) != 1 || got["apps/sales.html"] != 1 {
		t.Errorf("a glob: %v", got)
	}
	if got := files("models/*.aml"); len(got) != 0 {
		t.Errorf("no app selected: %v", got)
	}
}

func TestUnmatched(t *testing.T) {
	apps := []string{"apps/sales.html", "apps/team/overview.html"}
	query := "explore { measures { s: sessions } limit:8 }"

	if err := unmatched([]string{"models", "apps/team"}, []string{"apps/team"}, apps, true); err != nil {
		t.Errorf("a glob selecting only a Data App: %v", err)
	}
	if err := unmatched(nil, nil, apps, true); err != nil {
		t.Errorf("no globs: %v", err)
	}

	err := unmatched([]string{"models", query, "nope/*.aml"}, []string{query, "nope/*.aml"}, apps, true)
	if !errors.Is(err, apperr.ValidationFailed) {
		t.Fatalf("got %v, want validation_failed", err)
	}
	vs, _ := apperr.DetailsOf(err, apperr.ValidationFailed)
	want := []apperr.Violation{
		{Field: "globs[1]", Code: "invalid", Message: `"` + query + `" matches no AML file or Data App in the repo; to check a query, use core.query.validate (anfra query validate)`},
		{Field: "globs[2]", Code: "invalid", Message: `"nope/*.aml" matches no AML file or Data App in the repo`},
	}
	if len(vs.Violations) != len(want) {
		t.Fatalf("violations %+v, want %+v", vs.Violations, want)
	}
	for i := range want {
		if vs.Violations[i] != want[i] {
			t.Errorf("violation %d: %+v, want %+v", i, vs.Violations[i], want[i])
		}
	}

	// A managed repo's Data Apps are not checked here: an app's path selects nothing.
	err = unmatched([]string{"apps/sales.html"}, []string{"apps/sales.html"}, nil, false)
	vs, _ = apperr.DetailsOf(err, apperr.ValidationFailed)
	if len(vs.Violations) != 1 || vs.Violations[0].Message != `"apps/sales.html" matches no AML file in the repo` {
		t.Errorf("managed repo: %v", err)
	}
}

func TestRefused(t *testing.T) {
	err := fmt.Errorf("validate AML: %w", &anfranode.RPCError{
		Method: "aml.validate", Code: anfranode.RPCInvalidParams, Message: `path selector "../x" resolves outside the repo`,
		Data: jsontext.Value(`{"code":"invalid_input","path":"paths[1]"}`),
	})
	v, ok := refused(err)
	if want := (apperr.Violation{Field: "globs[1]", Code: "invalid", Message: `path selector "../x" resolves outside the repo`}); !ok || v != want {
		t.Errorf("refused: %+v %v, want %+v", v, ok, want)
	}
	if _, ok := refused(&anfranode.RPCError{Method: "aml.validate", Code: -32603, Message: "boom"}); ok {
		t.Error("anfra-node's own failure taken as a refused glob")
	}
}

func TestLooksLikeQuery(t *testing.T) {
	for g, want := range map[string]bool{
		"explore { measures { s: sessions } limit:8 }": true,
		"sum(orders.quantity)":                         true,
		"orders | select(orders.id)":                   true,
		"count_distinct(users.id)\n":                   true,
		"models/*.aml":                                 false,
		"models/{orders,users}.model.aml":              false,
		"models/@(orders|users).model.aml":             false,
		"models/!(old).aml":                            false,
		"models/orders.model.aml":                      false,
	} {
		if got := looksLikeQuery(g); got != want {
			t.Errorf("looksLikeQuery(%q) = %v, want %v", g, got, want)
		}
	}
}

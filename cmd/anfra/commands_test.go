package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/holistics/anfra/internal/app"
)

func command(t *testing.T, name string) app.Command {
	t.Helper()
	c, ok := app.Find(name)
	if !ok {
		t.Fatalf("no command %s", name)
	}
	return c
}

// An answer carries its verdict: an invalid one is printed, then exits 1
// silently, since the answer says why.
func TestPresentExitsOneOnAnInvalidVerdict(t *testing.T) {
	var out bytes.Buffer
	err := presentTo(command(t, "query.validate"), []byte(`{"valid":false,"diagnostics":[{"severity":"error","message":"no such field"}]}`), &out)
	var ec *exitCodeError
	if !errors.As(err, &ec) || ec.code != 1 {
		t.Errorf("an invalid verdict: got %v, want exit code 1", err)
	}
	if out.Len() == 0 {
		t.Error("the invalid answer was not printed")
	}
	if err := presentTo(command(t, "query.validate"), []byte(`{"valid":true,"diagnostics":[]}`), &out); err != nil {
		t.Errorf("a valid verdict: got %v", err)
	}
}

func TestPresentSearchRendersCompactResults(t *testing.T) {
	body := []byte(`{
		"results": [
			{"display_name": "Orders", "source": "local_aml_repo", "type": "aml.model"},
			{"display_name": "Revenue", "source": "local_aml_repo", "type": "aml.metric"}
		],
		"meta": {"total": 2},
		"sql": "SELECT 1"
	}`)
	var out bytes.Buffer

	if err := presentTo(command(t, "search"), body, &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	want := "local_aml_repo | aml.model | Orders\nlocal_aml_repo | aml.metric | Revenue\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestPresentSearchRendersEmptyOutputForNoResults(t *testing.T) {
	body := []byte(`{"results":[],"meta":{"total":0}}`)
	var out bytes.Buffer

	if err := presentTo(command(t, "search"), body, &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	if out.String() != "" {
		t.Fatalf("output = %q, want empty", out.String())
	}
}

func TestPresentSearchRendersEmptyDisplayName(t *testing.T) {
	body := []byte(`{
		"results": [
			{"display_name": null, "source": "warehouse", "type": "database.table"},
			{"source": "warehouse", "type": "database.column"}
		]
	}`)
	var out bytes.Buffer

	if err := presentTo(command(t, "search"), body, &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	want := "warehouse | database.table | \nwarehouse | database.column | \n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestPresentNonSearchUsesYAML(t *testing.T) {
	body := []byte(`{"version":"1.2.3"}`)
	var out bytes.Buffer

	if err := presentTo(command(t, "version"), body, &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	want := "version: 1.2.3\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

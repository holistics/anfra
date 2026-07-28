package main

import (
	"bytes"
	"testing"
)

func TestPresentSearchRendersCompactResults(t *testing.T) {
	body := []byte(`{
		"status": "ok",
		"data": {
			"results": [
				{"display_name": "Orders", "source": "local_aml_repo", "type": "aml.model"},
				{"display_name": "Revenue", "source": "local_aml_repo", "type": "aml.metric"}
			],
			"meta": {"total": 2},
			"sql": "SELECT 1"
		}
	}`)
	var out bytes.Buffer

	if err := presentTo("search", body, "application/json", &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	want := "local_aml_repo | aml.model | Orders\nlocal_aml_repo | aml.metric | Revenue\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestPresentSearchRendersEmptyOutputForNoResults(t *testing.T) {
	body := []byte(`{"status":"ok","data":{"results":[],"meta":{"total":0}}}`)
	var out bytes.Buffer

	if err := presentTo("search", body, "application/json", &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	if out.String() != "" {
		t.Fatalf("output = %q, want empty", out.String())
	}
}

func TestPresentSearchRendersEmptyDisplayName(t *testing.T) {
	body := []byte(`{
		"status": "ok",
		"data": {
			"results": [
				{"display_name": null, "source": "warehouse", "type": "database.table"},
				{"source": "warehouse", "type": "database.column"}
			]
		}
	}`)
	var out bytes.Buffer

	if err := presentTo("search", body, "application/json", &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	want := "warehouse | database.table | \nwarehouse | database.column | \n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestPresentNonSearchUsesYAML(t *testing.T) {
	body := []byte(`{"status":"ok","data":{"message":"ingest complete"}}`)
	var out bytes.Buffer

	if err := presentTo("ingest", body, "application/json", &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	want := "message: ingest complete\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

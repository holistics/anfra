package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/command/sl"
	"github.com/holistics/anfra/shared/jsonkit"
)

func TestSLOutputAndSchema(t *testing.T) {
	t.Cleanup(func() { resetOutput(); output.pretty = false })
	t.Chdir(t.TempDir())

	changed := false
	result := sl.Result{Message: "Model is up to date.", ModelName: "orders", Path: "models/orders.model.aml", Content: "Model orders {}", WarningList: []string{"Check the type."}, Changed: &changed}
	body, err := jsonkit.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"sl.add", "sl.update"} {
		c := registered(t, name)
		var stdout, stderr bytes.Buffer
		if err := deliver(t.Context(), c, body, delivery{stdout: &stdout, stderr: &stderr}); err != nil {
			t.Fatal(err)
		}

		got := stdout.String()
		if !strings.Contains(got, result.Message) || !strings.Contains(got, result.Content) || !strings.Contains(got, "Check the type.") || stderr.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", got, stderr.String())
		}

		output.json = true
		stdout.Reset()
		stderr.Reset()
		if err := deliver(t.Context(), c, body, delivery{stdout: &stdout, stderr: &stderr}); err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(stdout.String(), `"content":"Model orders {}"`) || stderr.Len() != 0 {
			t.Fatalf("json = %s, stderr = %s", &stdout, &stderr)
		}

		resetOutput()
		cmd := newRootCmd()
		cmd.SetArgs([]string{"sl", strings.TrimPrefix(name, "sl."), "--schema", "--json"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}

		resetOutput()
	}

	entries, _ := os.ReadDir(".")
	if len(entries) != 0 {
		t.Fatalf("schema or presentation wrote files: %v", entries)
	}
}

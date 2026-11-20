package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command/sl"
	"github.com/holistics/anfra/shared/jsonkit"
)

func TestSLFilesAddUpdateAndNoop(t *testing.T) {
	dir := t.TempDir()
	result := sl.Result{ModelName: "public_orders", Path: "models/public_orders.model.aml", Content: "Model public_orders {}\n"}
	if err := writeSLFile(dir, "sl.add", result); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, result.Path)
	before, _ := os.ReadFile(path)
	if err := writeSLFile(dir, "sl.add", result); err == nil {
		t.Fatal("overwrote an existing model")
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("failed add changed the file")
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	changed := true
	result.Changed = &changed
	result.Content = "// updated\n" + result.Content
	if err := writeSLFile(dir, "sl.update", result); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	after, _ := os.ReadFile(path)
	if info.Mode().Perm() != 0o640 || string(after) != result.Content {
		t.Fatalf("mode = %v, content = %s", info.Mode(), after)
	}
	past := time.Unix(1000, 0)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	changed = false
	if err := writeSLFile(dir, "sl.update", result); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(path)
	if !info.ModTime().Equal(past) {
		t.Fatal("no-op update wrote the file")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

func TestSLFilesRefuseUnsafePathsAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.aml"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.aml"), filepath.Join(dir, "file.aml")); err != nil {
		t.Fatal(err)
	}
	changed := true
	for _, name := range []string{"sl.add", "sl.update"} {
		for _, path := range []string{"../escape.aml", filepath.Join(outside, "absolute.aml"), "linked/outside.aml", "file.aml"} {
			result := sl.Result{Path: path, Content: "replacement", Changed: &changed}
			if err := writeSLFile(dir, name, result); err == nil {
				t.Errorf("%s accepted %s", name, path)
			}
		}
	}
	content, _ := os.ReadFile(filepath.Join(outside, "outside.aml"))
	if string(content) != "original" {
		t.Fatal("modified a file outside the repo")
	}
}

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
		if !strings.Contains(stdout.String(), result.Message) || strings.Contains(stdout.String(), result.Content) || !strings.Contains(stderr.String(), "Check the type.") {
			t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
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

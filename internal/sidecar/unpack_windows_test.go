//go:build windows

package sidecar

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Each version unpacks to a folder of its own. Unpacking a new one leaves an
// older folder whose binary still runs, and removes it once it has exited.
func TestExtractRuntimeKeepsRunningVersion(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	exe, err := os.ReadFile(os.Args[0]) //nolint:gosec // G703: this test's binary, one that runs
	if err != nil {
		t.Fatal(err)
	}
	v1, err := extractRuntime("anfra-test", exe, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(v1) != "anfra-test.exe" {
		t.Errorf("unpacked to %s, want a file named anfra-test.exe", v1)
	}

	p, err := StartProcess(context.Background(), ProcSpec{
		Name: "helper-stdin-eof", Path: v1, Args: []string{"-test.run=^$"},
		ExtraEnv: []string{helperEnv + "=stdin-eof"}, PipeStdin: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	v2data := append(slices.Clone(exe), 0) // another version: Windows ignores bytes past the image
	v2, err := extractRuntime("anfra-test", v2data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(v1) == filepath.Dir(v2) {
		t.Fatalf("both versions in %s", filepath.Dir(v1))
	}
	if !exists(v1) {
		t.Error("the running version was pruned")
	}

	p.Close(context.Background())
	<-p.Done()
	if _, err := extractRuntime("anfra-test", v2data, nil); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Dir(v1)) {
		t.Errorf("the old version's folder is still there after it exited")
	}
	if !exists(v2) {
		t.Error("the new version is gone")
	}
}

// Libraries unpack beside the binary, and a new library is a new version.
func TestExtractRuntimeLibraries(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	v1, err := extractRuntime("anfra-test", []byte("exe"), []Library{{Name: "duckdb.dll", Data: []byte("v1")}})
	if err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(filepath.Dir(v1), "duckdb.dll")) {
		t.Error("duckdb.dll isn't beside the binary")
	}
	v2, err := extractRuntime("anfra-test", []byte("exe"), []Library{{Name: "duckdb.dll", Data: []byte("v2")}})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(v1) == filepath.Dir(v2) {
		t.Error("a new library unpacked into the same folder")
	}
}

// exists reports whether path is there.
func exists(path string) bool {
	_, err := os.Stat(path) //nolint:gosec // G703: the test's own temp folder
	return err == nil
}

package engine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The engine package, and the shared packages hosts build on (shared/: apperr,
// appstep, apptracing, apikit, httpkit, requestid), are meant to be the ONLY
// importable packages outside internal/. That is
// the property the repo split rests on: everything else can be refactored
// freely because no other module can reach it.
//
// The shared packages are public because a host shares them: an engine error
// reaches it as a formal error, with typed details and the engine's steps,
// rendered by the host's own renderer; and the engine's operations are served by
// the same API framework (apikit, over httpkit and requestid) in `anfra serve`
// and in a host. They are grouped under shared/ so they can move to a module of
// their own.
//
// It is also the property that erodes silently. Adding a package at the top
// level is a one-line mistake that nothing else would catch — the build stays
// green, the tests pass, and an open-source API has quietly grown a second
// entry point that someone will import and then depend on.
//
// Listing what is allowed rather than what is forbidden is deliberate: a new
// top-level package fails this test by default.
func TestOnlyTheAPIPackagesAreExported(t *testing.T) {
	allowed := map[string]bool{
		"engine":            true,
		"shared/apperr":     true,
		"shared/appstep":    true,
		"shared/apptracing": true,
		"shared/apikit":     true,
		"shared/httpkit":    true,
		"shared/jsonkit":    true,
		"shared/requestid":  true,
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}

	var exported []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		base := d.Name()
		// Skip the module root itself, and trees that cannot contribute an
		// importable library package.
		if rel == "." {
			return nil
		}
		if base == "internal" || base == "testdata" || base == "node_modules" ||
			strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") {
			return filepath.SkipDir
		}
		// cmd/ holds package main, which no one can import.
		if rel == "cmd" || strings.HasPrefix(rel, "cmd"+string(filepath.Separator)) {
			return filepath.SkipDir
		}
		if hasGoLibrary(t, path) {
			exported = append(exported, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	for _, pkg := range exported {
		if !allowed[pkg] {
			t.Errorf("package %q is importable from outside this module; "+
				"only engine and shared/{apperr,appstep,apptracing,apikit,httpkit,jsonkit,requestid} are meant to be. Move it under internal/, "+
				"or add it here deliberately and accept that it is public API forever.", pkg)
		}
	}
	if len(exported) == 0 {
		t.Error("found no exported packages at all — this test is not looking where it thinks it is")
	}
}

// hasGoLibrary reports whether dir holds a non-main, non-test Go package.
func hasGoLibrary(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "package ") {
				return strings.TrimSpace(strings.TrimPrefix(line, "package ")) != "main"
			}
		}
	}
	return false
}

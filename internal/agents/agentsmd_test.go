package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBlockIntoANewAgentsMD(t *testing.T) {
	dir := t.TempDir()
	if b, _ := CheckBlock(dir); b.State != BlockMissing {
		t.Errorf("before: %+v", b)
	}
	if wroteBlock, wroteImport, err := WriteBlock(dir); err != nil || !wroteBlock || wroteImport {
		t.Fatalf("wrote %v, %v; err %v", wroteBlock, wroteImport, err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if string(got) != block {
		t.Errorf("AGENTS.md:\n%s", got)
	}
	if b, _ := CheckBlock(dir); b.State != BlockUpToDate || b.NoImport {
		t.Errorf("after: %+v", b)
	}
	// Again: nothing to change.
	if wroteBlock, wroteImport, _ := WriteBlock(dir); wroteBlock || wroteImport {
		t.Error("a second write changed something")
	}
}

// The rest of an existing AGENTS.md stays as it was: the block is added after
// it, then replaced in place.
func TestWriteBlockKeepsTheRest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	mine := "# Our repo\n\nRun the tests."
	if err := os.WriteFile(path, []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteBlock(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != mine+"\n\n"+block {
		t.Errorf("AGENTS.md:\n%s", got)
	}

	// An older block, between other text, is replaced and nothing else moves.
	old := "# Our repo\n\n" + blockBegin + "\nold advice\n" + blockEnd + "\n\n## Tests\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, _ := CheckBlock(dir); b.State != BlockOutdated {
		t.Errorf("an old block: %+v", b)
	}
	if _, _, err := WriteBlock(dir); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != "# Our repo\n\n"+block+"\n## Tests\n" {
		t.Errorf("AGENTS.md:\n%s", got)
	}
}

// Claude Code reads AGENTS.md only without a CLAUDE.md, so an existing one
// gets the import, once; none is created.
func TestWriteBlockImportsItInClaudeMD(t *testing.T) {
	dir := t.TempDir()
	claude := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(claude, []byte("# Notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, _ := CheckBlock(dir); !b.NoImport {
		t.Errorf("before: %+v", b)
	}
	if wroteBlock, wroteImport, err := WriteBlock(dir); err != nil || !wroteBlock || !wroteImport {
		t.Fatalf("wrote %v, %v; err %v", wroteBlock, wroteImport, err)
	}
	got, _ := os.ReadFile(claude)
	if string(got) != "# Notes\n\n@AGENTS.md\n" {
		t.Errorf("CLAUDE.md: %q", got)
	}
	if b, _ := CheckBlock(dir); b.State != BlockUpToDate || b.NoImport {
		t.Errorf("after: %+v", b)
	}
	if wroteBlock, wroteImport, _ := WriteBlock(dir); wroteBlock || wroteImport {
		t.Error("a second write changed something")
	}

	// The block up-to-date, the import gone: only the import is written.
	if err := os.WriteFile(claude, []byte("# Notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if wroteBlock, wroteImport, _ := WriteBlock(dir); wroteBlock || !wroteImport {
		t.Errorf("wrote %v, %v", wroteBlock, wroteImport)
	}

	dir = t.TempDir()
	if _, _, err := WriteBlock(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Error("a CLAUDE.md was created")
	}
}

func TestWriteBlockRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.md")
	if err := os.WriteFile(outside, []byte("not ours"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteBlock(dir); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("got %v, want a refusal", err)
	}
	if got, _ := os.ReadFile(outside); string(got) != "not ours" {
		t.Errorf("the link's target was written: %q", got)
	}
}

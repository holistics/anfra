package cliui

import (
	"bytes"
	"testing"
)

// Each kind of line has its mark; a section's heading shows with its first line,
// after a blank one; quoted text keeps its lines, without escapes.
func TestPrinterMarksLines(t *testing.T) {
	var b bytes.Buffer
	p := NewPrinter(&b)
	p.Step("Validating…")
	p.Section("Empty")
	p.Section("Data sources")
	p.Warn("one is %s", "local")
	p.Note("load it elsewhere")
	p.Fail("two is not reachable:")
	p.Detail("dial error:\n\t\x1b[31mrefused\x1b[0m\n")
	p.Question("Create two anyway?", "create anyway", "skip")
	p.Section("")
	p.Gap()
	p.Success("Published.")
	want := "• Validating…\n\nData sources\n! one is local\n  load it elsewhere\n✗ two is not reachable:\n" +
		"    dial error:\n      refused\n? Create two anyway? [c]reate anyway / [s]kip (default: skip) \n✓ Published.\n"
	if b.String() != want {
		t.Errorf("got\n%q\nwant\n%q", b.String(), want)
	}
}

// A long line wraps under its text, not under its mark.
func TestPrinterWrapsUnderText(t *testing.T) {
	var b bytes.Buffer
	p := &Printer{w: &b, width: 30}
	p.Warn("a data source the server cannot read is not created")
	want := "! a data source the server\n  cannot read is not created\n"
	if b.String() != want {
		t.Errorf("got\n%q\nwant\n%q", b.String(), want)
	}
}

// Choices that say what they do are listed one to a line, and the default is
// named by its name alone.
func TestPrinterListsChoicesThatSayWhatTheyDo(t *testing.T) {
	var b bytes.Buffer
	NewPrinter(&b).Question("Upload it?", "yes", "quit: switch first", "no, skip it")
	want := "? Upload it?\n    [y]es\n    [q]uit: switch first\n    [n]o, skip it\n  (default: no) "
	if b.String() != want {
		t.Errorf("got\n%q\nwant\n%q", b.String(), want)
	}
	if ChoiceName("no, skip it") != "no" || ChoiceName("quit: x") != "quit" || ChoiceName("yes") != "yes" {
		t.Error("ChoiceName")
	}
}

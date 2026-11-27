package workdir

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// A text file's CRLF line breaks read as LF; a lone CR, and a binary file,
// stay as they are.
func TestAsPublished(t *testing.T) {
	binary := append([]byte("PNG\x00\r\n"), bytes.Repeat([]byte("x"), 10)...)
	lateNUL := append(bytes.Repeat([]byte("a\r\n"), 3000), 0) // past 8,000 bytes: text, by git's rule
	for _, c := range []struct{ name, in, want string }{
		{"crlf", "a\r\nb\r\n", "a\nb\n"},
		{"lf", "a\nb\n", "a\nb\n"},
		{"lone cr", "a\rb\r\n", "a\rb\n"},
		{"mixed", "a\r\nb\n", "a\nb\n"},
		{"binary", string(binary), string(binary)},
		{"nul past 8,000 bytes", string(lateNUL), string(bytes.ReplaceAll(lateNUL, []byte("\r\n"), []byte("\n")))},
		{"empty", "", ""},
	} {
		if got := AsPublished([]byte(c.in)); !bytes.Equal(got, []byte(c.want)) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// A folder saved with CRLF hashes as the same folder saved with LF.
func TestReadHashesAsPublished(t *testing.T) {
	lf, crlf := t.TempDir(), t.TempDir()
	for dir, eol := range map[string]string{lf: "\n", crlf: "\r\n"} {
		path := filepath.Join(dir, "models", "a.aml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("Model a {"+eol+"}"+eol), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a, err := Read(lf)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Read(crlf)
	if err != nil {
		t.Fatal(err)
	}
	if a.Files()["models/a.aml"] != b.Files()["models/a.aml"] {
		t.Errorf("LF %s, CRLF %s: want the same blob", a.Files()["models/a.aml"], b.Files()["models/a.aml"])
	}
	content, err := b.ReadBlob(t.Context(), b.Files()["models/a.aml"])
	if err != nil || string(content) != "Model a {\n}\n" {
		t.Errorf("ReadBlob: %q, %v; want the LF content", content, err)
	}
}

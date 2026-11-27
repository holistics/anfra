package workdir

import (
	"bytes"
	"os"
)

// AsPublished is content as publish sends it, and as pull compares it: a text
// file's CRLF line breaks as LF, as git stores text, so a file saved on Windows
// matches the same file saved on a Mac. A binary file (a zero byte in its first
// 8,000 bytes, git's rule) and a lone CR are left as they are.
func AsPublished(content []byte) []byte {
	if binary(content) || !bytes.Contains(content, []byte("\r\n")) {
		return content
	}
	return bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
}

// binary reports whether content is a binary file, by git's rule.
func binary(content []byte) bool {
	return bytes.IndexByte(content[:min(len(content), 8000)], 0) >= 0
}

// ReadFile reads the file at path as published (AsPublished).
func ReadFile(path string) ([]byte, error) {
	content, err := os.ReadFile(path) //nolint:gosec // G304: a file of the folder being published or pulled into
	if err != nil {
		return nil, err
	}
	return AsPublished(content), nil
}

// allCRLF reports whether content is a text file whose every line break is
// CRLF: one saved on Windows, which pull writes back the same way, as an
// editor does.
func allCRLF(content []byte) bool {
	lf := bytes.Count(content, []byte("\n"))
	return lf > 0 && !binary(content) && bytes.Count(content, []byte("\r\n")) == lf
}

// withCRLF is LF text content with its line breaks as CRLF.
func withCRLF(content []byte) []byte {
	if binary(content) {
		return content
	}
	return bytes.ReplaceAll(AsPublished(content), []byte("\n"), []byte("\r\n"))
}

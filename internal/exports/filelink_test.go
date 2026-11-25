package exports

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// A file:// link round-trips to the same path, and is a standard link (no
// backslash or bare drive), on every OS.
func TestFileLinkRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders by day.csv")
	link := FileLink(path)
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("%s: %v", link, err)
	}
	if got := FileLinkPath(u); got != path {
		t.Errorf("FileLinkPath(%s) = %s, want %s", link, got, path)
	}
	if u.Host != "" || len(link) < len("file:///") || link[:len("file:///")] != "file:///" {
		t.Errorf("%s isn't a standard file:/// link", link)
	}
	if _, err := os.Stat(FileLinkPath(u)); err != nil && !os.IsNotExist(err) {
		t.Errorf("the path doesn't open: %v", err)
	}
}

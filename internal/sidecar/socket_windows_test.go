//go:build windows

package sidecar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// checkPrivate fails t unless only this user can open dir: its access list
// gives this user alone access, and takes nothing from the folder above.
func checkPrivate(t *testing.T, dir string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sd.String(), "D:P(A;OICI;FA;;;"+user.User.Sid.String()+")"; got != want {
		t.Errorf("%s's access list = %s, want %s (this user only)", dir, got, want)
	}
}

// With TMP at a folder everyone may open, as C:\Temp can be, the socket's
// folder is still this user's alone.
func TestSocketPathInSharedTemp(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "shared")
	sd, err := windows.SecurityDescriptorFromString("D:(A;OICI;FA;;;WD)") // Everyone: full access, passed down
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(shared, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMP", shared)

	path, err := SocketPath("canal-query")
	if err != nil {
		t.Fatal(err)
	}
	defer RemoveSocket(path)
	if !strings.HasPrefix(path, shared) {
		t.Fatalf("socket %s isn't in TMP %s", path, shared)
	}
	checkPrivate(t, filepath.Dir(path))
}

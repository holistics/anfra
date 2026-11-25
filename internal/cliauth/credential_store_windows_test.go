//go:build windows

package cliauth

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func currentUserSID(t *testing.T) string {
	t.Helper()
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return u.User.Sid.String()
}

// setDACL gives path exactly the access list sddl (protected: nothing inherited).
func setDACL(t *testing.T, path, sddl string) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
}

// save writes an inventory through the store, with its lock, as login does.
func save(t *testing.T, token string) error {
	t.Helper()
	return withCredentialLock(context.Background(), func(inv *credentialInventory, path string) error {
		inv.Entries["org"] = storedCredential{AccessToken: token}
		return saveCredentialInventory(path, inv)
	})
}

// Saving twice reads the file back and replaces it: the read must not keep it open.
func TestCredentialsSaveTwice(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	if err := save(t, "one"); err != nil {
		t.Fatal(err)
	}
	if err := save(t, "two"); err != nil {
		t.Fatal(err)
	}
}

// The saved file is this user's alone: no mode 600 on Windows, so its access list.
func TestCredentialsSavedPrivate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ANFRA_HOME", home)
	if err := save(t, "one"); err != nil {
		t.Fatal(err)
	}
	assertPrivateFile(t, filepath.Join(home, "credentials.json"))
}

// assertPrivateFile fails t unless path's access list is protected (nothing
// inherited) and holds one entry: this user, full access.
func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	got := sd.String() // D:P<flags>(A;;FA;;;<sid>)
	head, entries, _ := strings.Cut(got, "(")
	if !strings.HasPrefix(head, "D:P") || "("+entries != "(A;;FA;;;"+currentUserSID(t)+")" {
		t.Errorf("%s's access list is %s, want this user only, nothing inherited", path, got)
	}
}

// A file others can read is refused, with how to fix it.
func TestCredentialsRefusedWhenShared(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ANFRA_HOME", home)
	if err := save(t, "one"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "credentials.json")
	setDACL(t, path, "D:P(A;;FA;;;"+currentUserSID(t)+")(A;;FR;;;WD)") // + Everyone: read
	err := save(t, "two")
	if err == nil || !strings.Contains(err.Error(), "icacls") {
		t.Errorf("got %v, want a refusal that says how to fix it with icacls", err)
	}
}

// The list Windows gives a file in a user's own folder (the user, SYSTEM,
// Administrators) is accepted: SYSTEM and Administrators stand where root does.
func TestCredentialsUsualListAccepted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ANFRA_HOME", home)
	if err := save(t, "one"); err != nil {
		t.Fatal(err)
	}
	setDACL(t, filepath.Join(home, "credentials.json"), "D:P(A;;FA;;;"+currentUserSID(t)+")(A;;FA;;;SY)(A;;FA;;;BA)")
	if err := save(t, "two"); err != nil {
		t.Fatal(err)
	}
}

// Marking a refresh to recover syncs the folder, which must work on Windows.
func TestSyncCredentialDirectory(t *testing.T) {
	if err := syncCredentialDirectory(filepath.Join(t.TempDir(), "credentials.json")); err != nil {
		t.Fatal(err)
	}
}

//go:build windows

package sidecar

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// privateTempDir makes a new folder in the temp dir that only this user can
// open. Windows ignores Unix modes, so os.MkdirTemp's 700 doesn't hold here:
// the folder is made with an access list of its own, giving this user full
// access and taking nothing from the temp dir, which TMP may point at a shared
// folder such as C:\Temp. It is set as the folder is made, so no one can open
// it in between.
func privateTempDir() (string, error) {
	sa, err := ownerOnly()
	if err != nil {
		return "", err
	}
	for range 10000 {
		dir := filepath.Join(os.TempDir(), "anfra-"+rand.Text()[:10])
		p, err := windows.UTF16PtrFromString(dir)
		if err != nil {
			return "", err
		}
		err = windows.CreateDirectory(p, sa)
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return "", &os.PathError{Op: "mkdir", Path: dir, Err: err}
		}
	}
	return "", errors.New("no free name for a folder in the temp dir")
}

// ownerOnly is an access list giving this user full access, passed down to
// what the folder holds, and nothing else: protected (P), so nothing is
// inherited from the folder above.
func ownerOnly() (*windows.SecurityAttributes, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return nil, err
	}
	sa := &windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(*sa))
	return sa, nil
}

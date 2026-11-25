//go:build windows

package cliauth

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/holistics/anfra/internal/clierr"
)

// makePrivate gives the new authentication file an access list of its own,
// this user only and nothing inherited from its folder: Windows has no mode
// 600, and ANFRA_HOME may be a folder others can open. It is set before any
// token is written; the rename over the old file keeps it.
func makePrivate(f *os.File) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(f.Name(), windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// checkPrivate refuses the authentication file when its access list lets
// anyone but this user read it. SYSTEM and Administrators may: they stand where
// root does on Unix, which mode 600 doesn't keep out either.
func checkPrivate(path string, _ os.FileInfo) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return clierr.Precondition("can't check who can read the authentication file (%s): %v", path, err)
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return clierr.Precondition("can't check who can read the authentication file (%s): %v", path, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil { // no list at all lets everyone in
		return sharedCredentialFile(path, user.User.Sid)
	}
	const read = windows.FILE_READ_DATA | windows.GENERIC_READ | windows.GENERIC_ALL
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return sharedCredentialFile(path, user.User.Sid)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask&read == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.Equals(user.User.Sid) && !sid.IsWellKnown(windows.WinLocalSystemSid) && !sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			return sharedCredentialFile(path, user.User.Sid)
		}
	}
	return nil
}

func sharedCredentialFile(path string, user *windows.SID) error {
	name := user.String()
	if account, domain, _, err := user.LookupAccount(""); err == nil {
		name = domain + `\` + account
	}
	return clierr.Precondition("can't use the authentication file (%s), because others can read it, not only you. To fix it, run `icacls \"%s\" /inheritance:r /grant:r \"%s:F\"`", path, path, name)
}

// syncCredentialDirectory has nothing to do: Windows can't flush a folder
// (Access is denied), and NTFS journals the rename that saved the file.
func syncCredentialDirectory(string) error { return nil }

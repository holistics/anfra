//go:build windows

package fileperm

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// OwnerOnly gives f an access list of its own: this user, full access, and
// nothing inherited from its folder, which may be one others can open.
func OwnerOnly(f *os.File) error {
	sid, err := userSID()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(ownerOnly(sid))
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

// IsOwnerOnly reports whether the file at path has the access list OwnerOnly
// gives: protected, with one entry, this user's.
func IsOwnerOnly(path string) (bool, error) {
	sid, err := userSID()
	if err != nil {
		return false, err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, err
	}
	head, entries, _ := strings.Cut(sd.String(), "(") // D:P<flags>(A;;FA;;;<sid>)
	return strings.HasPrefix(head, "D:P") && "("+entries == "(A;;FA;;;"+sid+")", nil
}

func ownerOnly(sid string) string { return "D:P(A;;FA;;;" + sid + ")" }

func userSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

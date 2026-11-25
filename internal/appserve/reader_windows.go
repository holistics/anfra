//go:build windows

package appserve

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows names zones its own way ("SE Asia Standard Time"); the ICU it ships
// (icu.dll, Windows 10 1903 and later) gives the system's zone as an IANA name.
var (
	icu                    = windows.NewLazySystemDLL("icu.dll")
	ucalGetDefaultTimeZone = icu.NewProc("ucal_getDefaultTimeZone")
)

// systemTimezone is the IANA name of Windows' zone, or "" when ICU is missing
// or can't tell.
func systemTimezone() string {
	if ucalGetDefaultTimeZone.Find() != nil {
		return ""
	}
	var buf [128]uint16
	var status int32 // ICU's UErrorCode: above 0 is a failure
	n, _, _ := ucalGetDefaultTimeZone.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&status)))
	if status > 0 || int32(n) <= 0 || int(n) > len(buf) {
		return ""
	}
	if tz := windows.UTF16ToString(buf[:n]); tz != "Etc/Unknown" {
		return tz
	}
	return ""
}

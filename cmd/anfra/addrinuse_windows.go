//go:build windows

package main

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isAddrInUse reports whether listening failed because the address is taken:
// Windows' own error, which syscall.EADDRINUSE doesn't match.
func isAddrInUse(err error) bool { return errors.Is(err, windows.WSAEADDRINUSE) }

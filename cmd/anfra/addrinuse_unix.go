//go:build !windows

package main

import (
	"errors"
	"syscall"
)

// isAddrInUse reports whether listening failed because the address is taken.
func isAddrInUse(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }

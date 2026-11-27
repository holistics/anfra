//go:build !windows

package cliui

import "os"

// EnableEscapeCodes has nothing to do: Unix terminals show escape codes.
func EnableEscapeCodes(...*os.File) {}

func escapesOff(any) bool { return false }

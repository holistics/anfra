//go:build windows

package pretty

// utf8Terminal is true: Windows sets no locale variables, and Go writes to a
// console in UTF-16, so it shows tree's lines whatever its code page.
func utf8Terminal() bool { return true }

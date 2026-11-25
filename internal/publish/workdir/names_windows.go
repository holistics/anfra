//go:build windows

package workdir

import (
	"fmt"
	"strings"
)

// nameProblem is why Windows can't hold part, a file or folder name, as written,
// or "" when it can: a device name reaches the device, a colon a file's hidden
// stream, and a trailing dot or space is dropped, which lands on another file.
func nameProblem(part string) string {
	base, _, _ := strings.Cut(part, ".")
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return fmt.Sprintf("Windows keeps the name %s for a device", base)
	}
	if i := strings.IndexAny(part, `:<>"|?*`); i >= 0 {
		return fmt.Sprintf("it holds %c, which Windows doesn't allow in a name", part[i])
	}
	for _, r := range part {
		if r < 0x20 {
			return "it holds a control character, which Windows doesn't allow in a name"
		}
	}
	if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
		return fmt.Sprintf("%q ends in a dot or a space, which Windows drops", part)
	}
	return ""
}

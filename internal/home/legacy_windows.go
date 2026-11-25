//go:build windows

package home

// legacyDir is "": anfra never kept files outside Dir on Windows, so
// %LOCALAPPDATA%\anfra, if there is one, isn't anfra's to remove.
func legacyDir() string { return "" }

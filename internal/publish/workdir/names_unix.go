//go:build !windows

package workdir

// nameProblem is "": Linux and macOS hold any name publish sends.
func nameProblem(string) string { return "" }

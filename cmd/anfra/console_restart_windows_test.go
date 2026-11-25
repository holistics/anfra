//go:build windows

package main

import "testing"

// The new anfra runs with the console's arguments and the restarted mark, and
// its exit code is the console's.
func TestRestartWith(t *testing.T) {
	code, err := restartWith("cmd.exe", []string{"/c", "if defined " + consoleRestarted + " (exit 3) else (exit 4)"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 3 {
		t.Errorf("exit code %d, want 3: the new anfra didn't see %s", code, consoleRestarted)
	}
}

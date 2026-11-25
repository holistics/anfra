//go:build windows

package main

import (
	"os"
	"os/exec"
)

// restartConsole runs the console again on the anfra now at exe, the path it
// was started from: the terminal, the folder and the history carry on. Windows
// has no exec, so this process runs the new anfra, waits for it, and exits with
// its code.
func restartConsole(exe string) error {
	code, err := restartWith(exe, os.Args[1:])
	if err != nil {
		return err
	}
	os.Exit(code)
	return nil
}

// restartWith runs exe with args in this process's terminal, marked as a
// restarted console, and answers its exit code.
func restartWith(exe string, args []string) (int, error) {
	cmd := exec.Command(exe, args...) //nolint:gosec // our own binary, our own args
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), consoleRestarted+"=1")
	if err := cmd.Run(); err != nil {
		if cmd.ProcessState != nil {
			return cmd.ProcessState.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

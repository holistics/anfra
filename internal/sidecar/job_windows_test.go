//go:build windows

package sidecar

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// A sidecar dies with anfra, even when anfra is killed and never stops it.
func TestSidecarDiesWithHost(t *testing.T) {
	host := exec.Command(os.Args[0], "-test.run=^$") //nolint:gosec // the test binary itself
	host.Env = append(os.Environ(), helperEnv+"=host")
	out, err := host.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("the host didn't say its sidecar's pid: %v", err)
	}
	pid, err := strconv.ParseUint(strings.TrimSpace(line), 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(sidecar) //nolint:errcheck // a test

	_ = host.Process.Kill() // TerminateProcess: the host gets no chance to stop it
	_ = host.Wait()

	if ev, _ := windows.WaitForSingleObject(sidecar, 1000); ev != windows.WAIT_OBJECT_0 {
		t.Error("the sidecar is still running 1s after its host was killed")
	}
}

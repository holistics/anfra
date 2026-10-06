//go:build linux

package anfra

import (
	"os/exec"
	"syscall"
)

// If the demo dies without a chance to stop anfra (SIGKILL), the kernel stops anfra too.
func setParentDeathSignal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}

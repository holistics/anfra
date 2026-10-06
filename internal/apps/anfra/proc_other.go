//go:build !linux

package anfra

import "os/exec"

func setParentDeathSignal(*exec.Cmd) {}

//go:build windows

package sidecar

import (
	"fmt"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// tieToHost puts the sidecar in anfra's Job Object, which Windows closes when
// anfra ends, however it ends, killing every process in it: Windows' answer to
// Linux's Pdeathsig. The job is never closed by anfra itself.
func tieToHost(cmd *exec.Cmd) error {
	job, err := hostJob()
	if err != nil {
		return err
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)) //nolint:gosec // G115: a pid fits a DWORD
	if err != nil {
		return fmt.Errorf("open the process: %w", err)
	}
	defer windows.CloseHandle(proc) //nolint:errcheck // nothing to do if it fails
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		return fmt.Errorf("add it to anfra's job: %w", err)
	}
	return nil
}

// hostJob is anfra's Job Object, made on the first sidecar start.
var hostJob = sync.OnceValues(func() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create anfra's job: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job) //nolint:errcheck // nothing to do if it fails
		return 0, fmt.Errorf("set anfra's job to end its processes: %w", err)
	}
	return job, nil
})

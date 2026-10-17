//go:build unix

package config

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configurePasswordProcess(cmd *exec.Cmd) {
	// Kill the shell and its children on timeout, not just the shell wrapper.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

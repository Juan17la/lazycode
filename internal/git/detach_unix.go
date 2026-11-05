//go:build unix

package git

import (
	"os/exec"
	"syscall"
)

// Detach gives git no controlling terminal, so that ssh and credential programs can't ask
// on /dev/tty and draw over the screen. Stopping it stops everything it started.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

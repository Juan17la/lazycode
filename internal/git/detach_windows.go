package git

import (
	"os/exec"
	"syscall"
)

// Detach gives git no console, so that ssh and credential programs can't ask on it.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

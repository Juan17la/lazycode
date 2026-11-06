//go:build unix

package lsp

import (
	"os/exec"
	"syscall"
)

// A server often starts helpers, such as gopls running the go tool. Putting them in one process group
// lets killing the server kill them too, so none stay behind.
func prepare(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func killTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) //nolint:errcheck // It may be gone already.
		cmd.Process.Kill()                              //nolint:errcheck // As above.
	}
}

//go:build unix

package editor

import (
	"errors"
	"os"
	"syscall"
)

// processAlive errs towards true: a reused process ID only delays an offer until that
// process exits, and the file stays safe meanwhile.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

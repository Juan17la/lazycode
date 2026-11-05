package app

import "os"

// FindProcess opens the process on Windows, so it fails once the process has gone.
func processGone(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	p.Release() //nolint:errcheck // Only frees the handle.
	return false
}

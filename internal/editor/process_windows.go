package editor

import "os"

// processAlive errs towards true: a reused process ID only delays an offer until that
// process exits, and the file stays safe meanwhile.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid) // On Windows this fails if the process is gone.
	if err != nil {
		return false
	}
	process.Release() //nolint:errcheck // Only frees the handle just opened.
	return true
}

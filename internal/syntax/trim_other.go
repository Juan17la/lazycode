//go:build !linux

package syntax

// ReleaseMemory does nothing here: only glibc keeps freed memory this way.
func ReleaseMemory() {}

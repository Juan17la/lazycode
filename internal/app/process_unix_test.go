//go:build unix

package app

import "syscall"

func processGone(pid int) bool { return syscall.Kill(pid, 0) != nil }

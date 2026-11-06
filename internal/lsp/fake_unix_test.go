//go:build unix

package lsp

import (
	"os/signal"
	"syscall"
)

func init() { signalIgnore = func() { signal.Ignore(syscall.SIGTERM, syscall.SIGHUP) } }

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

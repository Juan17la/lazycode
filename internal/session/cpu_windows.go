package session

import (
	"syscall"
	"time"
	"unsafe"
)

// TreeCPU returns the total CPU used so far by a process and everything it started.
func TreeCPU(pid int) (time.Duration, error) {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer syscall.CloseHandle(snapshot) //nolint:errcheck // Only frees the handle.
	parent := map[int]int{}
	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		parent[int(entry.ProcessID)] = int(entry.ParentProcessID)
	}
	var total time.Duration
	for candidate := range parent {
		for at, steps := candidate, 0; at != 0 && steps < 4096; at, steps = parent[at], steps+1 {
			if at == pid {
				total += processCPU(candidate)
				break
			}
		}
	}
	return total, nil
}

func processCPU(pid int) time.Duration {
	const queryLimitedInformation = 0x1000
	handle, err := syscall.OpenProcess(queryLimitedInformation, false, uint32(pid))
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(handle) //nolint:errcheck // Only frees the handle.
	var creation, exit, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return 0
	}
	return time.Duration(kernel.Nanoseconds() + user.Nanoseconds())
}

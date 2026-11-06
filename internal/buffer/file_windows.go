package buffer

import (
	"io/fs"
	"os"
	"syscall"
	"unsafe"
)

// mapFile maps a read-only file too big to read into memory. While it's mapped, Windows
// refuses to let other programs truncate it, so the crash possible on other systems
// can't happen here.
func mapFile(file *os.File, size int64) ([]byte, func() error, error) {
	mapping, err := syscall.CreateFileMapping(syscall.Handle(file.Fd()), nil, syscall.PAGE_READONLY, 0, 0, nil)
	if err != nil {
		return nil, nil, err
	}
	defer syscall.CloseHandle(mapping) //nolint:errcheck // The view keeps the mapping alive.
	addr, err := syscall.MapViewOfFile(mapping, syscall.FILE_MAP_READ, 0, 0, 0)
	if err != nil {
		return nil, nil, err
	}
	// The address comes from the OS, not the Go heap, so the garbage collector won't move it.
	data := unsafe.Slice(*(**byte)(unsafe.Pointer(&addr)), size)
	return data, func() error { return syscall.UnmapViewOfFile(addr) }, nil
}

// keepOwner does nothing: a new file's owner and ACLs come from its folder here.
func keepOwner(*os.File, fs.FileInfo) {}

// syncDir does nothing: Windows has no way to flush a folder.
func syncDir(string) {}

//go:build unix

package buffer

import (
	"io/fs"
	"os"
	"syscall"
)

// mapFile maps a read-only file too big to read into memory.
//
// ponytail: truncating the file while it's mapped crashes with SIGBUS on the next read
// of a lost page, such as a log rotated by truncation. Recovery files protect other
// unsaved work. Reading pages on demand is the upgrade if this happens in practice.
func mapFile(file *os.File, size int64) ([]byte, func() error, error) {
	data, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return nil, nil, err
	}
	return data, func() error { return syscall.Munmap(data) }, nil
}

// keepOwner matters when editing with sudo, where the new file would otherwise belong
// to root. Without the rights to change it, the owner becomes the user saving.
func keepOwner(temp *os.File, info fs.FileInfo) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		temp.Chown(int(st.Uid), -1) //nolint:errcheck // Allowed to fail, see above.
		temp.Chown(-1, int(st.Gid)) //nolint:errcheck // Allowed to fail, see above.
	}
}

// syncDir makes a finished rename survive a power loss. The file's data is already
// safe and the rename has happened, so a failure here isn't a failed save.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync() //nolint:errcheck // See above.
		d.Close()
	}
}

package buffer

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"hash/maphash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const (
	// readOnlyAbove is the largest file that can be edited (D-32).
	readOnlyAbove = 100 << 20
	// binarySniff is how much of the start of a file is checked for NUL bytes.
	binarySniff = 8 * 1024
)

var (
	// ErrBinary means the file holds NUL bytes near its start. That includes UTF-16
	// text, until other encodings are supported.
	ErrBinary = errors.New("this is a binary file")
	// ErrNotRegular means the path is a directory, pipe or device.
	ErrNotRegular = errors.New("this isn't a regular file")
	// ErrChangedOnDisk means something else changed the file since it was loaded or
	// saved. Nothing was written. The caller lets the user choose Overwrite or Reload.
	ErrChangedOnDisk = errors.New("this file changed on disk")
)

// hashSeed only lives as long as the process, and so do the hashes made with it.
var hashSeed = maphash.MakeSeed()

// File is a Buffer tied to a path on disk. Like Buffer, one goroutine owns it.
type File struct {
	*Buffer
	Path string // Absolute.

	saved   *node // The tree as it was at the last load or save.
	onDisk  bool  // False if the file didn't exist at the last load or save.
	size    int64
	hash    uint64
	modTime time.Time    // Only for a mapped file, which is too big to hash each time.
	unmap   func() error // Set if the original text is a memory mapping.
}

// Open loads a file. A file over 100 MiB opens read-only.
func Open(path string) (*File, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Stat first: only opening a pipe would block until something writes to it.
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	source, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	// Taken before reading, so a change made during the read counts as a change.
	if info, err = source.Stat(); err != nil {
		return nil, err
	}

	start := make([]byte, binarySniff)
	n, err := source.ReadAt(start, 0)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if bytes.IndexByte(start[:n], 0) >= 0 {
		return nil, fmt.Errorf("%s: %w", path, ErrBinary)
	}

	f := &File{Path: path, onDisk: true, size: info.Size()}
	if info.Size() > readOnlyAbove {
		data, unmap, err := mapFile(source, info.Size())
		if err != nil {
			return nil, fmt.Errorf("map %s: %w", path, err)
		}
		f.Buffer, f.unmap, f.modTime = New(data), unmap, info.ModTime()
		f.readOnly = true
		f.saved = f.root
		return f, nil
	}

	// Editable files are read, not mapped. Another program rewriting a mapped file
	// changes the text under the buffer, so saving could write a corrupt mix, and
	// truncating it crashes the editor with SIGBUS. Both were tested on Linux, and AI
	// tools rewrite open files all the time. Mapping saves little anyway: scanning
	// for lines brings the whole file into RSS.
	var data bytes.Buffer
	data.Grow(int(info.Size()) + bytes.MinRead)
	if _, err := data.ReadFrom(source); err != nil {
		return nil, err
	}
	f.Buffer = New(data.Bytes())
	f.size = int64(data.Len())
	f.hash = maphash.Bytes(hashSeed, data.Bytes())
	f.saved = f.root
	return f, nil
}

// Create returns an empty File for a path that doesn't exist yet. Nothing is written
// until Save.
func Create(path string) (*File, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &File{Buffer: New(nil), Path: path}, nil
}

// Modified reports whether the text differs from the last load or save. Undoing back
// to that point makes it false again.
func (f *File) Modified() bool { return f.root != f.saved }

// ChangedOnDisk reports whether the file on disk differs from the last load or save.
// It compares contents, not times: a timestamp can miss a quick same-size rewrite, and
// a touch isn't a change. It reads the file, so it's for saving and for file change
// events, not for every frame.
func (f *File) ChangedOnDisk() (bool, error) {
	info, err := os.Stat(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return f.onDisk, nil
	}
	if err != nil {
		return false, err
	}
	if !f.onDisk || !info.Mode().IsRegular() || info.Size() != f.size {
		return true, nil
	}
	if f.unmap != nil {
		return !info.ModTime().Equal(f.modTime), nil
	}
	source, err := os.Open(f.Path)
	if err != nil {
		return false, err
	}
	defer source.Close()
	var hash maphash.Hash
	hash.SetSeed(hashSeed)
	if _, err := io.Copy(&hash, source); err != nil {
		return false, err
	}
	return hash.Sum64() != f.hash, nil
}

// Save writes the text, unless the file changed on disk, which gives ErrChangedOnDisk.
// A failed save leaves the file on disk as it was, and the edits still unsaved.
func (f *File) Save() error {
	if f.readOnly {
		return ErrReadOnly
	}
	if err := f.checkDisk(); err != nil {
		return err
	}
	if !f.Modified() && f.onDisk {
		return nil // Rewriting it would only wake up tools watching the file.
	}
	return f.write(f.checkDisk)
}

// Overwrite saves even if the file changed on disk. Only call it once the user has
// chosen to replace what's there.
func (f *File) Overwrite() error {
	if f.readOnly {
		return ErrReadOnly
	}
	return f.write(func() error { return nil })
}

func (f *File) checkDisk() error {
	changed, err := f.ChangedOnDisk()
	if err != nil {
		return err
	}
	if changed {
		return fmt.Errorf("%s: %w", f.Path, ErrChangedOnDisk)
	}
	return nil
}

// write makes a temporary file next to the target, flushes it, then renames it over
// the target, so a crash or failure at any point leaves one complete version.
// beforeRename is the last check, kept as close to the rename as possible.
//
// ponytail: blocks its caller, so very large saves pause the UI. Saving a Snapshot on
// another goroutine is the upgrade if that gets noticed.
// ponytail: renaming replaces the file, so a hard link elsewhere keeps the old text,
// and extended attributes and Windows ACLs aren't copied. Writing in place would keep
// them, but it can't protect the file from a failed write.
func (f *File) write(beforeRename func() error) (err error) {
	target := f.Path
	// Replace what a symlink points at, not the symlink. A missing file or a broken
	// link has no target to resolve, so the path itself is used.
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	}
	info, err := os.Stat(target)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	perm := fs.FileMode(0o666) // For a new file, the OS applies the user's umask.
	if exists {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s: %w", target, ErrNotRegular)
		}
		// Renaming only needs a writable folder, so it would replace a file the user
		// marked read-only.
		probe, err := os.OpenFile(target, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		probe.Close()
		perm = 0o600 // Stays private until the real permissions are set below.
	}

	temp, err := createTemp(filepath.Dir(target), perm)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			temp.Close()
			os.Remove(temp.Name())
		}
	}()

	var hash maphash.Hash
	hash.SetSeed(hashSeed)
	out := bufio.NewWriterSize(io.MultiWriter(temp, &hash), 64*1024)
	size, err := f.WriteTo(out)
	if err != nil {
		return err
	}
	if err = out.Flush(); err != nil {
		return err
	}
	if exists {
		if err = temp.Chmod(info.Mode().Perm()); err != nil {
			return err
		}
		keepOwner(temp, info)
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = beforeRename(); err != nil {
		return err
	}
	if err = os.Rename(temp.Name(), target); err != nil {
		return err
	}
	syncDir(filepath.Dir(target))

	f.saved, f.onDisk, f.size, f.hash = f.root, true, size, hash.Sum64()
	return nil
}

func createTemp(dir string, perm fs.FileMode) (*os.File, error) {
	for {
		// The short fixed name still works when the real name is at the length limit.
		temp, err := os.OpenFile(filepath.Join(dir, ".lazycode-"+rand.Text()), os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if !errors.Is(err, fs.ErrExist) {
			return temp, err
		}
	}
}

// Reload replaces the text with the file on disk, which discards unsaved edits and
// the undo history, because the old original text is gone. If it can't load the file,
// it returns the error and changes nothing.
func (f *File) Reload() error {
	fresh, err := Open(f.Path)
	if err != nil {
		return err
	}
	f.Close() //nolint:errcheck // The new text is loaded, and the old mapping can't affect it.
	*f = *fresh
	return nil
}

// Close frees a mapped file's memory. Snapshots of that file must no longer be in use.
// The File reads as empty afterwards.
func (f *File) Close() error {
	if f.unmap == nil {
		return nil
	}
	unmap := f.unmap
	f.unmap = nil
	f.Buffer = New(nil)
	f.readOnly = true
	f.saved = f.root
	return unmap()
}

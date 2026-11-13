// Package editor holds what happens around a buffer while it's edited: the cursor,
// selection, editing commands, what to draw, and recovery files, which keep unsaved
// edits safe through a crash (D-29).
package editor

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Juan17la/lazycode/internal/buffer"
)

const (
	recoveryDelay   = time.Second // After typing stops.
	recoveryExt     = ".recovery"
	recoveryVersion = 1
	maxHeader       = 64 * 1024
	// staleTemp is when a half-written file must be left over from a crash.
	staleTemp = time.Hour
)

// header is the first line of a recovery file. The text follows it unchanged.
type header struct {
	Version int
	Path    string
	Pid     int // A running process's files aren't offered: that isn't a crash.
	Time    time.Time
}

// Recovery keeps recovery files in one folder. Schedule and Discard never block on
// the disk, so the UI goroutine can call them.
type Recovery struct {
	dir     string
	delay   time.Duration
	onError func(path string, err error)

	mu     sync.Mutex
	latest map[string]uint64 // Per path, so an older request can tell it's been replaced.
	timers map[string]*time.Timer

	diskMu sync.Mutex // Only one file at a time is written or removed.
	wg     sync.WaitGroup
}

// RecoveryDir returns the folder for recovery files. It's in the config folder and not
// the cache, which the OS is allowed to clear.
func RecoveryDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazycode", "recovery"), nil
}

// NewRecovery uses dir, creating it on the first write. onError runs on another
// goroutine when a write fails, since the user's edits are then unprotected.
func NewRecovery(dir string, onError func(path string, err error)) *Recovery {
	return &Recovery{
		dir:     dir,
		delay:   recoveryDelay,
		onError: onError,
		latest:  map[string]uint64{},
		timers:  map[string]*time.Timer{},
	}
}

// fileFor names a path's recovery file. A hash means any path makes a safe, short name.
func (r *Recovery) fileFor(path string) string {
	sum := sha256.Sum256([]byte(path))
	return filepath.Join(r.dir, hex.EncodeToString(sum[:])+recoveryExt)
}

// next records a new request for path, and cancels what's waiting.
func (r *Recovery) next(path string) uint64 {
	r.latest[path]++
	if timer := r.timers[path]; timer != nil {
		if timer.Stop() {
			r.wg.Done()
		}
		delete(r.timers, path)
	}
	return r.latest[path]
}

// isLatest is checked while holding diskMu, so an old request can't undo a newer one.
func (r *Recovery) isLatest(path string, request uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latest[path] == request
}

// Schedule saves text as path's recovery file about a second after the last call for
// that path. Call it after every edit.
//
// ponytail: writes the whole text each time, which is a lot of disk for a 100 MB file.
// Storing only the changed pieces is the upgrade, but then recovery depends on the
// original still being on disk, and an AI tool may have rewritten it.
func (r *Recovery) Schedule(path string, text buffer.Snapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request := r.next(path)
	r.wg.Add(1)
	r.timers[path] = time.AfterFunc(r.delay, func() {
		defer r.wg.Done()
		r.diskMu.Lock()
		defer r.diskMu.Unlock()
		if !r.isLatest(path, request) {
			return
		}
		if err := r.write(path, text, os.Getpid()); err != nil && r.onError != nil {
			r.onError(path, err)
		}
	})
}

// Discard removes path's recovery file and cancels any waiting write. Call it when the
// edits are saved or thrown away.
func (r *Recovery) Discard(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request := r.next(path)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.diskMu.Lock()
		defer r.diskMu.Unlock()
		if !r.isLatest(path, request) {
			return // Newer edits came in, and their write replaces the file.
		}
		err := os.Remove(r.fileFor(path))
		if err != nil && !errors.Is(err, os.ErrNotExist) && r.onError != nil {
			r.onError(path, err)
		}
	}()
}

// Wait blocks until everything scheduled so far has finished, including waiting writes.
// Call it before exiting.
func (r *Recovery) Wait() { r.wg.Wait() }

// write replaces the recovery file in one step, so a crash during it keeps the last one.
func (r *Recovery) write(path string, text buffer.Snapshot, pid int) (err error) {
	// The contents can hold secrets, so only the user may read them. CreateTemp makes
	// private files, and Chmod covers a folder that already existed.
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(r.dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(r.dir, "*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			temp.Close()
			os.Remove(temp.Name())
		}
	}()
	out := bufio.NewWriterSize(temp, 64*1024)
	head, err := json.Marshal(header{recoveryVersion, path, pid, time.Now()})
	if err != nil {
		return err
	}
	if _, err = out.Write(append(head, '\n')); err != nil {
		return err
	}
	if _, err = text.WriteTo(out); err != nil {
		return err
	}
	if err = out.Flush(); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), r.fileFor(path))
}

// Recovered is unsaved work left behind by a process that's gone.
type Recovered struct {
	Path string
	Time time.Time // When it was written.
	file string
}

// List returns what can be recovered, oldest first. Files it can't understand are
// reported in the error and left in place, as they may hold someone's only copy. A
// missing folder just means there's nothing to recover.
func (r *Recovery) List() ([]Recovered, error) {
	entries, err := os.ReadDir(r.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var found []Recovered
	var problems []error
	for _, entry := range entries {
		file := filepath.Join(r.dir, entry.Name())
		if strings.HasSuffix(entry.Name(), ".tmp") {
			// A crash during a write. It may hold secrets, so don't leave it forever.
			if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > staleTemp {
				os.Remove(file)
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), recoveryExt) {
			continue
		}
		head, _, closeFile, err := openRecovery(file)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		closeFile()
		if !processAlive(head.Pid) {
			found = append(found, Recovered{head.Path, head.Time, file})
		}
	}
	slices.SortFunc(found, func(a, b Recovered) int { return a.Time.Compare(b.Time) })
	return found, errors.Join(problems...)
}

// Read returns the saved text. To restore it, load or create the file, then replace all
// of its text as one edit. That leaves it unsaved, with undo going back to what's on
// disk now, and only the user's Save can change the file.
func (rec Recovered) Read() ([]byte, error) {
	_, body, closeFile, err := openRecovery(rec.file)
	if err != nil {
		return nil, err
	}
	defer closeFile()
	return io.ReadAll(body)
}

func openRecovery(file string) (header, io.Reader, func(), error) {
	f, err := os.Open(file)
	if err != nil {
		return header{}, nil, nil, err
	}
	in := bufio.NewReaderSize(f, maxHeader) // Also limits how much a broken file can make it read.
	line, err := in.ReadSlice('\n')
	if err != nil {
		f.Close()
		return header{}, nil, nil, fmt.Errorf("no header: %w", err)
	}
	var head header
	if err := json.Unmarshal(line, &head); err != nil {
		f.Close()
		return header{}, nil, nil, fmt.Errorf("bad header: %w", err)
	}
	if head.Version != recoveryVersion {
		f.Close()
		return header{}, nil, nil, fmt.Errorf("unknown version %d", head.Version)
	}
	return head, in, func() { f.Close() }, nil
}

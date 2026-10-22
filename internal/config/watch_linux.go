package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const configWatchDebounce = 100 * time.Millisecond

const configWatchMask = syscall.IN_MODIFY | syscall.IN_CLOSE_WRITE | syscall.IN_ATTRIB |
	syscall.IN_CREATE | syscall.IN_DELETE | syscall.IN_MOVED_FROM | syscall.IN_MOVED_TO |
	syscall.IN_DELETE_SELF | syscall.IN_MOVE_SELF

type watchedDirectory struct {
	path  string
	names map[string]struct{}
}

// Watcher reports changes to config source files, including atomic replacement
// and creation of a source that did not exist when it was first loaded.
type Watcher struct {
	fd     int
	events chan struct{}
	done   chan struct{}
	wg     sync.WaitGroup
	mu     sync.RWMutex
	byPath map[string]int
	byWD   map[int32]watchedDirectory
	closed bool
	close  sync.Once
}

func NewWatcher(paths []string) (*Watcher, error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("config watcher: inotify init: %w", err)
	}
	w := &Watcher{
		fd: fd, events: make(chan struct{}, 1), done: make(chan struct{}),
		byPath: map[string]int{}, byWD: map[int32]watchedDirectory{},
	}
	if err := w.Update(paths); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	w.wg.Add(1)
	go w.readLoop()
	return w, nil
}

func (w *Watcher) Events() <-chan struct{} { return w.events }

// Update replaces the watched sources while retaining directory watches shared
// with the previous source set.
func (w *Watcher) Update(paths []string) error {
	wanted := make(map[string]map[string]struct{})
	for _, path := range paths {
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("config watcher: resolve %q: %w", path, err)
		}
		absolute = filepath.Clean(absolute)
		dir, name := filepath.Dir(absolute), filepath.Base(absolute)
		for {
			info, err := os.Stat(dir)
			if err == nil && info.IsDir() {
				resolved, err := filepath.EvalSymlinks(dir)
				if err != nil {
					return fmt.Errorf("config watcher: resolve parent %q: %w", dir, err)
				}
				addWatchName(wanted, resolved, name)
				break
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("config watcher: stat %q: %w", dir, err)
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return fmt.Errorf("config watcher: no existing parent for %q", absolute)
			}
			name, dir = filepath.Base(dir), parent
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("config watcher: closed")
	}
	nextByPath := make(map[string]int, len(wanted))
	nextByWD := make(map[int32]watchedDirectory, len(wanted))
	var addErrors []error
	for path, names := range wanted {
		wd, err := syscall.InotifyAddWatch(w.fd, path, configWatchMask)
		if err != nil {
			addErrors = append(addErrors, fmt.Errorf("watch %q: %w", path, err))
			continue
		}
		nextByPath[path] = wd
		nextByWD[int32(wd)] = watchedDirectory{path: path, names: names}
	}
	if len(addErrors) != 0 {
		for path, wd := range nextByPath {
			if _, existed := w.byPath[path]; !existed {
				_, _ = syscall.InotifyRmWatch(w.fd, uint32(wd))
			}
		}
		return errors.Join(addErrors...)
	}
	for path, wd := range w.byPath {
		if _, keep := wanted[path]; !keep {
			_, _ = syscall.InotifyRmWatch(w.fd, uint32(wd))
		}
	}
	w.byPath, w.byWD = nextByPath, nextByWD
	return errors.Join(addErrors...)
}

func addWatchName(wanted map[string]map[string]struct{}, dir, name string) {
	names := wanted[dir]
	if names == nil {
		names = map[string]struct{}{}
		wanted[dir] = names
	}
	names[name] = struct{}{}
}

func (w *Watcher) Close() {
	w.close.Do(func() {
		w.mu.Lock()
		w.closed = true
		close(w.done)
		w.mu.Unlock()
		w.wg.Wait()
		_ = syscall.Close(w.fd)
	})
}

func (w *Watcher) readLoop() {
	defer w.wg.Done()
	buffer := make([]byte, 64*1024)
	var changedAt time.Time
	for {
		select {
		case <-w.done:
			return
		default:
		}
		timeout := 100 * time.Millisecond
		if !changedAt.IsZero() {
			remaining := time.Until(changedAt.Add(configWatchDebounce))
			if remaining <= 0 {
				w.notifyChanged()
				changedAt = time.Time{}
				continue
			}
			if remaining < timeout {
				timeout = remaining
			}
		}
		var readSet syscall.FdSet
		if w.fd >= len(readSet.Bits)*64 {
			return
		}
		readSet.Bits[w.fd/64] |= int64(1) << uint(w.fd%64)
		tv := syscall.NsecToTimeval(timeout.Nanoseconds())
		ready, err := syscall.Select(w.fd+1, &readSet, nil, nil, &tv)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return
		}
		if ready == 0 {
			continue
		}
		n, err := syscall.Read(w.fd, buffer)
		if err == syscall.EINTR || err == syscall.EAGAIN {
			continue
		}
		if err != nil || n == 0 {
			return
		}
		if w.hasRelevantEvent(buffer[:n]) {
			changedAt = time.Now()
		}
	}
}

func (w *Watcher) hasRelevantEvent(buffer []byte) bool {
	headerSize := int(syscall.SizeofInotifyEvent)
	changed := false
	for offset := 0; offset+headerSize <= len(buffer); {
		event := *(*syscall.InotifyEvent)(unsafe.Pointer(&buffer[offset]))
		nameEnd := offset + headerSize + int(event.Len)
		if nameEnd > len(buffer) {
			return false
		}
		if event.Mask&syscall.IN_Q_OVERFLOW != 0 {
			changed = true
			offset = nameEnd
			continue
		}
		name := strings.TrimRight(string(buffer[offset+headerSize:nameEnd]), "\x00")
		invalid := event.Mask & (syscall.IN_IGNORED | syscall.IN_DELETE_SELF | syscall.IN_MOVE_SELF | syscall.IN_UNMOUNT)
		if invalid != 0 {
			w.dropWatch(event.Wd)
			if invalid&(syscall.IN_DELETE_SELF|syscall.IN_MOVE_SELF|syscall.IN_UNMOUNT) != 0 {
				changed = true
			}
			offset = nameEnd
			continue
		}
		if event.Mask&configWatchMask != 0 && name != "" {
			w.mu.RLock()
			watch, exists := w.byWD[event.Wd]
			_, matches := watch.names[name]
			w.mu.RUnlock()
			if exists && matches {
				changed = true
			}
		}
		offset = nameEnd
	}
	return changed
}

func (w *Watcher) dropWatch(wd int32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	watch, exists := w.byWD[wd]
	if !exists {
		return
	}
	delete(w.byWD, wd)
	if w.byPath[watch.path] == int(wd) {
		delete(w.byPath, watch.path)
	}
}

func (w *Watcher) notifyChanged() {
	select {
	case w.events <- struct{}{}:
	default:
	}
}

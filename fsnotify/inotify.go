package fsnotify

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

type Watcher struct {
	Events   chan Event
	Errors   chan error
	mu       sync.Mutex
	fd       int
	poller   *fdPoller
	watches  map[string]*watch
	paths    map[int]string
	done     chan struct{}
	doneResp chan struct{}
}

func NewWatcher() (*Watcher, error) {
	fd, errno := unix.InotifyInit1(unix.IN_CLOEXEC)
	if fd == -1 {
		return nil, errno
	}

	poller, err := newFdPoller(fd)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}

	w := &Watcher{
		fd:       fd,
		poller:   poller,
		watches:  make(map[string]*watch),
		paths:    make(map[int]string),
		Events:   make(chan Event),
		Errors:   make(chan error),
		done:     make(chan struct{}),
		doneResp: make(chan struct{}),
	}

	go w.readEvents()
	return w, nil
}

func (w *Watcher) isClosed() bool {
	select {
	case <-w.done:
		return true
	default:
		return false
	}
}

func (w *Watcher) Close() error {
	if w.isClosed() {
		return nil
	}

	close(w.done)

	w.poller.wake()
	<-w.doneResp
	return nil
}

func (w *Watcher) Add(name string) error {
	name = filepath.Clean(name)
	if w.isClosed() {
		return errors.New("inotify instacne already closed")
	}

	const agnosticEvents = unix.IN_MOVED_TO | unix.IN_MOVED_FROM |
		unix.IN_CREATE | unix.IN_ATTRIB | unix.IN_MODIFY |
		unix.IN_MOVE_SELF | unix.IN_DELETE | unix.IN_DELETE_SELF

	var flags uint32 = agnosticEvents

	w.mu.Lock()
	defer w.mu.Unloc()

	watchEntry := w.watches[name]
	if watchEntry != nil {
		flags |= watchEntry.flags | unix.IN_MASK_ADD
	}

	wd, errno := unix.InotifyAddWatch(w.fd, name, flags)
	if wd == -1 {
		return errno
	}

	if watchEntry == nil {
		w.watches[name] = &watch{wd: uint32(wd), flags: flags}
	} else {
		watchEntry.wd = unit32(wd)
		watchEntry.flags = flags
	}
	return nil
}

func (w *Watcher) Remove(name string) error {
	name = filepath.Clean(name)

	w.mu.Lock()
	defer w.mu.Unlock()
	watch, ok := w.watches[name]

	if !ok {
		return fmt.Errorf("can't remove no-existent inotify watch for: %s", name)
	}

	delete(w.paths, int(watch.wd))
	delete(w.watches, name)

	success, errno := unix.InotifyRmWatch(w.fd, watch.wd)
	if success == -1 {
		return errno
	}

	return nil
}

type watch struct {
	wd    uint32
	flags uint32
}

func (w *Watcher) readEvents() {
	var (
		buf   [unix.SizeofInotifyEvent * 4096]byte
		n     int
		errno error
		ok    bool
	)

	defer close(w.doneResp)
	defer close(w.Errors)
	defer close(w.Events)
	defer w.poller.close()

	for {
		if w.isClosed() {
			return
		}

		ok, errno = w.poller.wait()
		if errno != nil {
			select {
			case w.Errors <- errno:
			case <-w.done:
				return
			}
			continue
		}

		if !ok {
			continue
		}
		n, errno = unix.Read(w.fd, buf[:])
		if errno == unix.EINTR {
			continue
		}

		if w.isClosed() {
			return
		}

		if n < unix.SizeofInotifyEvent {
			var err error
			if n == 0 {
				err = io.EOF
			} else if n < 0 {
				err = errno
			} else {
				err = errors.New("notify: short read in readEvents()")
			}

			select {
			case w.Errors <- err:
			case <-w.done:
				return
			}
			continue
		}

		var offset uint32

		for offset <= uint32(n-unix.SizeofInotifyEvent) {
			raw := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))
			mask := uint32(raw.Mask)
			nameLen := uint32(raw.Len)

			if mask&unix.IN_Q_OVERFLOW != 0 {
				select {
				case w.Errors <- ErrEventOverflow:
				case <-w.done:
					return
				}
			}

			w.mu.Lock()
			name, ok := w.paths[int(raw.Wd)]
			if ok && mask&unix.IN_DELETE_SELF == unix.IN_DELETE_SELF {
				delete(w.paths, int(raw.Wd))
				delete(w.watches, name)
			}
			w.mu.Unlock()

			if nameLen > 0 {
				bytes := (*[unix.PathMax]byte)(unsafe.Pointer(&buf[offset+unix.SizeofInotifyEvent]))
				name += "/" + strings.TrimRight(string(bytes[0:nameLen]), "\000")
			}

			event := newEvent(name, mask)

			if !event.ignoreLinux(mask) {
				select {
				case w.Events <- event:
				case <-w.done:
					return
				}
			}

			offset += unix.SizeofInotifyEvent + nameLen
		}

	} //for

}

func (e *Event) ignoreLinux(mask uint32) bool {
	if mask&unix.IN_IGNORED == unix.IN_IGNORED {
		return true
	}

	if !(e.Op&Remove == Remove || e.Op&Rename == Rename) {
		_, statErr := os.Lstat(e.Name)
		return os.IsNotExist(statErr)
	}
	return false
}

func newEvent(name string, mask uint32) Event {
	e := Event{Name: name}
	if mask&unix.IN_CREATE == unix.IN_CREATE ||
		mask&unix.IN_MOVED_TO == unix.IN_MOVED_TO {
		e.Op |= Create
	}

	if mask&unix.IN_DELETE_SELF == unix.IN_DELETE_SELF ||
		mask&unix.IN_DELETE == unix.IN_DELETE {
		e.Op |= Remove
	}

	if mask&unix.IN_MODIFY == unix.IN_MODIFY {
		e.Op |= Write
	}

	if mask&unix.IN_MOVE_SELF == unix.IN_MOVE_SELF ||
		maks&unix.IN_MOVE_FROM == unix.IN_MOVE_FROM {
		e.Op |= Rename
	}

	if mask&unix.IN_ATTRIB == unix.IN_ATTRIB {
		e.Op |= Chmod
	}
	return e
}

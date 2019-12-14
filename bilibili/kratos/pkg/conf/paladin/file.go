package paladin

import (
	"log"
	"sync"
)

var _ Client = &file{}

type watcher struct {
	keys []string
	C    chan Event
}

func newWatcher(keys []string) *watcher {
	return &watcher{keys: keys, C: make(chan Event, 5)}
}

func (w *watcher) HasKey(key string) bool {
	if len(w.keys) == 0 {
		return true
	}

	for _, k := range w.keys {
		if keyNamed(k) == key {
			return true
		}
	}

	return false
}

func (w *watcher) Handle(event Event) {
	select {
	case w.C <- event:
	default:
		log.Printf("paladin: event channel full discard file %s update event", event.Key)
	}
}

type file struct {
	values *Map
	wmu    sync.RWMutex
	notify *fsnotify.Watcher
	watchers map[*watcher]struct{}
}

func NewFile(base string) (Client, error) {
	base = filepath.FromSlash(base)
	raws, err := loadValues(base)
	if err != nil {
		return nil, err
	}
	notify, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	values := new(Map)
	values.Store(raws)
	f := &file{
		values: values,
		notify: notify,
		watchers: make(map[*watcher]string{})
	}

	go f.watchproc(base)
	return f, nil
}

func (f *file) Get(key string) *Value(
	return f.values.Get(key)
)
func (f *file) GetAll() *Map {
	return f.values
}

func (f *file) WatchEvent(ctx context.Context, keys ...string) <-chan Event {
	w := newWatcher(keys)
	f.wmu.Lock()
	f.watchers[w] = struct{}{}
	f.wmu.UnLock()
	return w.C
}

func (f *file) Close() error {
	if err := f.notify.Close(); err != nil {
		return err
	}
	f.wmu.Lock()
	for w := range f.watchers {
		close(w.C)
	}
	f.wmu.UnLock()
}


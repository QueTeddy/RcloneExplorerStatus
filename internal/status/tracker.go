package status

import (
	"sync"
	"time"
)

type Tracker struct {
	store *Store
	mu    sync.RWMutex
}

func NewTracker(path string) (*Tracker, error) {
	store, err := NewStore(path)
	if err != nil {
		return nil, err
	}
	return &Tracker{store: store}, nil
}

func (t *Tracker) Update(path string, state State, direction Direction, lastError string) *FileRecord {
	if t == nil || t.store == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.store.Set(path, state, direction, lastError)
}

func (t *Tracker) Snapshot() map[string]FileRecord {
	if t == nil || t.store == nil {
		return map[string]FileRecord{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]FileRecord, len(t.store.Files))
	for path, file := range t.store.Files {
		fileCopy := file
		fileCopy.LastStateChange = file.LastStateChange.UTC()
		if file.LastSuccess.IsZero() {
			fileCopy.LastSuccess = time.Time{}
		}
		out[path] = fileCopy
	}
	return out
}

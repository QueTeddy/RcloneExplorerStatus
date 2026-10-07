package status

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type State string

const (
	StateUnknown   State = "unknown"
	StatePending   State = "pending"
	StateSyncing   State = "syncing"
	StateSynced    State = "synced"
	StateError     State = "error"
	StateConflict  State = "conflict"
	StateOffline   State = "offline"
	StateCloudOnly State = "cloud_only"
)

type Direction string

const (
	DirectionNone     Direction = "none"
	DirectionUpload   Direction = "upload"
	DirectionDownload Direction = "download"
)

type FileRecord struct {
	Path            string    `json:"path"`
	State           State     `json:"state"`
	Direction       Direction `json:"direction"`
	LastStateChange time.Time `json:"last_state_change"`
	LastSuccess     time.Time `json:"last_success"`
	LastError       string    `json:"last_error,omitempty"`
	RetryCount      int       `json:"retry_count"`
	Size            int64     `json:"size,omitempty"`
}

type Store struct {
	Version int                   `json:"version"`
	Files   map[string]FileRecord `json:"files"`
	path    string
	mu      sync.RWMutex
}

func NewStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("state path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	if _, err := os.Stat(path); err == nil {
		return LoadStore(path)
	}

	store := &Store{
		Version: 1,
		Files:   map[string]FileRecord{},
		path:    path,
	}
	return store, nil
}

func LoadStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("state path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		store := &Store{Version: 1, Files: map[string]FileRecord{}, path: path}
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}

	if len(data) == 0 {
		return &Store{Version: 1, Files: map[string]FileRecord{}, path: path}, nil
	}

	store := &Store{path: path}
	if err := json.Unmarshal(data, &store); err != nil {
		corrupt := fmt.Sprintf("%s.corrupt.%d", path, time.Now().Unix())
		if renameErr := os.Rename(path, corrupt); renameErr != nil {
			return nil, fmt.Errorf("state file is corrupt and could not be renamed: %w", err)
		}
		store = &Store{Version: 1, Files: map[string]FileRecord{}, path: path}
		return store, nil
	}
	if store.Version == 0 {
		store.Version = 1
	}
	if store.Files == nil {
		store.Files = make(map[string]FileRecord)
	}
	for rawPath, file := range store.Files {
		normalized := NormalizePath(rawPath)
		if normalized != rawPath {
			delete(store.Files, rawPath)
			store.Files[normalized] = file
		}
		file.Path = normalized
		store.Files[normalized] = file
	}
	return store, nil
}

func (s *Store) Save() error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	if s.path == "" {
		return fmt.Errorf("state path is required")
	}

	payload, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o600); err != nil {
		return fmt.Errorf("write temp state file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("rename state file: %w", err)
	}
	return nil
}

func (s *Store) Get(path string) *FileRecord {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := NormalizePath(path)
	file, ok := s.Files[key]
	if !ok {
		return nil
	}
	copyRecord := file
	return &copyRecord
}

func (s *Store) Set(path string, state State, direction Direction, lastError string) *FileRecord {
	if s == nil {
		return nil
	}
	if s.Files == nil {
		s.Files = make(map[string]FileRecord)
	}
	key := NormalizePath(path)
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.Files[key]
	if !ok {
		record = FileRecord{Path: key, State: StateUnknown, Direction: DirectionNone}
	}

	record.Path = key
	record.State = state
	record.Direction = direction
	record.LastStateChange = now
	if lastError != "" {
		record.LastError = lastError
	} else {
		record.LastError = ""
	}
	if state == StateSynced {
		record.LastSuccess = now
	}
	if state == StateError && record.LastError == "" {
		record.LastError = "unknown error"
	}

	s.Files[key] = record
	stored := s.Files[key]
	return &stored
}

func NormalizePath(rawPath string) string {
	if rawPath == "" {
		return ""
	}
	path := strings.TrimSpace(rawPath)
	path = strings.ReplaceAll(path, "/", "\\")
	path = strings.TrimPrefix(path, "\\\\?\\")
	path = strings.TrimPrefix(path, "//?/")
	path = strings.ReplaceAll(path, "\\\\", "\\")
	if len(path) >= 2 && path[1] == ':' {
		path = strings.ToLower(path[:1]) + path[1:]
		if len(path) > 3 {
			path = strings.TrimRight(path, "\\")
		}
	}
	if path == "" {
		return ""
	}
	path = strings.ToLower(path)
	if len(path) > 3 && strings.HasSuffix(path, "\\") {
		path = strings.TrimRight(path, "\\")
	}
	if len(path) == 2 && path[1] == ':' {
		path += "\\"
	}
	return path
}

func IsValidTransition(from State, to State) bool {
	transitions := map[State][]State{
		StateUnknown:   {StatePending},
		StatePending:   {StateSyncing},
		StateSyncing:   {StateSynced, StateError, StateConflict},
		StateSynced:    {StateUnknown},
		StateError:     {StateSyncing, StateSynced},
		StateConflict:  {StateSyncing},
		StateOffline:   {StateSyncing, StateSynced},
		StateCloudOnly: {StateSyncing},
	}
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

package rclone

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/queteddy/rclone-explorer-status/internal/status"
)

type Change struct {
	Path      string
	State     status.State
	LastError string
}

type Monitor struct {
	Executable string
	Remote     string
	RCEnabled  bool
	RCAddress  string
	Client     *http.Client
	StartedAt  time.Time
	seen       map[string]struct{}
}

type transfer struct {
	Name      string `json:"name"`
	What      string `json:"what"`
	Error     string `json:"error"`
	Timestamp int64  `json:"timestamp"`
}

type statsResponse struct {
	Transferring []transfer `json:"transferring"`
}

type queueItem struct {
	Name      string `json:"name"`
	Uploading bool   `json:"uploading"`
	Tries     int    `json:"tries"`
}

type queueResponse struct {
	Queue []queueItem `json:"queue"`
}

type transferredResponse struct {
	Transferred []transfer `json:"transferred"`
}

func NewMonitor(executable, remote string, rcEnabled bool, rcAddress string) *Monitor {
	return &Monitor{
		Executable: executable,
		Remote:     remote,
		RCEnabled:  rcEnabled,
		RCAddress:  rcAddress,
		Client:     &http.Client{Timeout: 2 * time.Second},
		StartedAt:  time.Now(),
		seen:       make(map[string]struct{}),
	}
}

func (m *Monitor) IsRunning() bool {
	return m != nil && m.Executable != ""
}

func (m *Monitor) StatusSummary() string {
	if m == nil || !m.IsRunning() {
		return "offline"
	}
	if m.RCEnabled {
		return "rc-enabled"
	}
	return "running"
}

func (m *Monitor) CheckRC(ctx context.Context) error {
	var stats statsResponse
	if err := m.call(ctx, "core/stats", &stats); err != nil {
		return fmt.Errorf("core/stats: %w", err)
	}
	var queue queueResponse
	if err := m.call(ctx, "vfs/queue", &queue); err != nil {
		return fmt.Errorf("vfs/queue: %w", err)
	}
	var completed transferredResponse
	if err := m.call(ctx, "core/transferred", &completed); err != nil {
		return fmt.Errorf("core/transferred: %w", err)
	}
	return nil
}

func (m *Monitor) Run(ctx context.Context, mountPath string, store *status.Store, interval time.Duration, onChange func(Change), onUnavailable func(error), onRecovered func()) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	unavailable := false

	poll := func() {
		changes, err := m.Poll(ctx, mountPath, store)
		if err != nil {
			if !unavailable && onUnavailable != nil {
				onUnavailable(err)
			}
			unavailable = true
			return
		}
		if unavailable && onRecovered != nil {
			onRecovered()
		}
		unavailable = false
		for _, change := range changes {
			if onChange != nil {
				onChange(change)
			}
		}
	}

	poll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}

func (m *Monitor) Poll(ctx context.Context, mountPath string, store *status.Store) ([]Change, error) {
	if m == nil || !m.RCEnabled {
		return nil, fmt.Errorf("rclone RC monitoring is disabled")
	}
	if store == nil {
		return nil, fmt.Errorf("status store is required")
	}

	var stats statsResponse
	if err := m.call(ctx, "core/stats", &stats); err != nil {
		return nil, fmt.Errorf("rclone RC core/stats: %w", err)
	}

	updates := make(map[string]Change)

	var queue queueResponse
	if err := m.call(ctx, "vfs/queue", &queue); err != nil {
		return nil, fmt.Errorf("rclone RC vfs/queue: %w", err)
	}
	for _, item := range queue.Queue {
		if path, ok := mountFilePath(mountPath, item.Name); ok {
			state := status.StatePending
			if item.Uploading {
				state = status.StateSyncing
			}
			updates[path] = Change{Path: path, State: state}
		}
	}
	for _, item := range stats.Transferring {
		if path, ok := mountFilePath(mountPath, item.Name); ok {
			updates[path] = Change{Path: path, State: status.StateSyncing}
		}
	}

	var completed transferredResponse
	if err := m.call(ctx, "core/transferred", &completed); err != nil {
		return nil, fmt.Errorf("rclone RC core/transferred: %w", err)
	}
	for _, item := range completed.Transferred {
		key := transferKey(item)
		if _, seen := m.seen[key]; seen {
			continue
		}
		m.seen[key] = struct{}{}
		if item.Timestamp <= 0 || time.UnixMilli(item.Timestamp).Before(m.StartedAt) {
			continue
		}
		if path, ok := mountFilePath(mountPath, item.Name); ok {
			state := status.StateSynced
			if item.Error != "" {
				state = status.StateError
			}
			updates[path] = Change{Path: path, State: state, LastError: item.Error}
		}
	}

	changes := make([]Change, 0, len(updates))
	for _, change := range updates {
		previous := store.Get(change.Path)
		if previous != nil && previous.State == change.State && previous.LastError == change.LastError {
			continue
		}
		store.Set(change.Path, change.State, status.DirectionNone, change.LastError)
		changes = append(changes, change)
	}
	if len(changes) == 0 {
		return nil, nil
	}
	if err := store.Save(); err != nil {
		return nil, fmt.Errorf("save rclone status updates: %w", err)
	}
	return changes, nil
}

func (m *Monitor) call(ctx context.Context, endpoint string, output any) error {
	base := strings.TrimSpace(m.RCAddress)
	if base == "" {
		base = "127.0.0.1:5572"
	}
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("invalid RC address: %w", err)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(endpoint, "/")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("create RC request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := m.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("RC endpoint returned HTTP %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return fmt.Errorf("decode RC response: %w", err)
	}
	return nil
}

func mountFilePath(mountPath, name string) (string, bool) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "/", `\`))
	if name == "" {
		return "", false
	}
	root := filepath.Clean(mountPath)
	candidate := name
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate = filepath.Clean(candidate)
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", false
	}
	return candidate, true
}

func transferKey(item transfer) string {
	return fmt.Sprintf("%d|%s|%s|%s", item.Timestamp, item.Name, item.What, item.Error)
}

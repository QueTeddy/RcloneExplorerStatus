package rclone

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/queteddy/rclone-explorer-status/internal/status"
)

func TestPollMapsVFSAndCompletedTransfersToFileStates(t *testing.T) {
	completedAt := time.Now().UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("request method = %q, want POST", request.Method)
		}
		switch request.URL.Path {
		case "/core/stats":
			fmt.Fprint(response, `{"transferring":[{"name":"busy.txt"}]}`)
		case "/vfs/queue":
			fmt.Fprint(response, `{"queue":[{"name":"queued.txt","uploading":false},{"name":"uploading.txt","uploading":true}]}`)
		case "/core/transferred":
			fmt.Fprintf(response, `{"transferred":[{"name":"done.txt","what":"transferring","timestamp":%d,"error":""},{"name":"failed.txt","what":"transferring","timestamp":%d,"error":"S3 upload failed"}]}`, completedAt, completedAt)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	mount := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := status.NewStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	monitor := NewMonitor("rclone.exe", "s3:bucket", true, server.URL)
	monitor.StartedAt = time.Now().Add(-time.Minute)

	changes, err := monitor.Poll(context.Background(), mount, store)
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(changes) != 5 {
		t.Fatalf("len(changes) = %d, want 5", len(changes))
	}

	want := map[string]status.State{
		"busy.txt":      status.StateSyncing,
		"queued.txt":    status.StatePending,
		"uploading.txt": status.StateSyncing,
		"done.txt":      status.StateSynced,
		"failed.txt":    status.StateError,
	}
	for name, state := range want {
		record := store.Get(filepath.Join(mount, name))
		if record == nil || record.State != state {
			t.Errorf("state for %s = %v, want %s", name, record, state)
		}
	}
	failed := store.Get(filepath.Join(mount, "failed.txt"))
	if failed != nil && failed.LastError != "S3 upload failed" {
		t.Errorf("failed transfer LastError = %q, want S3 upload failed", failed.LastError)
	}
	changes, err = monitor.Poll(context.Background(), mount, store)
	if err != nil {
		t.Fatalf("second Poll() error = %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("second Poll() returned %d unchanged updates, want 0", len(changes))
	}
	if _, err := filepath.Abs(statePath); err != nil {
		t.Fatal(err)
	}
}

func TestMountFilePathRejectsOutsideMount(t *testing.T) {
	mount := t.TempDir()
	if _, ok := mountFilePath(mount, `..\outside.txt`); ok {
		t.Fatal("mountFilePath accepted path outside mount")
	}
	if path, ok := mountFilePath(mount, `nested\file.txt`); !ok || path != filepath.Join(mount, "nested", "file.txt") {
		t.Fatalf("mountFilePath(relative) = %q, %v", path, ok)
	}
}

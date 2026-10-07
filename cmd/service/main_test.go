package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/queteddy/rclone-explorer-status/internal/config"
	"github.com/queteddy/rclone-explorer-status/internal/status"
)

func TestSetFileStateWritesStateForFileInsideMount(t *testing.T) {
	mount := t.TempDir()
	file := filepath.Join(mount, "sample.txt")
	if err := os.WriteFile(file, []byte("sample"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Mount.Path = mount
	cfg.Status.StateFile = filepath.Join(t.TempDir(), "state.json")
	if err := setFileState(cfg, file, "synced"); err != nil {
		t.Fatalf("setFileState() error = %v", err)
	}
	store, err := status.LoadStore(cfg.Status.StateFile)
	if err != nil {
		t.Fatalf("LoadStore() error = %v", err)
	}
	record := store.Get(file)
	if record == nil || record.State != status.StateSynced {
		t.Fatalf("stored record = %#v, want synced", record)
	}
}

func TestSetFileStateRejectsPathOutsideMount(t *testing.T) {
	mount := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("sample"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Mount.Path = mount
	cfg.Status.StateFile = filepath.Join(t.TempDir(), "state.json")
	if err := setFileState(cfg, outside, "syncing"); err == nil {
		t.Fatal("setFileState() succeeded for a path outside the mount")
	}
}

func TestSetFileStateRejectsUnsupportedState(t *testing.T) {
	mount := t.TempDir()
	file := filepath.Join(mount, "sample.txt")
	if err := os.WriteFile(file, []byte("sample"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Mount.Path = mount
	cfg.Status.StateFile = filepath.Join(t.TempDir(), "state.json")
	if err := setFileState(cfg, file, "offline"); err == nil {
		t.Fatal("setFileState() succeeded for an unsupported overlay state")
	}
}

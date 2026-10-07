package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/queteddy/rclone-explorer-status/internal/config"
)

func TestInstallMountPersistsSelectedExistingDirectory(t *testing.T) {
	root := t.TempDir()
	mountPath := filepath.Join(root, "s3-mount")
	if err := os.Mkdir(mountPath, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "user-config", "config.json")

	if err := installMount(mountPath, configPath); err != nil {
		t.Fatalf("installMount() error = %v", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if cfg.Mount.Path != mountPath {
		t.Fatalf("mount path = %q, want %q", cfg.Mount.Path, mountPath)
	}
}

func TestInstallMountRejectsMissingDirectoryWithoutWritingConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "user-config", "config.json")
	missingMount := filepath.Join(root, "not-mounted")

	if err := installMount(missingMount, configPath); err == nil {
		t.Fatal("installMount() succeeded for missing mount directory")
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config file stat error = %v, want not-exist", err)
	}
}

func TestWriteMountConfigPersistsPreselectedPathWithoutRecheckingDrive(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "user-config", "config.json")
	selectedPath := filepath.Join("S:\\", "rclone-mount")

	if err := writeMountConfig(selectedPath, configPath); err != nil {
		t.Fatalf("writeMountConfig() error = %v", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if cfg.Mount.Path != selectedPath {
		t.Fatalf("mount path = %q, want %q", cfg.Mount.Path, selectedPath)
	}
}

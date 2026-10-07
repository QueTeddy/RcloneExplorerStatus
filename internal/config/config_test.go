package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigUsesEnvironmentFallbacks(t *testing.T) {
	t.Setenv("LOCALAPPDATA", filepath.Join("C:\\", "Users", "demo", "AppData", "Local"))
	cfg := Default()
	if cfg.Status.StateFile == "" {
		t.Fatal("default state file should not be empty")
	}
	if filepath.Dir(cfg.Status.StateFile) == "" {
		t.Fatal("state file should include a directory")
	}
}

func TestLoadExpandsWindowsPathVariables(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	statePath := filepath.Join(dir, "state.json")
	t.Setenv("LOCALAPPDATA", dir)

	cfg := Default()
	cfg.Status.StateFile = filepath.Join("%LOCALAPPDATA%", "RcloneExplorerStatus", "state.json")
	if err := Write(configPath, cfg); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Status.StateFile != filepath.Join(dir, "RcloneExplorerStatus", "state.json") {
		t.Fatalf("loaded state file = %q, want %q", loaded.Status.StateFile, filepath.Join(dir, "RcloneExplorerStatus", "state.json"))
	}
	_ = statePath
}

func TestValidateMountPathRequiresExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ValidateMountPath(dir); err != nil {
		t.Fatalf("ValidateMountPath(existing directory) error = %v", err)
	}
	if err := ValidateMountPath(filePath); err == nil {
		t.Fatal("ValidateMountPath(file) succeeded, want error")
	}
	if err := ValidateMountPath(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("ValidateMountPath(missing path) succeeded, want error")
	}
}

func TestConfigWritePersistsSelectedMountPath(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	mountPath := filepath.Join(t.TempDir(), "s3-mount")
	if err := os.Mkdir(mountPath, 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := Default()
	if err := ValidateMountPath(mountPath); err != nil {
		t.Fatalf("ValidateMountPath() error = %v", err)
	}
	cfg.Mount.Path = mountPath
	if err := Write(configPath, cfg); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Mount.Path != mountPath {
		t.Fatalf("loaded mount path = %q, want %q", loaded.Mount.Path, mountPath)
	}
}

func TestLoadEmptyPathUsesPerUserConfigLocation(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	configPath := filepath.Join(appData, "RcloneExplorerStatus", "config.json")

	if _, err := Load(""); err != nil {
		t.Fatalf("Load(empty path) error = %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config at %q: %v", configPath, err)
	}
}

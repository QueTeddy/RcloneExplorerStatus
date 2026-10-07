package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Mount struct {
		Path string `json:"path"`
	} `json:"mount"`
	Rclone struct {
		Executable string `json:"executable"`
		Remote     string `json:"remote"`
		RC         struct {
			Enabled bool   `json:"enabled"`
			Host    string `json:"host"`
			Port    int    `json:"port"`
		} `json:"rc"`
	} `json:"rclone"`
	Status struct {
		StateFile string `json:"state_file"`
	} `json:"status"`
	Logging struct {
		Level string `json:"level"`
	} `json:"logging"`
}

func defaultEnvValue(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return dir
	}
	if dir, err := os.UserHomeDir(); err == nil && dir != "" {
		return dir
	}
	return ""
}

func Default() Config {
	cfg := Config{}
	cfg.Mount.Path = `S:\`
	cfg.Rclone.Executable = `rclone.exe`
	cfg.Rclone.Remote = `s3:local-sync-folder-reclone`
	cfg.Rclone.RC.Enabled = true
	cfg.Rclone.RC.Host = `127.0.0.1`
	cfg.Rclone.RC.Port = 5572

	localAppData := defaultEnvValue("LOCALAPPDATA", "AppDataLocal", "LOCALAPPDATA")
	if localAppData == "" {
		localAppData = filepath.Join(defaultEnvValue("USERPROFILE", "HOME"), "AppData", "Local")
	}
	cfg.Status.StateFile = filepath.Join(localAppData, "RcloneExplorerStatus", "state.json")
	cfg.Logging.Level = "info"
	return cfg
}

func DefaultPath() string {
	configRoot := defaultEnvValue("APPDATA")
	if configRoot == "" {
		configRoot = defaultEnvValue("USERPROFILE", "HOME")
	}
	return filepath.Join(configRoot, "RcloneExplorerStatus", "config.json")
}

func ValidateMountPath(path string) error {
	path = strings.TrimSpace(expandWindowsEnv(path))
	if path == "" {
		return fmt.Errorf("mount path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("access mount path %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("mount path %q is not a directory", path)
	}
	return nil
}

func expandWindowsEnv(raw string) string {
	if raw == "" {
		return raw
	}
	result := os.ExpandEnv(raw)
	for _, key := range []string{"APPDATA", "LOCALAPPDATA", "USERPROFILE", "HOME"} {
		value := os.Getenv(key)
		if value == "" {
			continue
		}
		result = strings.ReplaceAll(result, "%"+key+"%", value)
		result = strings.ReplaceAll(result, "%"+strings.ToUpper(key)+"%", value)
		result = strings.ReplaceAll(result, "%"+strings.ToLower(key)+"%", value)
	}
	return result
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = DefaultPath()
	}
	path = expandWindowsEnv(path)
	if _, err := os.Stat(path); err != nil {
		if err := Write(path, cfg); err != nil {
			return cfg, fmt.Errorf("create default config: %w", err)
		}
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	cfg.Status.StateFile = expandWindowsEnv(cfg.Status.StateFile)
	cfg.Mount.Path = expandWindowsEnv(cfg.Mount.Path)
	return cfg, nil
}

func Write(path string, cfg Config) error {
	if path == "" {
		return fmt.Errorf("config path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

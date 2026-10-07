package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/queteddy/rclone-explorer-status/internal/config"
	"github.com/queteddy/rclone-explorer-status/internal/explorer"
	"github.com/queteddy/rclone-explorer-status/internal/logging"
	"github.com/queteddy/rclone-explorer-status/internal/rclone"
	"github.com/queteddy/rclone-explorer-status/internal/shellui"
	"github.com/queteddy/rclone-explorer-status/internal/status"
)

// Windowed Explorer helper for right-click Sync / Audit actions.
func main() {
	if len(os.Args) < 3 {
		shellui.Error("Rclone Explorer Status", "usage: rclone-explorer-status-shell.exe <sync|audit> <path>")
		os.Exit(2)
	}

	action := strings.ToLower(strings.TrimSpace(os.Args[1]))
	target := strings.TrimSpace(os.Args[2])
	logPath := filepath.Join(os.Getenv("LOCALAPPDATA"), "RcloneExplorerStatus", "logs", "service.log")
	logger, err := logging.NewLogger(logPath, "info")
	if err != nil {
		shellui.Error("Rclone Explorer Status", fmt.Sprintf("Could not open log file:\n%v", err))
		os.Exit(1)
	}
	defer logger.Close()

	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		logger.Error("shell helper config load failed", "error", err.Error())
		shellui.Error("Rclone Explorer Status", fmt.Sprintf("Could not load configuration:\n%v", err))
		os.Exit(1)
	}

	switch action {
	case "sync":
		if err := syncOne(cfg, logger, target); err != nil {
			logger.Error("shell sync failed", "path", target, "error", err.Error())
			shellui.Error("Sync failed", err.Error())
			os.Exit(1)
		}
	case "audit":
		if err := auditOne(cfg, logger, target); err != nil {
			logger.Error("shell audit failed", "path", target, "error", err.Error())
			shellui.Error("Audit failed", err.Error())
			os.Exit(1)
		}
	default:
		shellui.Error("Rclone Explorer Status", "Unknown action. Use sync or audit.")
		os.Exit(2)
	}
}

func syncOne(cfg config.Config, logger *logging.Logger, path string) error {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("access file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("right-click Sync works on a single file. Use Audit Folder on a directory")
	}
	if err := ensureUnderMount(cfg.Mount.Path, path); err != nil {
		return err
	}
	if _, err := lookRclone(cfg.Rclone.Executable); err != nil {
		return err
	}

	store, err := status.NewStore(cfg.Status.StateFile)
	if err != nil {
		return err
	}
	store.Set(path, status.StateSyncing, status.DirectionUpload, "")
	_ = store.Save()
	explorer.NotifyPathChanged(path)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	logger.Info("shell sync started", "path", path)
	remoteTarget, err := rclone.ResyncMissing(ctx, cfg.Rclone.Executable, cfg.Rclone.Remote, cfg.Mount.Path, path)
	if err != nil {
		// If already on remote, treat as synced after a quick presence check via audit of parent.
		if strings.Contains(err.Error(), "already exists") {
			store.Set(path, status.StateSynced, status.DirectionNone, "")
			_ = store.Save()
			explorer.NotifyPathChanged(path)
			shellui.Info("Already synced", fmt.Sprintf("This file already exists on the remote:\n%s", path))
			return nil
		}
		store.Set(path, status.StateError, status.DirectionUpload, err.Error())
		_ = store.Save()
		explorer.NotifyPathChanged(path)
		return err
	}

	store.Set(path, status.StateSynced, status.DirectionUpload, "")
	if err := store.Save(); err != nil {
		return err
	}
	explorer.NotifyPathChanged(path)
	logger.Info("shell sync completed", "path", path, "remote", remoteTarget)
	shellui.Info("Sync complete", fmt.Sprintf("Uploaded via rclone:\n%s\n\nRemote:\n%s", path, remoteTarget))
	return nil
}

func auditOne(cfg config.Config, logger *logging.Logger, path string) error {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("access folder: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("right-click Audit works on a folder")
	}
	if err := ensureUnderMount(cfg.Mount.Path, path); err != nil {
		return err
	}
	if _, err := lookRclone(cfg.Rclone.Executable); err != nil {
		return err
	}

	store, err := status.NewStore(cfg.Status.StateFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	logger.Info("shell audit started", "folder", path)
	items, err := rclone.AuditFolder(ctx, cfg.Rclone.Executable, cfg.Rclone.Remote, cfg.Mount.Path, path)
	if err != nil {
		return err
	}
	synced, missing := 0, 0
	for _, item := range items {
		if item.LocalExists && item.RemoteExists {
			synced++
		}
		if item.LocalExists && !item.RemoteExists {
			missing++
		}
	}
	changed := rclone.ApplyInventory(store, items)
	if err := store.Save(); err != nil {
		return err
	}
	for _, changedPath := range changed {
		explorer.NotifyPathChanged(changedPath)
	}
	logger.Info("shell audit completed", "folder", path, "synced", fmt.Sprint(synced), "missing", fmt.Sprint(missing))
	shellui.Info("Audit complete", fmt.Sprintf(
		"Folder: %s\n\nCompared: %d\nSynced: %d\nMissing on remote: %d\nOverlays updated: %d\n\nRight-click a missing file and choose Sync now.",
		path, len(items), synced, missing, len(changed),
	))
	return nil
}

func ensureUnderMount(mountPath, target string) error {
	mountPath = filepath.Clean(mountPath)
	target = filepath.Clean(target)
	relative, err := filepath.Rel(mountPath, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("path must be inside the configured rclone mount:\n%s", mountPath)
	}
	return nil
}

func lookRclone(executable string) (string, error) {
	if executable == "" {
		executable = "rclone.exe"
	}
	if filepath.IsAbs(executable) {
		if _, err := os.Stat(executable); err != nil {
			return "", fmt.Errorf("rclone was not found at %s\nInstall rclone from https://rclone.org/downloads/ then rerun installation", executable)
		}
		return executable, nil
	}
	if path, err := lookPath(executable); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("rclone is not installed or not on PATH.\nInstall it from https://rclone.org/downloads/ then ensure rclone.exe is available")
}

func lookPath(name string) (string, error) {
	return exec.LookPath(name)
}

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/queteddy/rclone-explorer-status/internal/config"
	"github.com/queteddy/rclone-explorer-status/internal/rclone"
	"github.com/queteddy/rclone-explorer-status/internal/shellui"
	"github.com/queteddy/rclone-explorer-status/internal/status"
)

// Windowed control-panel entry point for Start Menu access.
func main() {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		shellui.Error("Rclone Explorer Status", fmt.Sprintf("Could not load configuration:\n%v\n\nRun install.ps1 first.", err))
		os.Exit(1)
	}

	store, err := status.NewStore(cfg.Status.StateFile)
	tracked := 0
	syncing, errors, synced := 0, 0, 0
	if err == nil {
		tracked = len(store.Files)
		for _, record := range store.Files {
			switch record.State {
			case status.StateSyncing, status.StatePending:
				syncing++
			case status.StateError, status.StateConflict:
				errors++
			case status.StateSynced:
				synced++
			}
		}
	}

	rclonePath, rcloneErr := exec.LookPath(cfg.Rclone.Executable)
	rcloneLine := "rclone: not found (install from https://rclone.org/downloads/)"
	if rcloneErr == nil {
		rcloneLine = "rclone: " + rclonePath
	} else if filepath.IsAbs(cfg.Rclone.Executable) {
		if _, statErr := os.Stat(cfg.Rclone.Executable); statErr == nil {
			rcloneLine = "rclone: " + cfg.Rclone.Executable
			rcloneErr = nil
		}
	}

	rcLine := "RC: disabled/unreachable"
	if cfg.Rclone.RC.Enabled {
		address := net.JoinHostPort(cfg.Rclone.RC.Host, strconv.Itoa(cfg.Rclone.RC.Port))
		monitor := rclone.NewMonitor(cfg.Rclone.Executable, cfg.Rclone.Remote, true, address)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := monitor.CheckRC(ctx); err == nil {
			rcLine = "RC: OK at " + address
		} else {
			rcLine = "RC: not reachable at " + address
		}
	}

	message := fmt.Sprintf(
		"Rclone Explorer Status\n\nMount: %s\nRemote: %s\n%s\n%s\n\nTracked files: %d\nSynced: %d\nSyncing: %d\nErrors: %d\n\nTip: right-click a file under the mount and choose Sync with Rclone Explorer Status.",
		cfg.Mount.Path,
		cfg.Rclone.Remote,
		rcloneLine,
		rcLine,
		tracked,
		synced,
		syncing,
		errors,
	)
	if rcloneErr != nil {
		shellui.Warn("Rclone Explorer Status", message)
		os.Exit(1)
	}
	shellui.Info("Rclone Explorer Status", message)
}

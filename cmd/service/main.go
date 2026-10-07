package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/queteddy/rclone-explorer-status/internal/config"
	"github.com/queteddy/rclone-explorer-status/internal/explorer"
	"github.com/queteddy/rclone-explorer-status/internal/logging"
	"github.com/queteddy/rclone-explorer-status/internal/rclone"
	"github.com/queteddy/rclone-explorer-status/internal/status"
)

func main() {
	logPath := filepath.Join(os.Getenv("LOCALAPPDATA"), "RcloneExplorerStatus", "logs", "service.log")
	logger, err := logging.NewLogger(logPath, "info")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	cfgPath := config.DefaultPath()
	logger.Info("service starting", "config_path", cfgPath)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logger.Error("load config failed", "error", err.Error())
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	logger.Info("config loaded", "mount_path", cfg.Mount.Path, "state_file", cfg.Status.StateFile)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "status":
			logger.Info("status command requested")
			printStatus(cfg, logger)
			return
		case "doctor":
			logger.Info("doctor command requested")
			printDoctor(cfg, logger)
			return
		case "logs":
			if len(os.Args) > 3 || (len(os.Args) == 3 && os.Args[2] != "--follow") {
				printUsage()
				os.Exit(2)
			}
			logger.Info("log stream requested", "follow", fmt.Sprint(len(os.Args) == 3))
			if len(os.Args) == 3 {
				ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
				defer stop()
				if err := logging.Follow(ctx, logPath, os.Stdout, 50, 300*time.Millisecond); err != nil {
					logger.Error("live log stream failed", "error", err.Error())
					os.Exit(1)
				}
			} else {
				ctx, stop := context.WithCancel(context.Background())
				stop()
				if err := logging.Follow(ctx, logPath, os.Stdout, 50, 300*time.Millisecond); err != nil {
					logger.Error("log read failed", "error", err.Error())
					os.Exit(1)
				}
			}
			return
		case "set":
			if len(os.Args) != 4 {
				printUsage()
				os.Exit(2)
			}
			logger.Info("file state change started", "path", os.Args[2], "state", os.Args[3])
			if err := setFileState(cfg, os.Args[2], os.Args[3]); err != nil {
				logger.Error("set file state failed", "path", os.Args[2], "state", os.Args[3], "error", err.Error())
				fmt.Fprintf(os.Stderr, "set file state: %v\n", err)
				os.Exit(1)
			}
			logger.Info("file state changed and Explorer notified", "path", os.Args[2], "state", os.Args[3])
			fmt.Printf("Updated %s to %s and notified Explorer.\n", os.Args[2], os.Args[3])
			return
		case "audit":
			folder := ""
			if len(os.Args) > 3 {
				printUsage()
				os.Exit(2)
			}
			if len(os.Args) == 3 {
				folder = os.Args[2]
			}
			logger.Info("inventory audit started", "folder", folder)
			if err := runAudit(cfg, logger, folder); err != nil {
				logger.Error("inventory audit failed", "folder", folder, "error", err.Error())
				fmt.Fprintf(os.Stderr, "audit failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "resync":
			target := ""
			if len(os.Args) > 3 {
				printUsage()
				os.Exit(2)
			}
			if len(os.Args) == 3 {
				target = os.Args[2]
			}
			logger.Info("resync missing started", "target", target)
			if err := runResync(cfg, logger, target); err != nil {
				logger.Error("resync missing failed", "target", target, "error", err.Error())
				fmt.Fprintf(os.Stderr, "resync failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "remaining", "left":
			folder, listItems, parseErr := parseRemainingArgs(os.Args[2:])
			if parseErr != nil {
				fmt.Fprintf(os.Stderr, "%v\n", parseErr)
				printUsage()
				os.Exit(2)
			}
			logger.Info("remaining inventory requested", "folder", folder, "list_items", fmt.Sprint(listItems))
			if err := runRemaining(cfg, logger, folder, listItems); err != nil {
				logger.Error("remaining inventory failed", "folder", folder, "error", err.Error())
				fmt.Fprintf(os.Stderr, "remaining failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "install":
			logger.Info("install command requested")
			fmt.Println("Run install.ps1 to build the app and select the local rclone mount folder.")
			return
		default:
			logger.Warn("unknown command received", "command", os.Args[1])
			printUsage()
			return
		}
	}

	store, err := status.NewStore(cfg.Status.StateFile)
	if err != nil {
		logger.Error("create state store failed", "error", err.Error())
		fmt.Fprintf(os.Stderr, "create state store: %v\n", err)
		os.Exit(1)
	}

	logger.Info("service monitor starting", "mount_path", cfg.Mount.Path, "state_file", cfg.Status.StateFile)
	fmt.Printf("Rclone Explorer Status monitor running\n")
	fmt.Printf("Mount path: %s\n", cfg.Mount.Path)
	fmt.Printf("State file: %s\n", cfg.Status.StateFile)

	address := net.JoinHostPort(cfg.Rclone.RC.Host, strconv.Itoa(cfg.Rclone.RC.Port))
	monitor := rclone.NewMonitor(cfg.Rclone.Executable, cfg.Rclone.Remote, cfg.Rclone.RC.Enabled, address)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	monitor.Run(ctx, cfg.Mount.Path, store, 2*time.Second,
		func(change rclone.Change) {
			logger.Info("rclone file status changed", "path", change.Path, "state", string(change.State), "error", change.LastError)
			explorer.NotifyPathChanged(change.Path)
		},
		func(err error) {
			logger.Warn("rclone RC unavailable; automatic file status updates paused", "address", address, "error", err.Error())
		},
		func() {
			logger.Info("rclone RC reconnected; automatic file status updates resumed", "address", address)
		},
	)
	logger.Info("service monitor stopped")
}

func printStatus(cfg config.Config, logger *logging.Logger) {
	store, err := status.NewStore(cfg.Status.StateFile)
	if err != nil {
		logger.Error("status store unavailable", "error", err.Error())
		fmt.Printf("status unavailable: %v\n", err)
		return
	}
	logger.Info("status reported", "mount_path", cfg.Mount.Path, "tracked_files", fmt.Sprint(len(store.Files)))
	fmt.Printf("Service: Running\n")
	fmt.Printf("Mount: %s\n", cfg.Mount.Path)
	fmt.Printf("Remote: %s\n", cfg.Rclone.Remote)
	fmt.Printf("Files tracked: %d\n", len(store.Files))
}

func printDoctor(cfg config.Config, logger *logging.Logger) {
	logger.Info("doctor report generated", "mount_path", cfg.Mount.Path, "state_file", cfg.Status.StateFile)
	address := net.JoinHostPort(cfg.Rclone.RC.Host, strconv.Itoa(cfg.Rclone.RC.Port))
	monitor := rclone.NewMonitor(cfg.Rclone.Executable, cfg.Rclone.Remote, cfg.Rclone.RC.Enabled, address)
	fmt.Println("[OK] Windows target requirements configured")
	fmt.Printf("[OK] Mount path: %s\n", cfg.Mount.Path)
	fmt.Printf("[OK] State store: %s\n", cfg.Status.StateFile)
	fmt.Println("[INFO] Native Explorer COM overlay handler: synced, syncing, and error states supported")

	rclonePath, rcloneErr := exec.LookPath(cfg.Rclone.Executable)
	if rcloneErr != nil && filepath.IsAbs(cfg.Rclone.Executable) {
		if _, statErr := os.Stat(cfg.Rclone.Executable); statErr == nil {
			rclonePath = cfg.Rclone.Executable
			rcloneErr = nil
		}
	}
	if rcloneErr != nil {
		logger.Warn("rclone executable missing", "configured", cfg.Rclone.Executable, "error", rcloneErr.Error())
		fmt.Printf("[FAIL] rclone not found (%s). Install from https://rclone.org/downloads/\n", cfg.Rclone.Executable)
	} else {
		fmt.Printf("[OK] rclone executable: %s\n", rclonePath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := monitor.CheckRC(ctx); err != nil {
		logger.Warn("rclone RC health check failed", "address", address, "error", err.Error())
		fmt.Printf("[WARN] rclone RC is not reachable at %s; automatic status updates need rclone started with --rc --rc-addr %s\n", address, address)
	} else {
		logger.Info("rclone RC health check passed", "address", address)
		fmt.Printf("[OK] rclone RC reachable at %s\n", address)
	}
}

func printUsage() {
	fmt.Println("usage: rclone-explorer-status.exe [status|doctor|logs [--follow]|install|set <file> <synced|syncing|error>|audit [folder]|resync [folder|file]|remaining [folder] [--items]]")
}

func parseRemainingArgs(args []string) (folder string, listItems bool, err error) {
	for _, arg := range args {
		switch strings.ToLower(arg) {
		case "--items", "-items", "--list", "-l":
			listItems = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, fmt.Errorf("unknown flag %q", arg)
			}
			if folder != "" {
				return "", false, fmt.Errorf("only one folder path is allowed")
			}
			folder = arg
		}
	}
	return folder, listItems, nil
}

func runRemaining(cfg config.Config, logger *logging.Logger, folder string, listItems bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	logger.Info("listing remaining folders/items via rclone", "remote", cfg.Rclone.Remote, "folder", folder)
	items, err := rclone.AuditFolder(ctx, cfg.Rclone.Executable, cfg.Rclone.Remote, cfg.Mount.Path, folder)
	if err != nil {
		return err
	}

	folderCounts := map[string]int{}
	var missing []rclone.InventoryItem
	for _, item := range items {
		if !(item.LocalExists && !item.RemoteExists) {
			continue
		}
		missing = append(missing, item)
		dir := filepath.Dir(item.RelativePath)
		if dir == "." || dir == "" {
			dir = "."
		}
		folderCounts[dir]++
	}

	dirs := make([]string, 0, len(folderCounts))
	for dir := range folderCounts {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	fmt.Printf("Folders left: %d\n", len(dirs))
	fmt.Printf("Items left: %d\n", len(missing))
	if len(dirs) == 0 {
		fmt.Println("Nothing left to sync under this path.")
		return nil
	}

	fmt.Println()
	fmt.Println("Folders:")
	for _, dir := range dirs {
		display := dir
		if display == "." {
			display = "(mount root)"
		}
		fmt.Printf("  %4d\t%s\n", folderCounts[dir], display)
	}

	if listItems {
		fmt.Println()
		fmt.Println("Items:")
		for _, item := range missing {
			fmt.Printf("  %s\n", item.LocalPath)
		}
	} else {
		fmt.Println()
		fmt.Println("Tip: add --items to list every remaining file.")
	}
	logger.Info("remaining inventory completed", "folders", fmt.Sprint(len(dirs)), "items", fmt.Sprint(len(missing)))
	return nil
}

func runAudit(cfg config.Config, logger *logging.Logger, folder string) error {
	store, err := status.NewStore(cfg.Status.StateFile)
	if err != nil {
		return fmt.Errorf("open status store: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	logger.Info("listing local and remote inventory via rclone", "remote", cfg.Rclone.Remote, "folder", folder)
	items, err := rclone.AuditFolder(ctx, cfg.Rclone.Executable, cfg.Rclone.Remote, cfg.Mount.Path, folder)
	if err != nil {
		return err
	}

	synced, missingRemote, cloudOnly := 0, 0, 0
	for _, item := range items {
		switch {
		case item.LocalExists && item.RemoteExists:
			synced++
		case item.LocalExists && !item.RemoteExists:
			missingRemote++
			fmt.Printf("MISSING_ON_REMOTE\t%s\n", item.LocalPath)
		case !item.LocalExists && item.RemoteExists:
			cloudOnly++
			fmt.Printf("REMOTE_ONLY\t%s\n", item.RelativePath)
		}
	}

	changed := rclone.ApplyInventory(store, items)
	if err := store.Save(); err != nil {
		return fmt.Errorf("save status store: %w", err)
	}
	for _, path := range changed {
		explorer.NotifyPathChanged(path)
	}

	logger.Info("inventory audit completed",
		"total", fmt.Sprint(len(items)),
		"synced", fmt.Sprint(synced),
		"missing_on_remote", fmt.Sprint(missingRemote),
		"remote_only", fmt.Sprint(cloudOnly),
		"overlays_updated", fmt.Sprint(len(changed)),
	)
	fmt.Printf("\nAudit complete via rclone (no direct S3 API)\n")
	fmt.Printf("Remote: %s\n", cfg.Rclone.Remote)
	fmt.Printf("Folder: %s\n", auditFolderLabel(cfg.Mount.Path, folder))
	fmt.Printf("Total compared: %d\n", len(items))
	fmt.Printf("Synced (local+remote): %d\n", synced)
	fmt.Printf("Missing on remote (need resync): %d\n", missingRemote)
	fmt.Printf("Remote only: %d\n", cloudOnly)
	fmt.Printf("Explorer overlays updated: %d\n", len(changed))
	if missingRemote > 0 {
		fmt.Printf("\nTo upload missing files:\n  rclone-explorer-status.exe resync %s\n", quoteIfNeeded(folder))
	}
	return nil
}

func runResync(cfg config.Config, logger *logging.Logger, target string) error {
	store, err := status.NewStore(cfg.Status.StateFile)
	if err != nil {
		return fmt.Errorf("open status store: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	target = strings.TrimSpace(target)
	if target != "" {
		if info, statErr := os.Stat(target); statErr == nil && info.Mode().IsRegular() {
			logger.Info("resync single missing file", "path", target)
			remoteTarget, resyncErr := rclone.ResyncMissing(ctx, cfg.Rclone.Executable, cfg.Rclone.Remote, cfg.Mount.Path, target)
			if resyncErr != nil {
				return resyncErr
			}
			store.Set(target, status.StateSynced, status.DirectionUpload, "")
			if err := store.Save(); err != nil {
				return fmt.Errorf("save status store: %w", err)
			}
			explorer.NotifyPathChanged(target)
			fmt.Printf("Uploaded missing object via rclone:\n  %s\n", remoteTarget)
			return nil
		}
	}

	logger.Info("resync missing files under folder", "folder", target)
	uploaded, failed, err := rclone.ResyncMissingUnder(ctx, cfg.Rclone.Executable, cfg.Rclone.Remote, cfg.Mount.Path, target)
	if err != nil {
		return err
	}

	// Re-audit so overlays match the remote after uploads.
	items, auditErr := rclone.AuditFolder(ctx, cfg.Rclone.Executable, cfg.Rclone.Remote, cfg.Mount.Path, target)
	if auditErr != nil {
		logger.Warn("post-resync audit failed", "error", auditErr.Error())
	} else {
		changed := rclone.ApplyInventory(store, items)
		if err := store.Save(); err != nil {
			return fmt.Errorf("save status store: %w", err)
		}
		for _, path := range changed {
			explorer.NotifyPathChanged(path)
		}
	}

	for _, remoteTarget := range uploaded {
		fmt.Printf("UPLOADED\t%s\n", remoteTarget)
	}
	for path, message := range failed {
		fmt.Printf("FAILED\t%s\t%s\n", path, message)
		logger.Error("resync file failed", "path", path, "error", message)
	}
	fmt.Printf("\nResync complete via rclone\n")
	fmt.Printf("Uploaded: %d\n", len(uploaded))
	fmt.Printf("Failed: %d\n", len(failed))
	if len(failed) > 0 {
		return fmt.Errorf("%d file(s) failed to resync", len(failed))
	}
	return nil
}

func auditFolderLabel(mountPath, folder string) string {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return mountPath
	}
	return folder
}

func quoteIfNeeded(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, " \t") {
		return `"` + value + `"`
	}
	return value
}

func setFileState(cfg config.Config, path, stateName string) error {
	path = filepath.Clean(path)
	mountPath := filepath.Clean(cfg.Mount.Path)
	relative, err := filepath.Rel(mountPath, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("file path must be inside the configured rclone mount %q", cfg.Mount.Path)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("access mounted file %q: %w", path, err)
	}

	var state status.State
	switch strings.ToLower(stateName) {
	case string(status.StateSynced):
		state = status.StateSynced
	case string(status.StateSyncing):
		state = status.StateSyncing
	case string(status.StateError):
		state = status.StateError
	default:
		return fmt.Errorf("unsupported overlay state %q (choose synced, syncing, or error)", stateName)
	}

	store, err := status.NewStore(cfg.Status.StateFile)
	if err != nil {
		return fmt.Errorf("open status store: %w", err)
	}
	store.Set(path, state, status.DirectionNone, "")
	if err := store.Save(); err != nil {
		return fmt.Errorf("save status store: %w", err)
	}
	explorer.NotifyPathChanged(path)
	return nil
}

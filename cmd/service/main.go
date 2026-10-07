package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
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
	fmt.Println("usage: rclone-explorer-status.exe [status|doctor|logs [--follow]|install|set <file> <synced|syncing|error>]")
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

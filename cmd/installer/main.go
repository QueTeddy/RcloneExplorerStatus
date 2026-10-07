package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/queteddy/rclone-explorer-status/internal/config"
	"github.com/queteddy/rclone-explorer-status/internal/logging"
)

const folderPickerScript = `Add-Type -AssemblyName System.Windows.Forms
$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
$dialog.Description = 'Select the local folder or drive where rclone has mounted your S3 remote.'
$dialog.ShowNewFolderButton = $false
if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
  [Console]::Out.Write($dialog.SelectedPath)
}`

func main() {
	selectedMountPath := flag.String("mount-path", "", "previously selected rclone mount path")
	flag.Parse()

	logger, err := logging.NewLogger(logging.DefaultLogPath(), "debug")
	if err != nil {
		showMessage("Installation failed", err.Error(), true)
		fmt.Fprintf(os.Stderr, "create installer logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()
	logger.Info("GUI installer started", "log_path", logging.DefaultLogPath())

	mountPath := strings.TrimSpace(*selectedMountPath)
	if mountPath == "" {
		mountPath, err = pickMountFolder()
		if err != nil {
			logger.Error("mount folder selection failed", "error", err.Error())
			showMessage("Installation cancelled", err.Error(), true)
			fmt.Fprintf(os.Stderr, "installation cancelled or folder selection failed: %v\n", err)
			os.Exit(1)
		}
	}
	mountPath = filepath.Clean(strings.TrimSpace(mountPath))
	configPath := config.DefaultPath()
	logger.Info("mount folder selected", "mount_path", mountPath, "config_path", configPath)
	var installErr error
	if *selectedMountPath == "" {
		installErr = installMount(mountPath, configPath)
	} else {
		installErr = writeMountConfig(mountPath, configPath)
	}
	if installErr != nil {
		logger.Error("mount configuration failed", "mount_path", mountPath, "config_path", configPath, "error", installErr.Error())
		showMessage("Installation failed", installErr.Error(), true)
		fmt.Fprintf(os.Stderr, "configure mount folder: %v\n", installErr)
		os.Exit(1)
	}

	message := fmt.Sprintf("Rclone Explorer Status is configured.\r\n\r\nMounted folder: %s\r\nConfiguration: %s\r\n\r\nThe selected folder is the rclone mount, not the application installation folder.", mountPath, configPath)
	logger.Info("configuration complete", "mount_path", mountPath, "config_path", configPath)
	showMessage("Configuration complete", message, false)
	fmt.Printf("Configured mount folder: %s\nConfiguration: %s\n", mountPath, configPath)
}

func pickMountFolder() (string, error) {
	command := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", folderPickerScript)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("open Windows folder picker: %w", err)
	}
	selected := strings.TrimSpace(string(output))
	if selected == "" {
		return "", fmt.Errorf("no folder was selected")
	}
	return selected, nil
}

func installMount(mountPath, configPath string) error {
	if err := config.ValidateMountPath(mountPath); err != nil {
		return err
	}
	return writeMountConfig(mountPath, configPath)
}

func writeMountConfig(mountPath, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	cfg.Mount.Path = mountPath
	if err := config.Write(configPath, cfg); err != nil {
		return fmt.Errorf("write configuration: %w", err)
	}
	return nil
}

func showMessage(title, message string, isError bool) {
	icon := "Information"
	if isError {
		icon = "Error"
	}
	script := fmt.Sprintf("Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.MessageBox]::Show('%s', '%s', 'OK', '%s') | Out-Null", escapePowerShellSingleQuote(message), escapePowerShellSingleQuote(title), icon)
	_ = exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script).Run()
}

func escapePowerShellSingleQuote(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

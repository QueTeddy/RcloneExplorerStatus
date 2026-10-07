package rclone

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/queteddy/rclone-explorer-status/internal/status"
)

type InventoryItem struct {
	RelativePath string
	LocalPath    string
	LocalExists  bool
	RemoteExists bool
	State        status.State
}

type listedObject struct {
	Path  string `json:"Path"`
	IsDir bool   `json:"IsDir"`
}

type CommandRunner func(context.Context, string, ...string) ([]byte, error)

func ListRemoteFiles(ctx context.Context, executable, remoteRoot string) ([]string, error) {
	return listRemoteFiles(ctx, execCommand, executable, remoteRoot)
}

func listRemoteFiles(ctx context.Context, run CommandRunner, executable, remoteRoot string) ([]string, error) {
	if strings.TrimSpace(executable) == "" {
		return nil, fmt.Errorf("rclone executable is required")
	}
	if strings.TrimSpace(remoteRoot) == "" {
		return nil, fmt.Errorf("rclone remote root is required")
	}
	output, err := run(ctx, executable, "lsjson", "--recursive", "--files-only", remoteRoot)
	if err != nil {
		return nil, fmt.Errorf("list remote objects with rclone: %w: %s", err, strings.TrimSpace(string(output)))
	}
	var objects []listedObject
	if err := json.Unmarshal(output, &objects); err != nil {
		return nil, fmt.Errorf("decode rclone lsjson output: %w", err)
	}
	files := make([]string, 0, len(objects))
	for _, object := range objects {
		if object.IsDir || object.Path == "" {
			continue
		}
		files = append(files, normalizeRelative(object.Path))
	}
	sort.Strings(files)
	return files, nil
}

func execCommand(ctx context.Context, executable string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, args...)
	output, err := command.CombinedOutput()
	return output, err
}

func AuditFolder(ctx context.Context, executable, remoteRoot, mountPath string) ([]InventoryItem, error) {
	remoteFiles, err := ListRemoteFiles(ctx, executable, remoteRoot)
	if err != nil {
		return nil, err
	}
	return CompareFileSets(mountPath, localFiles(mountPath), remoteFiles)
}

func CompareFileSets(mountPath string, localFiles, remoteFiles []string) ([]InventoryItem, error) {
	local := make(map[string]struct{}, len(localFiles))
	remote := make(map[string]struct{}, len(remoteFiles))
	for _, name := range localFiles {
		if normalized := normalizeRelative(name); normalized != "" {
			local[normalized] = struct{}{}
		}
	}
	for _, name := range remoteFiles {
		if normalized := normalizeRelative(name); normalized != "" {
			remote[normalized] = struct{}{}
		}
	}

	all := make(map[string]struct{}, len(local)+len(remote))
	for name := range local {
		all[name] = struct{}{}
	}
	for name := range remote {
		all[name] = struct{}{}
	}
	ordered := make([]string, 0, len(all))
	for name := range all {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	items := make([]InventoryItem, 0, len(ordered))
	for _, name := range ordered {
		_, localExists := local[name]
		_, remoteExists := remote[name]
		state := status.StateSynced
		switch {
		case localExists && !remoteExists:
			state = status.StateError
		case !localExists && remoteExists:
			state = status.StateCloudOnly
		}
		items = append(items, InventoryItem{
			RelativePath: name,
			LocalPath:    filepath.Join(mountPath, filepath.FromSlash(name)),
			LocalExists:  localExists,
			RemoteExists: remoteExists,
			State:        state,
		})
	}
	return items, nil
}

func ResyncMissing(ctx context.Context, executable, remoteRoot, mountPath, localPath string) (string, error) {
	relative, err := relativeToMount(mountPath, localPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return "", fmt.Errorf("read local source file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("resync source must be a file: %s", localPath)
	}

	remoteFiles, err := ListRemoteFiles(ctx, executable, remoteRoot)
	if err != nil {
		return "", err
	}
	remoteSet := make(map[string]struct{}, len(remoteFiles))
	for _, file := range remoteFiles {
		remoteSet[normalizeRelative(file)] = struct{}{}
	}
	if _, exists := remoteSet[normalizeRelative(relative)]; exists {
		return "", fmt.Errorf("S3 object already exists for %s; refusing to overwrite it", relative)
	}

	remoteTarget := strings.TrimRight(remoteRoot, "/") + "/" + filepath.ToSlash(relative)
	output, err := execCommand(ctx, executable, "copyto", "--ignore-existing", localPath, remoteTarget)
	if err != nil {
		return "", fmt.Errorf("rclone failed to upload missing object %s: %w: %s", relative, err, strings.TrimSpace(string(output)))
	}
	return remoteTarget, nil
}

func relativeToMount(mountPath, filePath string) (string, error) {
	root, err := filepath.Abs(filepath.Clean(mountPath))
	if err != nil {
		return "", fmt.Errorf("resolve mount root: %w", err)
	}
	file, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return "", fmt.Errorf("resolve file path: %w", err)
	}
	relative, err := filepath.Rel(root, file)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("file path must be inside configured mount %q", mountPath)
	}
	return filepath.ToSlash(relative), nil
}

func localFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if relative, relErr := filepath.Rel(root, path); relErr == nil {
			files = append(files, filepath.ToSlash(relative))
		}
		return nil
	})
	return files
}

func normalizeRelative(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimPrefix(name, "./")
	name = strings.Trim(name, "/")
	return strings.ToLower(name)
}

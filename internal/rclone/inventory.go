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

// AuditFolder compares local mount files under optional folderPath with the
// remote via rclone lsjson. folderPath may be empty (whole mount), a path under
// the mount, or a mount-relative folder. No direct S3 API is used.
func AuditFolder(ctx context.Context, executable, remoteRoot, mountPath, folderPath string) ([]InventoryItem, error) {
	folderRel, err := resolveFolderRelative(mountPath, folderPath)
	if err != nil {
		return nil, err
	}

	localRoot := mountPath
	if folderRel != "" {
		localRoot = filepath.Join(mountPath, filepath.FromSlash(folderRel))
		info, statErr := os.Stat(localRoot)
		if statErr != nil {
			return nil, fmt.Errorf("audit folder: %w", statErr)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("audit path must be a folder: %s", localRoot)
		}
	}

	remoteTarget := joinRemote(remoteRoot, folderRel)
	remoteFiles, err := ListRemoteFiles(ctx, executable, remoteTarget)
	if err != nil {
		return nil, err
	}
	if folderRel != "" {
		prefixed := make([]string, 0, len(remoteFiles))
		for _, name := range remoteFiles {
			prefixed = append(prefixed, normalizeRelative(folderRel+"/"+name))
		}
		remoteFiles = prefixed
	}

	return CompareFileSets(mountPath, localFiles(localRoot, mountPath), remoteFiles)
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

// ApplyInventory writes presentation state for local files so Explorer overlays
// reflect remote existence. Local-only files become error; files on both sides
// become synced. Cloud-only (remote without local) is skipped for overlays.
func ApplyInventory(store *status.Store, items []InventoryItem) []string {
	changed := make([]string, 0, len(items))
	for _, item := range items {
		if !item.LocalExists {
			continue
		}
		message := ""
		state := item.State
		switch state {
		case status.StateError:
			message = "object missing on remote"
		case status.StateSynced:
			message = ""
		case status.StateCloudOnly:
			continue
		default:
			state = status.StateSynced
		}
		store.Set(item.LocalPath, state, status.DirectionNone, message)
		changed = append(changed, item.LocalPath)
	}
	return changed
}

func ResyncMissing(ctx context.Context, executable, remoteRoot, mountPath, localPath string) (string, error) {
	return resyncMissing(ctx, execCommand, executable, remoteRoot, mountPath, localPath)
}

func resyncMissing(ctx context.Context, run CommandRunner, executable, remoteRoot, mountPath, localPath string) (string, error) {
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

	remoteFiles, err := listRemoteFiles(ctx, run, executable, remoteRoot)
	if err != nil {
		return "", err
	}
	remoteSet := make(map[string]struct{}, len(remoteFiles))
	for _, file := range remoteFiles {
		remoteSet[normalizeRelative(file)] = struct{}{}
	}
	if _, exists := remoteSet[normalizeRelative(relative)]; exists {
		return "", fmt.Errorf("remote object already exists for %s; refusing to overwrite it", relative)
	}

	remoteTarget := joinRemote(remoteRoot, relative)
	output, err := run(ctx, executable, "copyto", "--ignore-existing", localPath, remoteTarget)
	if err != nil {
		return "", fmt.Errorf("rclone failed to upload missing object %s: %w: %s", relative, err, strings.TrimSpace(string(output)))
	}
	return remoteTarget, nil
}

// ResyncMissingUnder audits folderPath and uploads every local-only file via rclone.
// It reuses the audit result so each missing file does not re-list the remote.
func ResyncMissingUnder(ctx context.Context, executable, remoteRoot, mountPath, folderPath string) (uploaded []string, failed map[string]string, err error) {
	items, err := AuditFolder(ctx, executable, remoteRoot, mountPath, folderPath)
	if err != nil {
		return nil, nil, err
	}
	failed = map[string]string{}
	for _, item := range items {
		if !item.LocalExists || item.RemoteExists {
			continue
		}
		target, resyncErr := uploadMissing(ctx, execCommand, executable, remoteRoot, item.RelativePath, item.LocalPath)
		if resyncErr != nil {
			failed[item.LocalPath] = resyncErr.Error()
			continue
		}
		uploaded = append(uploaded, target)
	}
	return uploaded, failed, nil
}

func uploadMissing(ctx context.Context, run CommandRunner, executable, remoteRoot, relative, localPath string) (string, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return "", fmt.Errorf("read local source file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("resync source must be a file: %s", localPath)
	}
	remoteTarget := joinRemote(remoteRoot, relative)
	output, err := run(ctx, executable, "copyto", "--ignore-existing", localPath, remoteTarget)
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

func resolveFolderRelative(mountPath, folderPath string) (string, error) {
	folderPath = strings.TrimSpace(folderPath)
	if folderPath == "" {
		return "", nil
	}
	cleanMount := filepath.Clean(mountPath)
	cleanFolder := filepath.Clean(folderPath)
	if filepath.IsAbs(cleanFolder) || (len(cleanFolder) >= 2 && cleanFolder[1] == ':') {
		return relativeToMount(cleanMount, cleanFolder)
	}
	joined := filepath.Join(cleanMount, cleanFolder)
	if info, err := os.Stat(joined); err == nil && !info.IsDir() {
		return "", fmt.Errorf("audit path must be a folder: %s", joined)
	}
	return relativeToMount(cleanMount, joined)
}

func joinRemote(remoteRoot, relative string) string {
	remoteRoot = strings.TrimRight(strings.TrimSpace(remoteRoot), "/")
	relative = normalizeRelative(relative)
	if relative == "" {
		return remoteRoot
	}
	return remoteRoot + "/" + relative
}

func localFiles(walkRoot, mountRoot string) []string {
	var files []string
	_ = filepath.WalkDir(walkRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if relative, relErr := filepath.Rel(mountRoot, path); relErr == nil {
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

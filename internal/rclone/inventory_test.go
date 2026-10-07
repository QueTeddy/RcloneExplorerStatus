package rclone

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/queteddy/rclone-explorer-status/internal/status"
)

func osWriteFile(path string) error {
	return os.WriteFile(path, []byte("sample"), 0o600)
}

func TestListRemoteFilesUsesStructuredRcloneListing(t *testing.T) {
	var gotExecutable string
	var gotArgs []string
	run := func(_ context.Context, executable string, args ...string) ([]byte, error) {
		gotExecutable = executable
		gotArgs = append([]string(nil), args...)
		return []byte(`[{"Path":"folder/a.txt","IsDir":false},{"Path":"folder","IsDir":true}]`), nil
	}

	files, err := listRemoteFiles(context.Background(), run, "rclone.exe", "s3:bucket/root")
	if err != nil {
		t.Fatalf("listRemoteFiles() error = %v", err)
	}
	if gotExecutable != "rclone.exe" {
		t.Fatalf("executable = %q, want rclone.exe", gotExecutable)
	}
	if !reflect.DeepEqual(gotArgs, []string{"lsjson", "--recursive", "--files-only", "s3:bucket/root"}) {
		t.Fatalf("rclone args = %#v", gotArgs)
	}
	if !reflect.DeepEqual(files, []string{"folder/a.txt"}) {
		t.Fatalf("files = %#v, want one regular object", files)
	}
}

func TestCompareFileSetsReportsLocalOnlyRemoteOnlyAndCommon(t *testing.T) {
	mount := t.TempDir()
	items, err := CompareFileSets(mount,
		[]string{"common.txt", "local-only.txt"},
		[]string{"common.txt", "remote-only.txt"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	got := map[string]InventoryItem{}
	for _, item := range items {
		got[item.RelativePath] = item
	}
	if item := got["common.txt"]; !item.LocalExists || !item.RemoteExists || item.State != status.StateSynced {
		t.Errorf("common item = %#v", item)
	}
	if item := got["local-only.txt"]; !item.LocalExists || item.RemoteExists || item.State != status.StateError {
		t.Errorf("local-only item = %#v", item)
	}
	if item := got["remote-only.txt"]; item.LocalExists || !item.RemoteExists || item.State != status.StateCloudOnly {
		t.Errorf("remote-only item = %#v", item)
	}
}

func TestApplyInventoryUpdatesLocalFilesOnly(t *testing.T) {
	mount := t.TempDir()
	localOnly := filepath.Join(mount, "local-only.txt")
	common := filepath.Join(mount, "common.txt")
	if err := osWriteFile(localOnly); err != nil {
		t.Fatal(err)
	}
	if err := osWriteFile(common); err != nil {
		t.Fatal(err)
	}
	store, err := status.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	items := []InventoryItem{
		{RelativePath: "local-only.txt", LocalPath: localOnly, LocalExists: true, RemoteExists: false, State: status.StateError},
		{RelativePath: "common.txt", LocalPath: common, LocalExists: true, RemoteExists: true, State: status.StateSynced},
		{RelativePath: "remote-only.txt", LocalPath: filepath.Join(mount, "remote-only.txt"), LocalExists: false, RemoteExists: true, State: status.StateCloudOnly},
	}
	changed := ApplyInventory(store, items)
	if len(changed) != 2 {
		t.Fatalf("changed = %d, want 2 local files", len(changed))
	}
	if record := store.Get(localOnly); record == nil || record.State != status.StateError || record.LastError == "" {
		t.Fatalf("local-only record = %#v", record)
	}
	if record := store.Get(common); record == nil || record.State != status.StateSynced {
		t.Fatalf("common record = %#v", record)
	}
	if store.Get(filepath.Join(mount, "remote-only.txt")) != nil {
		t.Fatal("cloud-only path should not be written for overlays")
	}
}

func TestResyncMissingRefusesExistingRemoteObjectBeforeCopy(t *testing.T) {
	mount := t.TempDir()
	file := filepath.Join(mount, "present.txt")
	if err := osWriteFile(file); err != nil {
		t.Fatal(err)
	}
	calls := 0
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls++
		if !reflect.DeepEqual(args, []string{"lsjson", "--recursive", "--files-only", "s3:bucket/root"}) {
			t.Fatalf("unexpected command; must not copy when key exists: %#v", args)
		}
		return []byte(`[{"Path":"present.txt","IsDir":false}]`), nil
	}
	_, err := resyncMissing(context.Background(), run, "rclone.exe", "s3:bucket/root", mount, file)
	if err == nil {
		t.Fatal("resyncMissing() succeeded for an existing remote object")
	}
	if calls != 1 {
		t.Fatalf("command calls = %d, want only the inventory check", calls)
	}
}

func TestResyncMissingRejectsPathsOutsideMount(t *testing.T) {
	mount := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	calls := 0
	run := func(context.Context, string, ...string) ([]byte, error) {
		calls++
		return nil, errors.New("must not run")
	}
	if _, err := resyncMissing(context.Background(), run, "rclone.exe", "s3:bucket", mount, outside); err == nil {
		t.Fatal("resyncMissing() accepted a path outside the mount")
	}
	if calls != 0 {
		t.Fatalf("runner called %d times for an out-of-mount path", calls)
	}
}

func TestJoinRemoteKeepsConfiguredRoot(t *testing.T) {
	if got := joinRemote("s3:bucket/root", "folder/a.txt"); got != "s3:bucket/root/folder/a.txt" {
		t.Fatalf("joinRemote() = %q", got)
	}
	if got := joinRemote("s3:bucket/root/", ""); got != "s3:bucket/root" {
		t.Fatalf("joinRemote empty = %q", got)
	}
}

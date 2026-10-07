package status

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	paths := []struct {
		input    string
		expected string
	}{
		{`S:\file.txt`, `s:\file.txt`},
		{`s:/file.txt`, `s:\file.txt`},
		{`S:/file.txt`, `s:\file.txt`},
		{`S:\courses\\nested\\file.txt`, `s:\courses\nested\file.txt`},
		{`\\?\S:\courses\file.txt`, `s:\courses\file.txt`},
	}

	for _, tc := range paths {
		got := NormalizePath(tc.input)
		if got != tc.expected {
			t.Fatalf("NormalizePath(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestStateTransitions(t *testing.T) {
	cases := []struct {
		from State
		to   State
		ok   bool
	}{
		{StateUnknown, StatePending, true},
		{StatePending, StateSyncing, true},
		{StateSyncing, StateSynced, true},
		{StateSyncing, StateError, true},
		{StateError, StateSyncing, true},
		{StateError, StateSynced, true},
		{StateSyncing, StateConflict, true},
		{StateUnknown, StateError, false},
		{StateSynced, StatePending, false},
	}

	for _, tc := range cases {
		if ok := IsValidTransition(tc.from, tc.to); ok != tc.ok {
			t.Fatalf("IsValidTransition(%q, %q) = %v, want %v", tc.from, tc.to, ok, tc.ok)
		}
	}
}

func TestStorePersistsJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	store.Set("S:\\courses\\jenkins.mp4", StateSyncing, DirectionUpload, "test failure")
	if err := store.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty state file")
	}

	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() error = %v", err)
	}

	file := loaded.Get(NormalizePath("s:/courses/jenkins.mp4"))
	if file == nil {
		t.Fatal("expected file record to exist after load")
	}
	if file.State != StateSyncing {
		t.Fatalf("state = %q, want %q", file.State, StateSyncing)
	}
}

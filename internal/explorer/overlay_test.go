package explorer

import (
	"testing"

	"github.com/queteddy/rclone-explorer-status/internal/status"
)

func TestForStateMapsSupportedStates(t *testing.T) {
	tests := []struct {
		state status.State
		want  Overlay
	}{
		{state: status.StateSynced, want: OverlaySynced},
		{state: status.StateSyncing, want: OverlaySyncing},
		{state: status.StatePending, want: OverlaySyncing},
		{state: status.StateConflict, want: OverlayError},
		{state: status.StateError, want: OverlayError},
		{state: status.StateUnknown, want: OverlayNone},
		{state: status.StateOffline, want: OverlayNone},
		{state: status.StateCloudOnly, want: OverlayNone},
	}
	for _, test := range tests {
		t.Run(string(test.state), func(t *testing.T) {
			if got := ForState(test.state); got != test.want {
				t.Fatalf("ForState(%q) = %q, want %q", test.state, got, test.want)
			}
		})
	}
}

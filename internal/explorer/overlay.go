package explorer

import "github.com/queteddy/rclone-explorer-status/internal/status"

type Overlay string

const (
	OverlayNone    Overlay = "none"
	OverlaySynced  Overlay = "synced"
	OverlaySyncing Overlay = "syncing"
	OverlayError   Overlay = "error"
)

func ForState(current status.State) Overlay {
	switch current {
	case status.StateSynced:
		return OverlaySynced
	case status.StateSyncing:
		return OverlaySyncing
	case status.StateError:
		return OverlayError
	case status.StatePending:
		return OverlaySyncing
	case status.StateConflict:
		return OverlayError
	default:
		return OverlayNone
	}
}

func (o Overlay) RegistrationName() string {
	switch o {
	case OverlayError:
		return "RcloneErrorOverlay"
	case OverlaySyncing:
		return "RcloneSyncingOverlay"
	case OverlaySynced:
		return "RcloneSyncedOverlay"
	default:
		return ""
	}
}


# PRD — Rclone Explorer Sync Status

## 1. Product Overview

### Product name

**Rclone Explorer Sync Status**

### Purpose

Build a free, open-source Windows application written in **Go** that provides Google Drive/OneDrive-style synchronization status icons directly inside Windows File Explorer for files managed by `rclone`.

The application must **not provide a replacement file browser** and must **not replace rclone**.

The user should continue using Windows Explorer normally.

Example:

```text
S:\
│
├── aws\
│
├── companies\
│
├── courses\
│   ├── Jenkins.mp4             🟢
│   ├── Docker.mp4              🔄
│   └── Kubernetes.mp4          🔴
│
├── documents\
│   ├── report.pdf              🟢
│   └── proposal.docx           ☁️
│
└── slides\
```

The icons communicate the current state of individual files.

---

# 2. Problem Statement

The current environment uses:

```text
Windows
   ↓
rclone
   ↓
Amazon S3
```

Example:

```bash
rclone mount s3:local-sync-folder-reclone S: --vfs-cache-mode full --links
```

The mount works and the S3 bucket is accessible.

However, Windows Explorer does not tell the user whether a file:

* is synchronized
* is currently uploading
* is currently downloading
* has an error
* is only available remotely
* is locally cached
* is waiting to synchronize
* has a conflict
* is excluded
* is offline

Google Drive, OneDrive and similar applications communicate this information through Explorer status overlays.

The objective is to provide that missing functionality while retaining rclone as the underlying data-transfer engine.

---

# 3. Goals

## Primary goals

The application must:

1. Be **free and open source**.
2. Be written in **Go**.
3. Run on **Windows 10 and Windows 11**.
4. Integrate directly with **Windows File Explorer**.
5. Display status icons directly over files/folders.
6. Work with an existing rclone mount.
7. Monitor rclone synchronization activity.
8. Maintain per-file synchronization state.
9. Update Explorer when status changes.
10. Run silently in the background.
11. Start automatically with Windows.
12. Require no separate file-management UI.
13. Provide diagnostics/logging.
14. Handle rclone failures gracefully.
15. Be installable/uninstallable cleanly.

---

# 4. Non-Goals

The application must **not**:

* replace rclone
* implement its own S3 client initially
* implement its own synchronization engine
* become another file explorer
* replace Windows Explorer
* require Mountain Duck
* require Google Drive
* require OneDrive
* require a paid application
* require a proprietary cloud API
* upload/download files directly in the first version
* modify the contents of files
* automatically delete files from S3
* manage AWS credentials

---

# 5. Target Architecture

```text
                         ┌──────────────────┐
                         │    Amazon S3      │
                         └────────┬─────────┘
                                  │
                                  │
                            ┌─────▼─────┐
                            │   rclone  │
                            │            │
                            │ Sync/Mount │
                            └─────┬─────┘
                                  │
                                  │
                            ┌─────▼─────┐
                            │    S:\     │
                            │            │
                            │ Windows    │
                            │ filesystem │
                            └─────┬──────┘
                                  │
                     ┌────────────┴────────────┐
                     │                         │
                     ▼                         ▼
             Windows Explorer          Go Status Service
                     │                         │
                     │                         │
                     │                  rclone RC/API
                     │                         │
                     │                  rclone logs/events
                     │                         │
                     │                         ▼
                     │                  Status Store
                     │                         │
                     └────────────┬────────────┘
                                  │
                                  ▼
                         Explorer Overlay
                              Handler
                                  │
                ┌─────────────────┼─────────────────┐
                │                 │                 │
                ▼                 ▼                 ▼
              🟢                🔄                🔴
             Synced            Syncing             Error
```

---

# 6. Major Components

The application consists of five major components.

## Component 1 — Status Service

Responsible for:

* monitoring rclone
* processing events
* maintaining file state
* detecting state transitions
* persisting state
* notifying Explorer
* exposing diagnostics

---

## Component 2 — Rclone Integration

The Go application should integrate with rclone using the **rclone Remote Control API where practical**.

Documentation:

[rclone Remote Control documentation](https://rclone.org/rc/?utm_source=chatgpt.com)

The implementation should not assume that the user always has RC enabled.

Therefore support two modes:

### Mode A — RC/API

Preferred.

```text
Go
 ↓
rclone RC
 ↓
rclone
```

### Mode B — Log monitoring

Fallback.

```text
Go
 ↓
rclone log
```

The application should detect which integration is available.

---

# 7. Explorer Integration

This is the most important component.

Implement a native Windows Explorer Shell Overlay Icon Handler.

The Go implementation should use Windows COM/Shell APIs.

Relevant Go dependency:

```text
golang.org/x/sys/windows
```

The implementation should be designed around the Windows `IShellIconOverlayIdentifier` mechanism where appropriate.

The extension should register multiple overlay handlers.

At minimum:

```text
RcloneSyncedOverlay
RcloneSyncingOverlay
RcloneErrorOverlay
```

Additional overlays may be added later.

---

# 8. Status Definitions

The application needs a formal state machine.

## Required states

### `unknown`

Initial state when the application has no knowledge of the file.

Icon:

```text
none
```

---

### `synced`

The local and remote versions are considered synchronized.

Icon:

```text
🟢
```

Recommended actual icon:

```text
green circle/check
```

Meaning:

> The file has successfully synchronized.

---

### `syncing`

The file is currently being uploaded or downloaded.

Icon:

```text
🔄
```

Recommended actual icon:

```text
blue circular arrows
```

---

### `error`

Synchronization failed.

Icon:

```text
🔴
```

Recommended actual icon:

```text
red circle with X
```

---

### `pending`

The file has changed locally but has not yet started synchronization.

Icon:

```text
🟡
```

Recommended:

```text
yellow/orange clock
```

---

### `conflict`

Local and remote versions cannot safely be reconciled.

Icon:

```text
⚠️
```

---

### `offline`

The application cannot currently communicate with rclone/S3.

Icon:

```text
⚪
```

or gray cloud.

---

### `cloud_only`

The file exists remotely but isn't locally cached.

Icon:

```text
☁️
```

This state is particularly important if the system eventually supports a cloud-file architecture.

---

# 9. Recommended Initial Icon Set

Create real `.ico` files.

Do NOT use emoji as the actual Explorer overlays.

Required:

```text
assets/
├── synced.ico
├── syncing.ico
├── error.ico
├── pending.ico
├── conflict.ico
├── offline.ico
└── cloud-only.ico
```

Each icon should:

* support Windows Explorer overlay dimensions
* have appropriate transparency
* be visually distinguishable at 16x16 and 32x32
* follow Windows UI conventions
* not contain copyrighted Google/OneDrive assets

---

# 10. Overlay Priority

Windows Explorer has a limited number of overlay slots.

Therefore the application must not register dozens of overlays.

Use the minimum:

### Priority 1

```text
Error
```

### Priority 2

```text
Syncing
```

### Priority 3

```text
Synced
```

Additional statuses should be represented through the existing three overlays where possible.

For example:

```text
pending → syncing
offline → no overlay or error
conflict → error
cloud_only → no overlay initially
```

The first release should use only:

```text
synced
syncing
error
```

This avoids unnecessary complexity.

---

# 11. Status Store

Create a local state store.

Version 1 can use JSON.

Example:

```json
{
  "version": 1,
  "files": {
    "S:\\courses\\jenkins.mp4": {
      "state": "synced",
      "direction": "upload",
      "updated_at": "2026-10-05T20:10:00Z",
      "last_error": null
    }
  }
}
```

Recommended location:

```text
%LOCALAPPDATA%\RcloneExplorerStatus\state.json
```

Logs:

```text
%LOCALAPPDATA%\RcloneExplorerStatus\logs\
```

Configuration:

```text
%APPDATA%\RcloneExplorerStatus\config.json
```

---

# 12. State Record

Each file should have:

```text
path
state
direction
last_state_change
last_success
last_error
retry_count
size
optional hash
```

Example:

```json
{
  "path": "S:\\courses\\Jenkins.mp4",
  "state": "syncing",
  "direction": "upload",
  "last_state_change": "2026-10-05T20:30:12Z",
  "last_success": "2026-10-05T20:29:50Z",
  "last_error": null,
  "retry_count": 0
}
```

---

# 13. State Machine

Implement explicit transitions.

```text
UNKNOWN
   │
   ▼
PENDING
   │
   ▼
SYNCING
   │
   ├───────────────┐
   │               │
   ▼               ▼
SYNCED           ERROR
   │               │
   │               │ retry
   │               ▼
   │            SYNCING
   │
   ▼
UNKNOWN
```

Additional:

```text
SYNCING
   │
   ▼
CONFLICT
```

---

# 14. Upload Flow

When a user creates/modifies:

```text
S:\documents\report.pdf
```

expected behavior:

```text
User saves file
       │
       ▼
PENDING
       │
       ▼
🔄 Explorer
       │
       ▼
rclone uploads
       │
       ▼
successful upload
       │
       ▼
🟢 Explorer
```

---

# 15. Error Flow

```text
User saves file
       │
       ▼
PENDING
       │
       ▼
SYNCING
       │
       ▼
rclone fails
       │
       ▼
ERROR
       │
       ▼
🔴 Explorer
```

The status record must retain:

```text
error message
timestamp
retry count
```

Example:

```json
{
  "state": "error",
  "last_error": "dial tcp: lookup ... no such host",
  "retry_count": 3
}
```

---

# 16. Retry Behavior

Do not implement independent upload retries if rclone already performs retries.

The Go application should observe rclone.

For example:

```text
rclone:
retry #1
retry #2
retry #3
success
```

Go should represent:

```text
🔄 syncing
```

until rclone finally succeeds or permanently fails.

If rclone reports failure:

```text
🔴 error
```

---

# 17. rclone Integration

The application must support configuration such as:

```json
{
  "rclone": {
    "executable": "rclone",
    "remote": "s3:local-sync-folder-reclone",
    "mount_path": "S:\\",
    "rc_enabled": true,
    "rc_address": "127.0.0.1:5572"
  }
}
```

Do not hardcode:

```text
S:
```

The user must be able to configure:

```text
S:
D:
X:
```

etc.

---

# 18. rclone Process Detection

The service should be able to detect whether rclone is running.

Windows process check:

```text
rclone.exe
```

If rclone is unavailable:

```text
state = offline
```

and log:

```text
rclone process unavailable
```

The application must not crash.

---

# 19. rclone RC

Where possible, communicate with:

```text
http://127.0.0.1:5572
```

or configured address.

Do not assume port `5572` is always available.

Configuration should support:

```text
host
port
username
password
TLS
```

if required.

---

# 20. rclone Startup

The application should **not automatically start rclone in version 1** unless explicitly configured.

The first release should assume:

```text
rclone is already running
```

This avoids interfering with an existing production setup.

Later versions may optionally support:

```json
{
  "start_rclone": true
}
```

---

# 21. Existing User Setup

The current user has:

```bash
rclone mount s3:local-sync-folder-reclone S: --vfs-cache-mode full --links
```

The application must work with this architecture.

Important:

The Go application does not control the mount.

It observes it.

---

# 22. S3 Awareness

Version 1 should **not** directly communicate with S3.

The architecture must remain:

```text
Go
 ↓
rclone
 ↓
S3
```

Not:

```text
Go ───────→ S3
```

This prevents duplicated credentials and synchronization logic.

---

# 23. File Identification

Paths must be normalized.

Windows:

```text
S:\courses\file.mp4
```

should always be stored consistently.

Normalize:

* slash direction
* case where appropriate
* trailing separators
* UNC paths
* drive letters

Do not accidentally treat:

```text
S:\File.txt
```

and:

```text
s:\file.txt
```

as separate files if Windows considers them equivalent for the application.

---

# 24. Directory Status

Explorer overlays should primarily operate on files.

For folders, initial behavior should be conservative.

A folder may show an error overlay if:

```text
any descendant has an active sync error
```

But this should be configurable.

Initial release:

> Show overlays on files only.

Folder aggregation can be Phase 2.

---

# 25. Explorer Refresh

When a file changes state:

```text
syncing → synced
```

Explorer must be notified so the icon updates.

Implement an Explorer refresh/invalidation mechanism.

The extension should not require:

```text
restart Explorer.exe
```

after every state change.

---

# 26. Explorer Cache

Explorer may cache overlay results.

Therefore the implementation must:

* invalidate affected paths
* notify Explorer
* avoid relying solely on polling
* handle Explorer restarts

If Explorer restarts:

```text
Go service remains running
        ↓
Explorer starts
        ↓
overlay handlers load
        ↓
state store is read
        ↓
icons return automatically
```

---

# 27. Shell Extension Registration

The installer must register the COM/Shell extension correctly.

Registry must be architecture-aware.

Support:

```text
64-bit Windows
```

Primary target:

```text
amd64
```

Optionally:

```text
arm64
```

Do not require users to manually edit Registry.

---

# 28. COM Registration

The installer must:

1. Register COM classes.
2. Register overlay identifiers.
3. Set approved overlay identifiers.
4. Configure DLL/EXE registration correctly.
5. Ensure Explorer can instantiate the handler.
6. Provide uninstall cleanup.

All registry modifications must be reversible.

---

# 29. Explorer Overlay Registry

The implementation must account for Windows' `ShellIconOverlayIdentifiers`.

Do not blindly overwrite existing entries.

The installer must:

* inspect existing keys
* avoid collisions
* use unique names
* preserve existing registrations
* provide uninstall cleanup

Example names:

```text
RcloneExplorerStatusSynced
RcloneExplorerStatusSyncing
RcloneExplorerStatusError
```

---

# 30. Installation

Provide:

```text
install.ps1
```

and eventually:

```text
RcloneExplorerStatusSetup.exe
```

Installation should:

1. Validate Windows version.
2. Validate architecture.
3. Install binaries.
4. Install icon resources.
5. Register Explorer handlers.
6. Create configuration.
7. Create log directory.
8. Register Windows startup/service.
9. Start service.
10. Notify Explorer.

---

# 31. Uninstallation

Uninstall must:

* stop service
* remove binaries
* remove registry entries created by the application
* remove Explorer handlers
* refresh Explorer
* optionally preserve logs/configuration

Never remove unrelated registry keys.

---

# 32. Startup

The application should start automatically.

Preferred initial mechanism:

```text
Windows Startup / Scheduled Task
```

rather than requiring administrator privileges for the runtime process.

Installation may require administrator rights.

Runtime should ideally run as the logged-in user.

---

# 33. No UI Requirement

The application must not require a UI.

Optional:

```text
system tray icon
```

may be implemented later.

The first release should work completely headlessly.

The only visible indication should be:

```text
Windows Explorer
```

---

# 34. Logging

Use structured logs.

Example:

```text
2026-10-05T20:30:00Z INFO rclone connection established
2026-10-05T20:30:03Z INFO file state changed
path=S:\documents\report.pdf state=syncing
2026-10-05T20:30:10Z INFO file state changed
path=S:\documents\report.pdf state=synced
```

Errors:

```text
ERROR path=S:\courses\video.mp4 state=error
error="dial tcp: lookup ..."
```

Use log levels:

```text
DEBUG
INFO
WARN
ERROR
```

---

# 35. Log Rotation

Logs must not grow indefinitely.

Implement:

```text
max size
max files
max age
```

Example defaults:

```text
10 MB/file
5 files
30 days
```

---

# 36. Configuration

Example:

```json
{
  "mount": {
    "path": "S:\\"
  },
  "rclone": {
    "executable": "rclone.exe",
    "remote": "s3:local-sync-folder-reclone",
    "rc": {
      "enabled": true,
      "host": "127.0.0.1",
      "port": 5572
    }
  },
  "status": {
    "state_file": "%LOCALAPPDATA%\\RcloneExplorerStatus\\state.json"
  },
  "logging": {
    "level": "info"
  }
}
```

---

# 37. CLI

The application should expose a CLI.

Example:

```bash
rclone-explorer-status.exe status
```

Output:

```text
Service: Running
rclone: Running
Mount: S:\
Remote: s3:local-sync-folder-reclone
Files tracked: 1,284
Syncing: 3
Errors: 1
Synced: 1,280
```

Commands:

```text
install
uninstall
start
stop
restart
status
doctor
config
clear-state
version
```

---

# 38. Doctor Command

This is important.

Implement:

```bash
rclone-explorer-status.exe doctor
```

It should check:

```text
Windows version
Architecture
Go application
rclone executable
rclone version
rclone process
rclone RC
mount path
mount accessibility
state directory
Explorer registration
overlay registration
permissions
```

Example:

```text
[OK] Windows 11
[OK] amd64
[OK] rclone.exe found
[OK] rclone process running
[OK] S:\ accessible
[OK] RC API available
[OK] Explorer overlay registered
[OK] State store writable
```

---

# 39. Testing Strategy

Testing must be extensive.

## Unit tests

Test:

### State transitions

```text
unknown → syncing
syncing → synced
syncing → error
error → syncing
error → synced
```

### Path normalization

```text
S:\file.txt
s:/file.txt
S:/file.txt
```

### State persistence

Write/read JSON.

### Concurrent updates

Multiple files changing simultaneously.

### Error handling

Malformed rclone events.

---

# 40. Integration Tests

Test against an actual rclone process.

Test:

```text
rclone → Go
```

with:

* upload
* download
* error
* retry
* cancellation
* process restart

---

# 41. Explorer Tests

Test:

### Test 1

Create:

```text
S:\test\synced.txt
```

State:

```text
synced
```

Expected:

```text
green overlay
```

### Test 2

State:

```text
syncing
```

Expected:

```text
blue overlay
```

### Test 3

State:

```text
error
```

Expected:

```text
red overlay
```

---

# 42. Explorer Restart Test

Start application.

Verify icons.

Kill:

```text
explorer.exe
```

Restart Explorer.

Verify icons return automatically.

---

# 43. Application Restart Test

Stop Go service.

Restart.

Verify state persists.

Example:

```text
report.pdf = synced
```

must remain:

```text
synced
```

---

# 44. rclone Restart Test

Stop rclone.

Expected:

```text
offline
```

Restart rclone.

Expected:

```text
syncing
```

or:

```text
synced
```

depending on actual state.

---

# 45. Network Failure Test

Disconnect network.

Expected:

```text
rclone retries
```

Go should not crash.

Files should transition appropriately.

After network returns:

```text
syncing
   ↓
synced
```

---

# 46. DNS Failure Test

Simulate the current problem:

```text
dial tcp:
lookup ...:
no such host
```

Expected:

```text
🔴 error
```

The error should appear in logs.

When retry succeeds:

```text
🔄
 ↓
🟢
```

---

# 47. Large File Test

Use:

```text
500 MB
1 GB
5 GB
```

files.

Verify:

```text
syncing
```

remains displayed during transfer.

After completion:

```text
synced
```

---

# 48. Concurrent File Test

Copy 20 files simultaneously.

Expected:

```text
file1  🔄
file2  🔄
file3  🔄
...
file20 🔄
```

As they complete:

```text
file1  🟢
file2  🟢
file3  🔄
...
```

---

# 49. Unicode Test

Test filenames:

```text
résumé.pdf
文档.pdf
دليل.pdf
DevOps – Advanced.mp4
```

Verify state tracking doesn't break.

---

# 50. Spaces Test

Example:

```text
Demo Onelogin Integration with Jenkins using SAML.mp4
```

This is particularly important because your current S3 objects contain long filenames and spaces.

---

# 51. Deep Path Test

Example:

```text
S:\courses\Learn DevOps_ CICD With Jenkins Using Pipelines And Docker\8. Advanced Jenkins usage\13. Demo Onelogin Integration with Jenkins using SAML.mp4
```

The status system must handle long paths.

---

# 52. Long Path Support

Enable/test Windows long-path behavior.

Do not truncate paths internally.

Use absolute normalized paths.

---

# 53. Performance Requirements

The status service should:

* use low CPU
* use low memory
* not continuously scan the entire drive
* not continuously query S3
* not open every file
* not interfere with rclone

Target:

```text
CPU: <1% idle
Memory: <100 MB
```

Typical idle state.

---

# 54. Event-Driven Architecture

Prefer:

```text
rclone event
      ↓
Go
      ↓
state update
      ↓
Explorer notification
```

over:

```text
every 1 second
   ↓
scan S:
   ↓
scan everything
```

Full scans should only occur:

* startup
* recovery
* manual doctor
* explicit resync

---

# 55. Cache Strategy

The status store should be cached in memory.

```text
JSON
 ↓
memory map
 ↓
Explorer queries
```

Explorer requests should not require disk access.

---

# 56. Thread Safety

The Go application must support concurrent access.

Use:

```go
sync.RWMutex
```

or another appropriate synchronization mechanism.

Explorer overlay handlers may query state concurrently.

---

# 57. Crash Recovery

If Go crashes:

1. Restart.
2. Load state.
3. Detect rclone state.
4. Reconcile active files.
5. Refresh Explorer.

Do not mark every file as error simply because the service restarted.

---

# 58. Stale State

State records should have timestamps.

If:

```text
syncing
```

has been active for an unreasonable period, log:

```text
WARN stale sync state
```

Do not automatically mark error without evidence.

---

# 59. Security

The application must:

* never log AWS secrets
* never log access keys
* never expose rclone passwords
* restrict state file permissions where possible
* bind RC to localhost by default
* not expose a network server unnecessarily

---

# 60. Privacy

The application should not send telemetry.

No:

```text
analytics
tracking
cloud telemetry
```

unless explicitly added later and opt-in.

---

# 61. Open Source

License:

```text
MIT
```

or:

```text
Apache-2.0
```

Recommended:

```text
MIT
```

Repository must contain:

```text
LICENSE
README.md
CONTRIBUTING.md
SECURITY.md
CHANGELOG.md
```

---

# 62. GitHub Repository

Suggested:

```text
rclone-explorer-status
```

Structure:

```text
.github/
├── workflows/
│   ├── build.yml
│   ├── test.yml
│   └── release.yml
│
├── ISSUE_TEMPLATE/
└── pull_request_template.md

cmd/
internal/
assets/
scripts/
tests/
docs/
```

---

# 63. CI/CD

GitHub Actions should:

```text
checkout
 ↓
setup Go
 ↓
go mod download
 ↓
go vet
 ↓
go test
 ↓
go build
 ↓
build Windows binary
```

Matrix:

```text
windows-amd64
```

Optional:

```text
windows-arm64
```

---

# 64. Build Commands

The project should support:

```bash
go build ./...
```

Testing:

```bash
go test ./...
```

Race detector where supported:

```bash
go test -race ./...
```

Vet:

```bash
go vet ./...
```

---

# 65. Release

Produce:

```text
rclone-explorer-status-windows-amd64.zip
```

containing:

```text
rclone-explorer-status.exe
install.ps1
uninstall.ps1
README.md
LICENSE
```

Eventually produce:

```text
RcloneExplorerStatusSetup.exe
```

---

# 66. Versioning

Use Semantic Versioning:

```text
v0.1.0
```

Initial stages:

```text
v0.1.0 = Explorer overlay proof
v0.2.0 = state engine
v0.3.0 = rclone integration
v0.4.0 = installer
v0.5.0 = production hardening
v1.0.0 = stable release
```

---

# 67. MVP Definition

The MVP is considered complete when all of these work:

### rclone

```text
rclone mount → S:\
```

### Go

```text
Go service running
```

### Explorer

```text
file → green synced overlay
file → blue syncing overlay
file → red error overlay
```

### State

```text
state survives restart
```

### Installation

```text
install.ps1
```

### Uninstallation

```text
uninstall.ps1
```

### No UI

No GUI required.

---

# 68. MVP Demo

The final MVP demo should be:

### Step 1

Mount:

```bash
rclone mount s3:local-sync-folder-reclone S: --vfs-cache-mode full --links
```

### Step 2

Start:

```bash
rclone-explorer-status.exe start
```

### Step 3

Open:

```text
S:\
```

### Step 4

Copy:

```text
test.mp4
```

into S:.

Explorer:

```text
test.mp4 🔄
```

### Step 5

Upload completes.

Explorer:

```text
test.mp4 🟢
```

### Step 6

Cause an upload failure.

Explorer:

```text
test.mp4 🔴
```

### Step 7

Restore connectivity.

Explorer:

```text
test.mp4 🔄
```

then:

```text
test.mp4 🟢
```

That is the fundamental product.

---

# 69. Important rclone Error Case From Current Environment

The current environment has produced:

```text
dial tcp: lookup local-sync-folder-reclone.s3.us-east-1.amazonaws.com: no such host
```

The application must treat this as an rclone synchronization failure.

It must **not attempt to solve DNS itself**.

Instead:

```text
rclone error
      ↓
Go detects failure
      ↓
file state = error
      ↓
Explorer = 🔴
```

When rclone retries successfully:

```text
rclone retry
      ↓
upload succeeds
      ↓
file state = synced
      ↓
Explorer = 🟢
```

This is a core acceptance test.

---

# 70. Important Architectural Rule

**Never make the Go application the source of truth for actual file synchronization.**

Rclone remains the source of truth for transfer operations.

Go maintains:

> **presentation state**

not:

> **file transfer state**

The distinction is important.

```text
rclone:
"Did the file actually upload?"

Go:
"How should Explorer represent what rclone reported?"
```

---

# 71. Future Features

Do not implement initially, but design so these are possible.

### Cloud-only

```text
☁️
```

### Offline available

```text
✓
```

### Selective sync

```text
right-click → Always keep on this device
```

### Right-click menu

```text
Rclone
├── Sync now
├── Retry
├── Open remote
└── Show sync status
```

### Tray application

```text
Rclone Sync
────────────────
Connected
1 syncing
0 errors
```

### Multi-remote

```text
S:\
X:\
Y:\
```

### Multiple rclone processes

```text
S: → S3
X: → Azure Blob
Y: → Google Drive
```

---

# 72. Future Cloud Files API

The project should keep the architecture flexible enough to eventually support Windows Cloud Files API.

That would allow a future experience closer to OneDrive:

```text
☁ online only
✓ available locally
🔄 syncing
```

However:

> **Do not implement Cloud Files API in MVP.**

Start with Explorer overlays.

---

# 73. Coding Standards

Go code should follow:

```text
gofmt
go vet
golangci-lint
```

Use idiomatic Go.

Avoid unnecessary abstractions.

Interfaces should be introduced where they make testing easier.

Do not create a giant `main.go`.

---

# 74. Error Handling

Errors must be:

* wrapped with context
* logged
* recoverable where possible
* surfaced through `doctor`

Example:

```go
return fmt.Errorf("connect to rclone RC: %w", err)
```

Avoid:

```go
panic(...)
```

for normal runtime errors.

---

# 75. Graceful Shutdown

Support:

```text
SIGINT
SIGTERM
Windows service stop
```

Shutdown should:

1. stop event listeners
2. flush state
3. close files
4. release resources
5. exit cleanly

---

# 76. State File Corruption

If:

```text
state.json
```

is corrupted:

1. Don't crash.
2. Rename it:

```text
state.json.corrupt.<timestamp>
```

3. Create a new state file.
4. Log the event.
5. Rebuild state where possible.

---

# 77. Atomic State Writes

Never write directly to:

```text
state.json
```

Use:

```text
state.json.tmp
```

then rename atomically.

This prevents corruption if the process crashes during write.

---

# 78. Explorer Handler Performance

The Explorer overlay handler must return quickly.

It should **never**:

```text
call S3
call rclone
read a huge log
perform network requests
```

during an Explorer icon request.

It should simply:

```text
path
 ↓
memory lookup
 ↓
status
 ↓
icon
```

Target:

```text
< 10 ms
```

for normal lookup.

---

# 79. Test Automation

Create a test script:

```text
scripts/test-overlay.ps1
```

It should create:

```text
S:\RcloneOverlayTest\
├── synced.txt
├── syncing.txt
└── error.txt
```

and populate the status store accordingly.

Then the developer can visually verify:

```text
synced.txt   🟢
syncing.txt  🔄
error.txt    🔴
```

---

# 80. Developer Acceptance Criteria

Copilot must not declare completion simply because:

```text
go build
```

works.

The following must be demonstrated:

### AC1

Explorer overlay DLL/COM registration works.

### AC2

Explorer displays the synced icon.

### AC3

Explorer displays syncing icon.

### AC4

Explorer displays error icon.

### AC5

Explorer changes icons without restarting Explorer.

### AC6

State survives application restart.

### AC7

State survives Explorer restart.

### AC8

rclone failure results in error status.

### AC9

rclone recovery results in synced status.

### AC10

Long filenames work.

### AC11

Unicode filenames work.

### AC12

Concurrent transfers work.

### AC13

Uninstall removes registrations.

### AC14

Application does not interfere with rclone.

### AC15

No AWS credentials are stored by the application.

---

# 81. Copilot Development Instructions

Give Copilot these rules:

```text
You are implementing a production-quality Windows application in Go.

Do not build a mock-only implementation.

Do not replace rclone.

Do not implement an S3 client.

Do not create a second file browser.

Do not use a web application.

Do not use Electron.

Do not use Mountain Duck.

The application must integrate directly with Windows Explorer.

The first priority is proving that native Windows Explorer overlay icons work.

Implement the project incrementally.

After every major phase:
1. build
2. test
3. document
4. provide manual verification steps

Do not move to the next phase if the previous phase is broken.

Use idiomatic Go.

Keep components independently testable.

Never hardcode the user's S: drive.

All paths and rclone settings must be configurable.

Never store AWS credentials.

Never expose rclone credentials in logs.

Use localhost for rclone RC by default.

Provide detailed error logging.

Provide a doctor command.

Provide install and uninstall scripts.

Write unit and integration tests.

The final result must run as a background Windows application with no required UI.

Explorer is the primary user interface.
```

---

# 82. Recommended Development Phases

## Phase 0 — Repository

Create:

```text
go.mod
README.md
LICENSE
.gitignore
```

Verify:

```bash
go build ./...
```

---

## Phase 1 — Explorer Overlay Proof

Implement:

```text
synced
syncing
error
```

with a manually populated state store.

**Do not implement rclone integration yet.**

Goal:

```text
Explorer → icons
```

---

## Phase 2 — State Engine

Implement:

```text
state.json
memory cache
state transitions
atomic persistence
```

Goal:

```text
file → state → overlay
```

---

## Phase 3 — rclone Integration

Implement:

```text
rclone RC
```

and fallback log monitoring.

Goal:

```text
rclone → state engine → Explorer
```

---

## Phase 4 — Failure Handling

Implement:

```text
retry
error
offline
recovery
```

Test the exact DNS failure you've encountered.

---

## Phase 5 — Production Hardening

Implement:

```text
logging
doctor
configuration
crash recovery
Explorer refresh
performance
```

---

## Phase 6 — Installer

Implement:

```text
install.ps1
uninstall.ps1
startup
registry
```

---

## Phase 7 — CI/CD

Implement GitHub Actions.

---

## Phase 8 — Release

Create:

```text
v0.1.0
```

with documentation.

---

# 83. Definition of Done

The project is **not done** until this works on a clean Windows machine:

```text
Install application
       ↓
Start rclone
       ↓
Mount S:\
       ↓
Open Explorer
       ↓
Create/edit file
       ↓
rclone starts transfer
       ↓
Explorer shows 🔄
       ↓
rclone succeeds
       ↓
Explorer shows 🟢
       ↓
force failure
       ↓
Explorer shows 🔴
       ↓
restore connection
       ↓
Explorer shows 🔄
       ↓
transfer succeeds
       ↓
Explorer shows 🟢
```

And:

```text
Restart Windows
       ↓
rclone starts
       ↓
Go status service starts
       ↓
Explorer starts
       ↓
icons work
```

without manually opening another application.

---

# 84. First Milestone

Don't ask Copilot to build the entire system in one shot.

Give it the PRD above, then give it this first implementation instruction:

> **Implement Phase 0 and Phase 1 only. Do not implement rclone integration yet. Build a minimal Go Windows Explorer Shell Overlay Icon Handler with three statuses: synced, syncing, and error. Use a local JSON state file for testing. Provide the complete source code, icon assets, registry/installation script, unit tests, and manual verification procedure. The first acceptance criterion is that Windows Explorer visibly displays different overlay icons for three test files without restarting Explorer when the status changes.**

Once Phase 1 is actually working, move to Phase 2.

That sequencing is important because **the hardest part is the native Windows Explorer integration**, not the rclone monitoring. If we prove that first, we know the entire concept is viable before we spend time building the state engine and rclone integration.

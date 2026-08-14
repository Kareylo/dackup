package backend

import (
	"dackup/internal/backend/default"
	"dackup/internal/shared"
)

// DefaultBackendName re-exports default.Name so callers only need to
// import this top-level package, not reach into the defaultbackend
// subpackage directly.
const DefaultBackendName = defaultbackend.Name

// Backend runs a backup or restore operation against a staging directory
// prepared by shared.TransferService. Implementations own their own typed
// configuration (see internal/backend/settings.go) and live in their own
// subpackage (see internal/backend/defaultbackend/).
type Backend interface {
	// Name returns the backend's identifier, e.g. "borg". For the default
	// no-op backend it returns "none" — a display-only value, never a
	// value written to a config file.
	Name() string

	// Backup reads staged data from stagingDir (as prepared by
	// shared.TransferService) and sends it to backend storage.
	Backup(stagingDir string) error

	// Restore fetches data from backend storage into stagingDir, ready for
	// shared.TransferService to copy into the live application paths.
	Restore(stagingDir string) error
}

// GroupedBackend is an optional interface a Backend can also implement to
// operate per container-group rather than on the whole stagingDir at once
// — e.g. one repository per group, plus a full mirror in a separate global
// repository. cmd/backup and cmd/restore type-assert the resolved Backend
// for GroupedBackend and prefer it when available; a Backend that doesn't
// implement it (like the default no-op backend) just gets the plain
// Backup/Restore call.
//
// Groups are shared.BackendGroup (not a type defined in this package) so
// that a concrete backend package (e.g. internal/backend/borg) can
// implement this interface by depending only on internal/shared, without
// importing internal/backend — internal/backend's own Factory already has
// to import every concrete backend subpackage to construct it, so the
// reverse import would be a cycle.
type GroupedBackend interface {
	BackupGroups(stagingDir string, groups []shared.BackendGroup) error
	RestoreGroups(stagingDir string, groups []shared.BackendGroup) error
}

// BinaryChecker is an optional interface a Backend can implement so callers
// can verify its CLI binary is on PATH during preflight, before any
// containers are stopped — otherwise a missing borg/kopia binary is only
// discovered after backup's stop/stage/restart sequence has already run
// (Backend.Backup is the last step) or restore's has already been staged
// for it (Backend.Restore is the first step, but still after preflight). A
// Backend that never shells out (the default no-op backend) just doesn't
// implement this and is skipped.
type BinaryChecker interface {
	BinaryName() string
}

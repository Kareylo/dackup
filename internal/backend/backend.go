package backend

import "dackup/internal/backend/default"

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

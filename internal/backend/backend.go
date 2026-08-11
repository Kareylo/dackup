package backend

import "dackup/internal/shared"

// DefaultBackendName is the display name of DefaultBackend. It is never
// written to a config file; an unset Backend field is the only "no backend
// configured" state.
const DefaultBackendName = "none"

// Backend runs a backup or restore operation against a staging directory
// prepared by shared.TransferService. Implementations own their own typed
// configuration (see internal/backend/settings.go).
type Backend interface {
	Name() string
	Backup(stagingDir string) error
	Restore(stagingDir string) error
}

// DefaultBackend is a no-op Backend loaded whenever no backend is configured.
type DefaultBackend struct {
	Logger shared.Logger
}

func (backend DefaultBackend) Name() string {
	return DefaultBackendName
}

func (backend DefaultBackend) Backup(stagingDir string) error {
	backend.log("No backend configured; skipping backup")
	return nil
}

func (backend DefaultBackend) Restore(stagingDir string) error {
	backend.log("No backend configured; skipping restore")
	return nil
}

func (backend DefaultBackend) log(message string) {
	if backend.Logger == nil {
		return
	}

	backend.Logger.Log("INFO", message)
}

package defaultbackend

import "dackup/internal/shared"

// Name is the display name of Backend. It is never written to a config
// file; an unset Backend field on shared.DackupConfig is the only "no
// backend configured" state.
const Name = "none"

// Backend is a no-op backend.Backend loaded whenever no backend is
// configured. It lives in its own subpackage, same as every other concrete
// backend (see internal/backend/borg/, planned), so the folder structure
// stays consistent regardless of which backend is being looked at.
type Backend struct {
	Logger shared.Logger
}

// Name returns the display name "none". It is never written to a config
// file — see the Name constant.
func (backend Backend) Name() string {
	return Name
}

// Backup does nothing but log that no backend is configured. stagingDir is
// accepted to satisfy backend.Backend but is otherwise unused.
func (backend Backend) Backup(stagingDir string) error {
	backend.log("No backend configured; skipping backup")
	return nil
}

// Restore does nothing but log that no backend is configured. stagingDir is
// accepted to satisfy backend.Backend but is otherwise unused.
func (backend Backend) Restore(stagingDir string) error {
	backend.log("No backend configured; skipping restore")
	return nil
}

func (backend Backend) log(message string) {
	if backend.Logger == nil {
		return
	}

	backend.Logger.Log("INFO", message)
}

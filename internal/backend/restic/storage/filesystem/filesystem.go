// Package filesystem implements storage.Provider for restic repositories
// stored as local directories — the default storage type, matching how
// internal/backend/borg and internal/backend/kopia's own filesystem storage
// type work.
package filesystem

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/shared"
	"path/filepath"
)

// Name is the identifier written to restic.Config.StorageType.
const Name = "filesystem"

// Storage is the storage.Provider for StorageType "filesystem". Unlike
// every other provider, it has no settings decoded from BackendSettings —
// its only input is ReposRoot (restic.Backend.ReposRoot), so
// restic.Config.provider constructs it directly rather than reading it off
// a Config field.
type Storage struct {
	ReposRoot string
}

// Validate always succeeds: there are no filesystem-specific settings to be
// malformed. ReposRoot itself is validated elsewhere —
// internal/backend/factory.go's Factory.GetBackend requires BackendDir to
// be set before constructing a restic.Backend at all.
func (s Storage) Validate() error {
	return nil
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	return storage.Invocation{Repository: filepath.Join(s.ReposRoot, repoName)}, nil
}

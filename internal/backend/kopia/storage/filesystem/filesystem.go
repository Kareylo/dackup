// Package filesystem implements storage.Provider for kopia repositories
// stored as local directories — the default storage type, matching how
// internal/backend/borg stores its repositories.
package filesystem

import (
	"dackup/internal/backend/kopia/storage"
	"dackup/internal/shared"
	"path/filepath"
)

// Name is the identifier written to kopia.Config.StorageType (and kopia's
// own "repository create/connect filesystem" subcommand), mirroring how
// internal/backend/borg and internal/backend/kopia identify themselves via
// their own Name constants.
const Name = "filesystem"

// Storage is the storage.Provider for StorageType "filesystem". Unlike
// every other provider, it has no settings decoded from BackendSettings —
// its only input is ReposRoot (kopia.Backend.ReposRoot), so
// kopia.Config.provider constructs it directly rather than reading it off
// a Config field.
type Storage struct {
	ReposRoot string
}

// Validate always succeeds: there are no filesystem-specific settings to
// be malformed. ReposRoot itself is validated elsewhere —
// internal/backend/factory.go's Factory.GetBackend requires BackendDir to
// be set before constructing a kopia.Backend at all.
func (fs Storage) Validate() error {
	return nil
}

func (fs Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	return storage.Invocation{
		Kind: Name,
		Args: []string{"--path=" + filepath.Join(fs.ReposRoot, repoName)},
	}, nil
}

// Package storage defines the contract every kopia storage backend
// implements (Provider) and the value it produces (Invocation), plus the
// small set of helpers shared by more than one storage type. Each concrete
// storage type lives in its own subpackage — storage/s3, storage/sftp,
// storage/b2, storage/azure, storage/gcs, storage/rclone, storage/webdav,
// storage/filesystem — so that adding, testing, or reasoning about one
// storage type never requires touching another's package. Every leaf
// subpackage imports this one (for Provider/Invocation and any shared
// helper it needs); this package imports none of them, so there's no cycle
// even though internal/backend/kopia itself also imports both this package
// and every leaf.
package storage

import (
	"dackup/internal/shared"
	"path"
)

// Invocation is the storage-type-specific portion of a "repository
// create/connect" call: the provider name kopia expects as its subcommand
// (e.g. "s3", matching that type's own Name constant), the CLI flags after
// it, and any extra environment variables the storage's credentials need
// beyond the repository password (which internal/backend/kopia supplies
// separately, the same way for every type).
type Invocation struct {
	Kind string
	Args []string
	Env  []string
}

// Provider is implemented by each concrete storage type's Storage struct
// (s3.Storage, sftp.Storage, ... and the built-in filesystem.Storage) and
// is kopia's single extension point for adding a new storage type: the
// type owns both its own validation and how it turns a repository name
// into an Invocation. Adding a storage type means adding a new subpackage
// implementing this interface, a field on kopia.Config, and one case in
// kopia.Config's provider dispatch — no existing storage type's package
// needs to change.
type Provider interface {
	// Validate reports whether the settings are well-formed.
	Validate() error

	// BuildInvocation decrypts any secret the storage type needs via
	// secrets and returns the Invocation for repoName's repository.
	BuildInvocation(repoName string, secrets shared.SecretStore) (Invocation, error)
}

// ObjectPrefix joins a configured base prefix with repoName into an
// object-storage key prefix, trailing-slashed the way kopia's docs
// recommend for a prefix meant to act as a directory. Shared by the four
// bucket-and-prefix storage types (s3, b2, azure, gcs); sftp and rclone
// address a literal directory instead, via plain path.Join in their own
// packages, and webdav addresses a URL via its own package-local urlJoin
// (path.Join isn't safe for a "scheme://" string).
func ObjectPrefix(base string, repoName string) string {
	return path.Join(base, repoName) + "/"
}

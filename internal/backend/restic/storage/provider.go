// Package storage defines the contract every restic storage backend
// implements (Provider) and the value it produces (Invocation), plus the
// small helper shared by more than one storage type. Each concrete storage
// type lives in its own subpackage — storage/s3, storage/sftp, storage/b2,
// storage/azure, storage/gcs, storage/rclone, storage/rest, storage/swift,
// storage/filesystem — mirroring internal/backend/kopia/storage's split, for
// the same reason: adding, testing, or reasoning about one storage type
// never requires touching another's package. Every leaf subpackage imports
// this one; this package imports none of them, so there's no cycle even
// though internal/backend/restic itself imports both this package and every
// leaf.
package storage

import (
	"dackup/internal/shared"
	"path"
)

// Invocation is the storage-type-specific portion of a restic CLI call:
// the repository address restic's own -r flag (and RESTIC_REPOSITORY env
// var) expects, plus any extra environment variables the storage's
// credentials need beyond the repository password (which
// internal/backend/restic supplies separately, the same way for every
// type), plus any extra global CLI flags restic needs before its
// subcommand — used only by storage/sftp (a non-default port or explicit
// keyfile, via -o sftp.command=...) and storage/rclone (a non-default
// rclone binary, via -o rclone.program=...); every other type leaves this
// empty. Unlike kopia's Invocation (a CLI subcommand name plus flags),
// restic addresses everything via a single repository URI string, so there
// is no separate "Kind" field.
type Invocation struct {
	Repository string
	Env        []string
	Args       []string
}

// Provider is implemented by each concrete storage type's Storage struct
// (s3.Storage, sftp.Storage, ... and the built-in filesystem.Storage) and
// is restic's single extension point for adding a new storage type — see
// internal/backend/kopia/storage.Provider's doc comment for the same
// reasoning, which applies unchanged here.
type Provider interface {
	// Validate reports whether the settings are well-formed.
	Validate() error

	// BuildInvocation decrypts any secret the storage type needs via
	// secrets and returns the Invocation for repoName's repository.
	BuildInvocation(repoName string, secrets shared.SecretStore) (Invocation, error)
}

// RepoPath joins a configured base path with repoName into the path segment
// several storage types append to their repository address (e.g. restic's
// "b2:bucket:path/to/repo", "azure:container:/path"). Shared by the
// bucket/container-and-path storage types (s3, b2, azure, gcs, swift); sftp
// and rclone address a literal remote directory instead via their own
// path.Join calls, and rest addresses a URL via its own package-local
// urlJoin (path.Join isn't safe for a "scheme://" string). Unlike
// internal/backend/kopia/storage.ObjectPrefix, this has no trailing slash —
// restic's own documented examples (e.g. "b2:bucketname:path/to/repo") show
// a plain path, not an object-key-style prefix.
func RepoPath(base string, repoName string) string {
	return path.Join(base, repoName)
}

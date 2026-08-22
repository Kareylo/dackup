// Package kopia implements backend.Backend and backend.GroupedBackend by
// driving the kopia CLI. It only depends on internal/shared (not
// internal/backend) for the same reason internal/backend/borg does — see
// that package's doc comment.
//
// The package is split one-file-per-responsibility, per AGENTS.md's "File
// Naming Guidelines":
//
//   - kopia.go (this file) — Config, the top-level typed settings decoded
//     from DackupConfig.BackendSettings.
//   - backend.go — the Backend type implementing backend.Backend and
//     backend.GroupedBackend, and its dependency defaults.
//   - repository.go — the lower-level mechanics of driving the kopia CLI
//     against one named repository (connect/create/snapshot/restore).
//   - storage/ — the storage.Provider interface every storage type
//     implements, and one subpackage per storage type (storage/s3,
//     storage/sftp, storage/b2, storage/azure, storage/gcs,
//     storage/rclone, storage/webdav, storage/filesystem) — see
//     storage/provider.go's doc comment for why each gets its own package.
package kopia

import (
	"dackup/internal/backend/kopia/storage"
	"dackup/internal/backend/kopia/storage/azure"
	"dackup/internal/backend/kopia/storage/b2"
	"dackup/internal/backend/kopia/storage/filesystem"
	"dackup/internal/backend/kopia/storage/gcs"
	"dackup/internal/backend/kopia/storage/rclone"
	"dackup/internal/backend/kopia/storage/s3"
	"dackup/internal/backend/kopia/storage/sftp"
	"dackup/internal/backend/kopia/storage/webdav"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// Name is the backend identifier written to DackupConfig.Backend.
	Name = "kopia"

	// DefaultBin is used when Config.Bin is empty; resolved via PATH.
	DefaultBin = "kopia"

	// DefaultGlobalRepoName is used when Config.GlobalRepoName is empty.
	DefaultGlobalRepoName = "global"

	passwordEnvVar = "KOPIA_PASSWORD"

	// configFileSubdir holds one kopia config file per repository, so each
	// repository (and the global mirror) can be connected to independently
	// without one connection's state clobbering another's — unlike borg,
	// which takes a repository path directly on every invocation, kopia
	// requires an explicit "connect" that's tied to a config file. This
	// directory is always local (under DackupConfig.BackendDir), even when
	// the repository data itself lives in remote storage — kopia always
	// needs somewhere local to keep the config file it reads on every call.
	configFileSubdir = ".kopia-config"
)

// Config is this backend's own typed settings, decoded from
// DackupConfig.BackendSettings when Backend == Name. Repository storage
// itself lives under the top-level DackupConfig.BackendDir only when
// StorageType is filesystem.Name (or empty); for a remote StorageType,
// BackendDir instead holds the local kopia config files every storage type
// still needs — see AGENTS.md's "Backend interface" section.
type Config struct {
	// Bin is the path to the kopia binary. Empty means "kopia", resolved
	// via PATH.
	Bin string `json:"bin,omitempty"`

	// GlobalRepoName names the repository that receives a full snapshot of
	// everything staged, in addition to each container-group's own
	// repository.
	GlobalRepoName string `json:"global_repo_name,omitempty"`

	// EncryptedPassword is ciphertext produced by a shared.SecretStore,
	// never a plaintext password. Unlike borg, kopia repositories are
	// always encrypted (there is no "none" mode), so this is always
	// required. It's the repository's own encryption password, independent
	// of any credential the storage backend below needs to reach the
	// bucket/container/host it lives in.
	EncryptedPassword string `json:"encrypted_password,omitempty"`

	// Compression is applied as kopia's global compression policy on each
	// repository right after it's created/connected, e.g. "zstd". Empty
	// leaves kopia's own default in place.
	Compression string `json:"compression,omitempty"`

	// StorageType selects where repository data lives: one of
	// filesystem.Name (the default), s3.Name, sftp.Name, b2.Name,
	// azure.Name, gcs.Name, rclone.Name, or webdav.Name. Empty means
	// filesystem.Name.
	StorageType string `json:"storage_type,omitempty"`

	// Exactly one of these is read, matching StorageType. They're separate
	// typed structs (one per storage/<name> subpackage) rather than a
	// generic map — see AGENTS.md's "Backend interface" section on why a
	// generic settings map was rejected for backend selection; the same
	// reasoning applies one level down, to storage-type selection within
	// kopia.
	S3     *s3.Storage     `json:"s3,omitempty"`
	SFTP   *sftp.Storage   `json:"sftp,omitempty"`
	B2     *b2.Storage     `json:"b2,omitempty"`
	Azure  *azure.Storage  `json:"azure,omitempty"`
	GCS    *gcs.Storage    `json:"gcs,omitempty"`
	Rclone *rclone.Storage `json:"rclone,omitempty"`
	WebDAV *webdav.Storage `json:"webdav,omitempty"`
}

// DefaultConfig returns a Config with GlobalRepoName set to its default;
// every other field is left empty (StorageType empty means
// filesystem.Name, via Config.storageType).
func DefaultConfig() Config {
	return Config{
		GlobalRepoName: DefaultGlobalRepoName,
	}
}

// Validate reports whether config is well-formed. It does not check
// GlobalRepoName against actual container-group names — that requires the
// groups passed to BackupGroups/RestoreGroups, so it's checked there
// instead (see Backend.validateGroupNames).
func (config Config) Validate() error {
	if strings.TrimSpace(config.GlobalRepoName) == "" {
		return fmt.Errorf("kopia global_repo_name cannot be empty")
	}

	if strings.TrimSpace(config.EncryptedPassword) == "" {
		return fmt.Errorf("kopia requires encrypted_password to be set")
	}

	provider, err := config.provider("")
	if err != nil {
		return err
	}

	return provider.Validate()
}

// ParseConfig decodes raw backend_settings JSON into a Config, applying
// defaults for any field raw doesn't set, then validates the result.
func ParseConfig(raw json.RawMessage) (Config, error) {
	config := DefaultConfig()

	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &config); err != nil {
			return Config{}, fmt.Errorf("failed to parse kopia backend settings: %w", err)
		}
	}

	if err := config.Validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

func (config Config) bin() string {
	if strings.TrimSpace(config.Bin) != "" {
		return config.Bin
	}

	return DefaultBin
}

func (config Config) storageType() string {
	if strings.TrimSpace(config.StorageType) != "" {
		return config.StorageType
	}

	return filesystem.Name
}

// provider resolves Config's active storage type (StorageType, defaulting
// to filesystem.Name) into the storage.Provider that owns its validation
// and CLI-invocation logic — the one place Config switches on StorageType;
// see storage/provider.go's doc comment on storage.Provider for why.
// reposRoot is only meaningful for the filesystem provider — Config itself
// carries no notion of local storage paths, that's Backend.ReposRoot (see
// AGENTS.md's "Backend interface" section for why); Validate calls this
// with "" since reposRoot doesn't affect whether the config is well-formed.
func (config Config) provider(reposRoot string) (storage.Provider, error) {
	switch config.storageType() {
	case filesystem.Name:
		return filesystem.Storage{ReposRoot: reposRoot}, nil
	case s3.Name:
		if config.S3 == nil {
			return nil, fmt.Errorf("kopia storage_type %q requires an \"s3\" settings block", s3.Name)
		}
		return config.S3, nil
	case sftp.Name:
		if config.SFTP == nil {
			return nil, fmt.Errorf("kopia storage_type %q requires an \"sftp\" settings block", sftp.Name)
		}
		return config.SFTP, nil
	case b2.Name:
		if config.B2 == nil {
			return nil, fmt.Errorf("kopia storage_type %q requires a \"b2\" settings block", b2.Name)
		}
		return config.B2, nil
	case azure.Name:
		if config.Azure == nil {
			return nil, fmt.Errorf("kopia storage_type %q requires an \"azure\" settings block", azure.Name)
		}
		return config.Azure, nil
	case gcs.Name:
		if config.GCS == nil {
			return nil, fmt.Errorf("kopia storage_type %q requires a \"gcs\" settings block", gcs.Name)
		}
		return config.GCS, nil
	case rclone.Name:
		if config.Rclone == nil {
			return nil, fmt.Errorf("kopia storage_type %q requires a \"rclone\" settings block", rclone.Name)
		}
		return config.Rclone, nil
	case webdav.Name:
		if config.WebDAV == nil {
			return nil, fmt.Errorf("kopia storage_type %q requires a \"webdav\" settings block", webdav.Name)
		}
		return config.WebDAV, nil
	default:
		return nil, fmt.Errorf("unknown kopia storage_type %q", config.StorageType)
	}
}

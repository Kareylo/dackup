// Package restic implements backend.Backend and backend.GroupedBackend by
// driving the restic CLI. It only depends on internal/shared (not
// internal/backend) for the same reason internal/backend/borg and
// internal/backend/kopia do — see either package's doc comment.
//
// Restic's own CLI shape sits between borg's and kopia's: like borg (and
// unlike kopia), it's stateless per invocation — there's no persistent
// "connect" step, just a repository address given on every call — so
// GroupedBackend mirrors borg's one-repository-per-group model exactly
// (see backend.go), not kopia's one-snapshot-per-path model. Like kopia
// (and unlike borg), restic repositories are always encrypted — there is no
// "none" mode, so Config.EncryptedPassword is always required.
//
// The package is split one-file-per-responsibility, per AGENTS.md's "File
// Naming Guidelines":
//
//   - restic.go (this file) — Config, the top-level typed settings decoded
//     from DackupConfig.BackendSettings.
//   - backend.go — the Backend type implementing backend.Backend and
//     backend.GroupedBackend, and its dependency defaults.
//   - repository.go — the lower-level mechanics of driving the restic CLI
//     against one named repository (init/backup/restore).
//   - storage/ — the storage.Provider interface every storage type
//     implements, and one subpackage per storage type (storage/s3,
//     storage/sftp, storage/b2, storage/azure, storage/gcs, storage/rclone,
//     storage/rest, storage/swift, storage/filesystem).
package restic

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/backend/restic/storage/azure"
	"dackup/internal/backend/restic/storage/b2"
	"dackup/internal/backend/restic/storage/filesystem"
	"dackup/internal/backend/restic/storage/gcs"
	"dackup/internal/backend/restic/storage/rclone"
	"dackup/internal/backend/restic/storage/rest"
	"dackup/internal/backend/restic/storage/s3"
	"dackup/internal/backend/restic/storage/sftp"
	"dackup/internal/backend/restic/storage/swift"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// Name is the backend identifier written to DackupConfig.Backend.
	Name = "restic"

	// DefaultBin is used when Config.Bin is empty; resolved via PATH.
	DefaultBin = "restic"

	// DefaultGlobalRepoName is used when Config.GlobalRepoName is empty.
	DefaultGlobalRepoName = "global"

	passwordEnvVar = "RESTIC_PASSWORD"
)

// Config is this backend's own typed settings, decoded from
// DackupConfig.BackendSettings when Backend == Name. Repository storage
// itself lives under the top-level DackupConfig.BackendDir only when
// StorageType is filesystem.Name (or empty) — see AGENTS.md's "Backend
// interface" section.
type Config struct {
	// Bin is the path to the restic binary. Empty means "restic", resolved
	// via PATH.
	Bin string `json:"bin,omitempty"`

	// GlobalRepoName names the repository that receives a full snapshot of
	// everything staged, in addition to each container-group's own
	// repository.
	GlobalRepoName string `json:"global_repo_name,omitempty"`

	// EncryptedPassword is ciphertext produced by a shared.SecretStore,
	// never a plaintext password. Restic repositories are always
	// encrypted (there is no "none" mode, unlike borg), so this is always
	// required. It's the repository's own encryption password, independent
	// of any credential the storage backend below needs to reach the
	// bucket/container/host it lives in.
	EncryptedPassword string `json:"encrypted_password,omitempty"`

	// StorageType selects where repository data lives: one of
	// filesystem.Name (the default), s3.Name, sftp.Name, b2.Name,
	// azure.Name, gcs.Name, rclone.Name, rest.Name, or swift.Name. Empty
	// means filesystem.Name.
	StorageType string `json:"storage_type,omitempty"`

	// Exactly one of these is read, matching StorageType. They're separate
	// typed structs (one per storage/<name> subpackage) rather than a
	// generic map — see AGENTS.md's "Backend interface" section on why a
	// generic settings map was rejected for backend selection; the same
	// reasoning applies one level down, to storage-type selection within
	// restic.
	S3     *s3.Storage     `json:"s3,omitempty"`
	SFTP   *sftp.Storage   `json:"sftp,omitempty"`
	B2     *b2.Storage     `json:"b2,omitempty"`
	Azure  *azure.Storage  `json:"azure,omitempty"`
	GCS    *gcs.Storage    `json:"gcs,omitempty"`
	Rclone *rclone.Storage `json:"rclone,omitempty"`
	Rest   *rest.Storage   `json:"rest,omitempty"`
	Swift  *swift.Storage  `json:"swift,omitempty"`
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
		return fmt.Errorf("restic global_repo_name cannot be empty")
	}

	if strings.TrimSpace(config.EncryptedPassword) == "" {
		return fmt.Errorf("restic requires encrypted_password to be set")
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
			return Config{}, fmt.Errorf("failed to parse restic backend settings: %w", err)
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
// and CLI-invocation logic — the one place Config switches on StorageType,
// mirroring kopia.Config.provider. reposRoot is only meaningful for the
// filesystem provider; Validate calls this with "" since reposRoot doesn't
// affect whether the config is well-formed.
func (config Config) provider(reposRoot string) (storage.Provider, error) {
	switch config.storageType() {
	case filesystem.Name:
		return filesystem.Storage{ReposRoot: reposRoot}, nil
	case s3.Name:
		if config.S3 == nil {
			return nil, fmt.Errorf("restic storage_type %q requires an \"s3\" settings block", s3.Name)
		}
		return config.S3, nil
	case sftp.Name:
		if config.SFTP == nil {
			return nil, fmt.Errorf("restic storage_type %q requires an \"sftp\" settings block", sftp.Name)
		}
		return config.SFTP, nil
	case b2.Name:
		if config.B2 == nil {
			return nil, fmt.Errorf("restic storage_type %q requires a \"b2\" settings block", b2.Name)
		}
		return config.B2, nil
	case azure.Name:
		if config.Azure == nil {
			return nil, fmt.Errorf("restic storage_type %q requires an \"azure\" settings block", azure.Name)
		}
		return config.Azure, nil
	case gcs.Name:
		if config.GCS == nil {
			return nil, fmt.Errorf("restic storage_type %q requires a \"gcs\" settings block", gcs.Name)
		}
		return config.GCS, nil
	case rclone.Name:
		if config.Rclone == nil {
			return nil, fmt.Errorf("restic storage_type %q requires a \"rclone\" settings block", rclone.Name)
		}
		return config.Rclone, nil
	case rest.Name:
		if config.Rest == nil {
			return nil, fmt.Errorf("restic storage_type %q requires a \"rest\" settings block", rest.Name)
		}
		return config.Rest, nil
	case swift.Name:
		if config.Swift == nil {
			return nil, fmt.Errorf("restic storage_type %q requires a \"swift\" settings block", swift.Name)
		}
		return config.Swift, nil
	default:
		return nil, fmt.Errorf("unknown restic storage_type %q", config.StorageType)
	}
}

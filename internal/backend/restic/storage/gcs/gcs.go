// Package gcs implements storage.Provider for restic repositories stored in
// a Google Cloud Storage bucket.
package gcs

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/shared"
	"fmt"
	"strings"
)

// Name is the identifier written to restic.Config.StorageType.
const Name = "gcs"

// Storage configures a Google Cloud Storage bucket as repository storage.
type Storage struct {
	Bucket string `json:"bucket,omitempty"`
	Prefix string `json:"prefix,omitempty"`

	// ProjectID is sent via GOOGLE_PROJECT_ID. Not itself a secret.
	ProjectID string `json:"project_id,omitempty"`

	// CredentialsFilePath is a path (on the machine running dackup) to a
	// GCS service account JSON key file, sent via
	// GOOGLE_APPLICATION_CREDENTIALS. Not itself a secret dackup manages —
	// same reasoning as sftp.Storage.KeyfilePath. Required unless
	// EmulatorHost is set — a local emulator typically accepts
	// unauthenticated requests.
	CredentialsFilePath string `json:"credentials_file_path,omitempty"`

	// EmulatorHost points restic at a local GCS emulator (e.g.
	// fsouza/fake-gcs-server) instead of real Google Cloud Storage, via the
	// STORAGE_EMULATOR_HOST environment variable — the same convention
	// kopia's own gcs storage type uses (see its doc comment). Restic's GCS
	// backend is built on the same Google Cloud Storage Go client library,
	// so this is a best-effort bet on that library honoring the same
	// variable, matching kopia's — unverified without a live emulator to
	// test against.
	EmulatorHost string `json:"emulator_host,omitempty"`
}

// Validate reports whether the GCS settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.Bucket) == "" {
		return fmt.Errorf("restic gcs storage requires bucket")
	}

	if strings.TrimSpace(s.CredentialsFilePath) == "" && strings.TrimSpace(s.EmulatorHost) == "" {
		return fmt.Errorf("restic gcs storage requires credentials_file_path unless emulator_host is set")
	}

	return nil
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	repository := fmt.Sprintf("gs:%s:/%s", s.Bucket, storage.RepoPath(s.Prefix, repoName))

	var env []string
	if s.ProjectID != "" {
		env = append(env, "GOOGLE_PROJECT_ID="+s.ProjectID)
	}
	if s.CredentialsFilePath != "" {
		env = append(env, "GOOGLE_APPLICATION_CREDENTIALS="+s.CredentialsFilePath)
	}
	if s.EmulatorHost != "" {
		env = append(env, "STORAGE_EMULATOR_HOST="+s.EmulatorHost)
	}

	return storage.Invocation{Repository: repository, Env: env}, nil
}

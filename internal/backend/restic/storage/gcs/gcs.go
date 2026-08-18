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
	// kopia's own gcs storage type uses (see its doc comment). Confirmed
	// against restic 0.19.1's internal/backend/gs source: restic's own
	// getStorageClient() unconditionally calls
	// google.DefaultTokenSource(ctx, storage.ScopeReadWrite) — and fails
	// with "could not find default credentials" — unless GOOGLE_ACCESS_TOKEN
	// is set, so BuildInvocation also sends a dummy GOOGLE_ACCESS_TOKEN
	// whenever EmulatorHost is set (see dummyEmulatorAccessToken below).
	// Only once that token short-circuits restic past its own credential
	// lookup does execution reach cloud.google.com/go/storage's
	// storage.NewClient, which is the thing that actually honors
	// STORAGE_EMULATOR_HOST (skipping auth and redirecting to the emulator
	// endpoint) — restic's own gs.go never references that variable itself.
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
		env = append(env, "STORAGE_EMULATOR_HOST="+s.EmulatorHost, "GOOGLE_ACCESS_TOKEN="+dummyEmulatorAccessToken)
	}

	return storage.Invocation{Repository: repository, Env: env}, nil
}

// dummyEmulatorAccessToken is sent as GOOGLE_ACCESS_TOKEN whenever
// EmulatorHost is set. It isn't a real credential — local GCS emulators
// like fake-gcs-server don't validate it — it only exists to make restic
// skip its own google.DefaultTokenSource lookup (see EmulatorHost's doc
// comment).
const dummyEmulatorAccessToken = "dackup-gcs-emulator-dummy-token"

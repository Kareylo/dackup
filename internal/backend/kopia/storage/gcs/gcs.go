// Package gcs implements storage.Provider for kopia repositories stored in
// a Google Cloud Storage bucket.
package gcs

import (
	"dackup/internal/backend/kopia/storage"
	"dackup/internal/shared"
	"fmt"
	"strings"
)

// Name is the identifier written to kopia.Config.StorageType (and kopia's
// own "repository create/connect gcs" subcommand).
const Name = "gcs"

// Storage configures a Google Cloud Storage bucket as repository storage.
type Storage struct {
	Bucket string `json:"bucket,omitempty"`
	Prefix string `json:"prefix,omitempty"`

	// CredentialsFilePath is a path (on the machine running dackup) to a
	// GCS service account JSON key file. Not itself a secret dackup
	// manages — same reasoning as sftp.Storage.KeyfilePath. Required
	// unless EmulatorHost is set — a local emulator typically accepts
	// unauthenticated requests.
	CredentialsFilePath string `json:"credentials_file_path,omitempty"`

	// EmulatorHost points kopia at a local GCS emulator (e.g.
	// fsouza/fake-gcs-server) instead of real Google Cloud Storage, via
	// the STORAGE_EMULATOR_HOST environment variable — the standard
	// convention Google's own Cloud Storage Go client libraries respect.
	// Unlike the other storage types' endpoint overrides, this isn't a
	// documented kopia flag (kopia's own --help lists no gcs endpoint
	// option), so it's a best-effort bet on kopia's GCS backend using the
	// standard client under the hood — unverified without a live emulator
	// to test against.
	EmulatorHost string `json:"emulator_host,omitempty"`
}

// Validate reports whether the GCS settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.Bucket) == "" {
		return fmt.Errorf("kopia gcs storage requires bucket")
	}

	if strings.TrimSpace(s.CredentialsFilePath) == "" && strings.TrimSpace(s.EmulatorHost) == "" {
		return fmt.Errorf("kopia gcs storage requires credentials_file_path unless emulator_host is set")
	}

	return nil
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	args := []string{"--bucket=" + s.Bucket}

	if s.CredentialsFilePath != "" {
		args = append(args, "--credentials-file="+s.CredentialsFilePath)
	}

	args = append(args, "--prefix="+storage.ObjectPrefix(s.Prefix, repoName))

	var env []string
	if s.EmulatorHost != "" {
		env = []string{"STORAGE_EMULATOR_HOST=" + s.EmulatorHost}
	}

	return storage.Invocation{Kind: Name, Args: args, Env: env}, nil
}

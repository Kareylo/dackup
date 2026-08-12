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
	// manages — same reasoning as sftp.Storage.KeyfilePath.
	CredentialsFilePath string `json:"credentials_file_path,omitempty"`
}

// Validate reports whether the GCS settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.Bucket) == "" {
		return fmt.Errorf("kopia gcs storage requires bucket")
	}

	if strings.TrimSpace(s.CredentialsFilePath) == "" {
		return fmt.Errorf("kopia gcs storage requires credentials_file_path")
	}

	return nil
}

func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	args := []string{
		"--bucket=" + s.Bucket,
		"--credentials-file=" + s.CredentialsFilePath,
		"--prefix=" + storage.ObjectPrefix(s.Prefix, repoName),
	}

	return storage.Invocation{Kind: Name, Args: args}, nil
}

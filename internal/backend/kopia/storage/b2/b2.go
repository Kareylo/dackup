// Package b2 implements storage.Provider for kopia repositories stored in
// a Backblaze B2 bucket, via B2's native API (not its S3-compatible one —
// see package s3 for that).
package b2

import (
	"dackup/internal/backend/kopia/storage"
	"dackup/internal/shared"
	"fmt"
	"strings"
)

// Name is the identifier written to kopia.Config.StorageType (and kopia's
// own "repository create/connect b2" subcommand).
const Name = "b2"

// Storage configures a Backblaze B2 bucket as repository storage.
type Storage struct {
	Bucket string `json:"bucket,omitempty"`
	Prefix string `json:"prefix,omitempty"`

	// KeyID is not a secret on its own, so it's stored as plain text.
	KeyID string `json:"key_id,omitempty"`

	// EncryptedApplicationKey is ciphertext produced by a
	// shared.SecretStore.
	EncryptedApplicationKey string `json:"encrypted_application_key,omitempty"`
}

// Validate reports whether the B2 settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.Bucket) == "" {
		return fmt.Errorf("kopia b2 storage requires bucket")
	}

	if strings.TrimSpace(s.KeyID) == "" {
		return fmt.Errorf("kopia b2 storage requires key_id")
	}

	if strings.TrimSpace(s.EncryptedApplicationKey) == "" {
		return fmt.Errorf("kopia b2 storage requires encrypted_application_key")
	}

	return nil
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	applicationKey, err := secrets.Decrypt(s.EncryptedApplicationKey)
	if err != nil {
		return storage.Invocation{}, fmt.Errorf("failed to decrypt kopia b2 application key: %w", err)
	}

	args := []string{
		"--bucket=" + s.Bucket,
		"--key-id=" + s.KeyID,
		"--key=" + applicationKey,
		"--prefix=" + storage.ObjectPrefix(s.Prefix, repoName),
	}

	return storage.Invocation{Kind: Name, Args: args}, nil
}

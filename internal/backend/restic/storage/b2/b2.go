// Package b2 implements storage.Provider for restic repositories stored in
// a Backblaze B2 bucket, via B2's native API (not its S3-compatible one —
// see package s3 for that).
package b2

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/shared"
	"fmt"
	"strings"
)

// Name is the identifier written to restic.Config.StorageType.
const Name = "b2"

// Storage configures a Backblaze B2 bucket as repository storage.
type Storage struct {
	Bucket string `json:"bucket,omitempty"`
	Prefix string `json:"prefix,omitempty"`

	// AccountID is not a secret on its own, so it's stored as plain text.
	AccountID string `json:"account_id,omitempty"`

	// EncryptedAccountKey is ciphertext produced by a shared.SecretStore.
	EncryptedAccountKey string `json:"encrypted_account_key,omitempty"`
}

// Validate reports whether the B2 settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.Bucket) == "" {
		return fmt.Errorf("restic b2 storage requires bucket")
	}

	if strings.TrimSpace(s.AccountID) == "" {
		return fmt.Errorf("restic b2 storage requires account_id")
	}

	if strings.TrimSpace(s.EncryptedAccountKey) == "" {
		return fmt.Errorf("restic b2 storage requires encrypted_account_key")
	}

	return nil
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	accountKey, err := secrets.Decrypt(s.EncryptedAccountKey)
	if err != nil {
		return storage.Invocation{}, fmt.Errorf("failed to decrypt restic b2 account key: %w", err)
	}

	repository := fmt.Sprintf("b2:%s:%s", s.Bucket, storage.RepoPath(s.Prefix, repoName))

	env := []string{"B2_ACCOUNT_ID=" + s.AccountID, "B2_ACCOUNT_KEY=" + accountKey}

	return storage.Invocation{Repository: repository, Env: env}, nil
}

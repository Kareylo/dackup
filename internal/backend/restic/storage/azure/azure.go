// Package azure implements storage.Provider for restic repositories stored
// in an Azure Blob Storage container.
package azure

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/shared"
	"fmt"
	"strings"
)

// Name is the identifier written to restic.Config.StorageType.
const Name = "azure"

// Storage configures an Azure Blob Storage container as repository storage.
type Storage struct {
	Container string `json:"container,omitempty"`
	Prefix    string `json:"prefix,omitempty"`

	// AccountName is not a secret on its own, so it's stored as plain text.
	AccountName string `json:"account_name,omitempty"`

	// EncryptedAccountKey is ciphertext produced by a shared.SecretStore.
	EncryptedAccountKey string `json:"encrypted_account_key,omitempty"`
}

// Validate reports whether the Azure settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.Container) == "" {
		return fmt.Errorf("restic azure storage requires container")
	}

	if strings.TrimSpace(s.AccountName) == "" {
		return fmt.Errorf("restic azure storage requires account_name")
	}

	if strings.TrimSpace(s.EncryptedAccountKey) == "" {
		return fmt.Errorf("restic azure storage requires encrypted_account_key")
	}

	return nil
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	accountKey, err := secrets.Decrypt(s.EncryptedAccountKey)
	if err != nil {
		return storage.Invocation{}, fmt.Errorf("failed to decrypt restic azure account key: %w", err)
	}

	repository := fmt.Sprintf("azure:%s:/%s", s.Container, storage.RepoPath(s.Prefix, repoName))

	env := []string{"AZURE_ACCOUNT_NAME=" + s.AccountName, "AZURE_ACCOUNT_KEY=" + accountKey}

	return storage.Invocation{Repository: repository, Env: env}, nil
}

// Package azure implements storage.Provider for kopia repositories stored
// in an Azure Blob Storage container.
package azure

import (
	"dackup/internal/backend/kopia/storage"
	"dackup/internal/shared"
	"fmt"
	"strings"
)

// Name is the identifier written to kopia.Config.StorageType (and kopia's
// own "repository create/connect azure" subcommand).
const Name = "azure"

// Storage configures an Azure Blob Storage container as repository
// storage.
type Storage struct {
	Container      string `json:"container,omitempty"`
	StorageAccount string `json:"storage_account,omitempty"`
	Prefix         string `json:"prefix,omitempty"`

	// EncryptedStorageKey is ciphertext produced by a shared.SecretStore.
	EncryptedStorageKey string `json:"encrypted_storage_key,omitempty"`
}

// Validate reports whether the Azure settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.Container) == "" {
		return fmt.Errorf("kopia azure storage requires container")
	}

	if strings.TrimSpace(s.StorageAccount) == "" {
		return fmt.Errorf("kopia azure storage requires storage_account")
	}

	if strings.TrimSpace(s.EncryptedStorageKey) == "" {
		return fmt.Errorf("kopia azure storage requires encrypted_storage_key")
	}

	return nil
}

func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	storageKey, err := secrets.Decrypt(s.EncryptedStorageKey)
	if err != nil {
		return storage.Invocation{}, fmt.Errorf("failed to decrypt kopia azure storage key: %w", err)
	}

	args := []string{
		"--container=" + s.Container,
		"--storage-account=" + s.StorageAccount,
		"--storage-key=" + storageKey,
		"--prefix=" + storage.ObjectPrefix(s.Prefix, repoName),
	}

	return storage.Invocation{Kind: Name, Args: args}, nil
}

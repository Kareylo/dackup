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

	// StorageDomain overrides the default blob.core.windows.net domain,
	// via kopia's --storage-domain flag. Verified against kopia's own
	// --help output (real Azure needs no override; a private or emulated
	// endpoint does). Note that this alone may not be sufficient for
	// Azurite specifically: Azurite serves path-style URLs
	// (host:port/account/container) rather than the virtual-hosted-style
	// (account.host/container) a domain override implies, so kopia's
	// Azure client may not reach it correctly even with this set —
	// unverified without a live Azurite instance to test against.
	StorageDomain string `json:"storage_domain,omitempty"`
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

// BuildInvocation implements storage.Provider.
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

	if s.StorageDomain != "" {
		args = append(args, "--storage-domain="+s.StorageDomain)
	}

	return storage.Invocation{Kind: Name, Args: args}, nil
}

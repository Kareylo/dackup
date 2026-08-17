// Package swift implements storage.Provider for restic repositories stored
// in an OpenStack Swift container. Restic supports Swift natively; kopia
// (the other backend in this codebase) does not, so this storage type has
// no kopia counterpart to mirror.
package swift

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/shared"
	"fmt"
	"strings"
)

// Name is the identifier written to restic.Config.StorageType.
const Name = "swift"

// Storage configures an OpenStack Swift container as repository storage,
// using Keystone v2/v3 username+password authentication — the common case.
// Restic's Swift backend also supports application-credential and
// pre-authenticated-token auth (via OS_APPLICATION_CREDENTIAL_ID/SECRET or
// OS_STORAGE_URL/OS_AUTH_TOKEN respectively), which this type deliberately
// doesn't cover — out of scope for dackup's initial Swift support.
type Storage struct {
	Container string `json:"container,omitempty"`
	Prefix    string `json:"prefix,omitempty"`

	// AuthURL is the Keystone identity endpoint, sent via OS_AUTH_URL.
	AuthURL string `json:"auth_url,omitempty"`

	// Username is not a secret on its own, so it's stored as plain text.
	Username string `json:"username,omitempty"`

	// EncryptedPassword is ciphertext produced by a shared.SecretStore.
	EncryptedPassword string `json:"encrypted_password,omitempty"`

	// TenantName selects the Keystone tenant/project, sent via
	// OS_TENANT_NAME. Optional — not every Swift deployment requires it.
	TenantName string `json:"tenant_name,omitempty"`

	// RegionName selects a specific Swift region, sent via OS_REGION_NAME.
	// Optional.
	RegionName string `json:"region_name,omitempty"`
}

// Validate reports whether the Swift settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.Container) == "" {
		return fmt.Errorf("restic swift storage requires container")
	}

	if strings.TrimSpace(s.AuthURL) == "" {
		return fmt.Errorf("restic swift storage requires auth_url")
	}

	if strings.TrimSpace(s.Username) == "" {
		return fmt.Errorf("restic swift storage requires username")
	}

	if strings.TrimSpace(s.EncryptedPassword) == "" {
		return fmt.Errorf("restic swift storage requires encrypted_password")
	}

	return nil
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	password, err := secrets.Decrypt(s.EncryptedPassword)
	if err != nil {
		return storage.Invocation{}, fmt.Errorf("failed to decrypt restic swift password: %w", err)
	}

	repository := fmt.Sprintf("swift:%s:/%s", s.Container, storage.RepoPath(s.Prefix, repoName))

	env := []string{"OS_AUTH_URL=" + s.AuthURL, "OS_USERNAME=" + s.Username, "OS_PASSWORD=" + password}
	if s.TenantName != "" {
		env = append(env, "OS_TENANT_NAME="+s.TenantName)
	}
	if s.RegionName != "" {
		env = append(env, "OS_REGION_NAME="+s.RegionName)
	}

	return storage.Invocation{Repository: repository, Env: env}, nil
}

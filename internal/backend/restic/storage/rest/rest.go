// Package rest implements storage.Provider for restic repositories stored
// on a REST server (restic's own dedicated server protocol, run via the
// restic/rest-server project) — restic's closest analogue to kopia's WebDAV
// storage type, which restic itself doesn't support natively.
package rest

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/shared"
	"fmt"
	"strings"
)

// Name is the identifier written to restic.Config.StorageType.
const Name = "rest"

// Storage configures a REST server as repository storage.
type Storage struct {
	// URL is the base REST server URL (including scheme), e.g.
	// "https://backup.example.com:8000"; each repository gets its own path
	// segment appended under it.
	URL string `json:"url,omitempty"`

	// Username is not a secret on its own, so it's stored as plain text.
	// Some REST servers run with authentication disabled, so this (and
	// EncryptedPassword) may both be left empty — but if one is set, the
	// other must be too.
	Username string `json:"username,omitempty"`

	// EncryptedPassword is ciphertext produced by a shared.SecretStore.
	// Sent via RESTIC_REST_USERNAME/RESTIC_REST_PASSWORD (confirmed
	// against the restic/rest-server project's own docs) rather than
	// embedded in the repository URL, avoiding the argv exposure a
	// credential-in-URL approach would have.
	EncryptedPassword string `json:"encrypted_password,omitempty"`
}

// Validate reports whether the REST settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.URL) == "" {
		return fmt.Errorf("restic rest storage requires url")
	}

	hasUsername := strings.TrimSpace(s.Username) != ""
	hasPassword := strings.TrimSpace(s.EncryptedPassword) != ""

	if hasUsername != hasPassword {
		return fmt.Errorf("restic rest storage requires username and encrypted_password to be set together")
	}

	return nil
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	repository := "rest:" + urlJoin(s.URL, repoName)

	var env []string
	if s.Username != "" {
		password, err := secrets.Decrypt(s.EncryptedPassword)
		if err != nil {
			return storage.Invocation{}, fmt.Errorf("failed to decrypt restic rest password: %w", err)
		}
		env = []string{"RESTIC_REST_USERNAME=" + s.Username, "RESTIC_REST_PASSWORD=" + password}
	}

	return storage.Invocation{Repository: repository, Env: env}, nil
}

// urlJoin appends repoName as a path segment onto a base URL. Unlike
// storage.RepoPath, this can't use "path".Join — that would collapse the
// "//" right after a URL's scheme, the same reason
// internal/backend/kopia/storage/webdav has its own copy of this helper.
// Local to this package since rest is the only restic storage type
// addressing a URL rather than a directory or bucket path.
func urlJoin(base string, repoName string) string {
	return strings.TrimSuffix(base, "/") + "/" + repoName
}

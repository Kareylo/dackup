// Package webdav implements storage.Provider for kopia repositories stored
// on a WebDAV server.
package webdav

import (
	"dackup/internal/backend/kopia/storage"
	"dackup/internal/shared"
	"fmt"
	"net/http"
	"strings"
)

// Name is the identifier written to kopia.Config.StorageType (and kopia's
// own "repository create/connect webdav" subcommand).
const Name = "webdav"

// Storage configures a WebDAV server as repository storage.
type Storage struct {
	// URL is the base WebDAV URL; each repository gets its own path
	// segment appended under it.
	URL string `json:"url,omitempty"`

	// Username is not a secret on its own, so it's stored as plain text.
	// Some WebDAV servers are unauthenticated, so this (and
	// EncryptedPassword) may both be left empty — but if one is set, the
	// other must be too.
	Username string `json:"username,omitempty"`

	// EncryptedPassword is ciphertext produced by a shared.SecretStore.
	EncryptedPassword string `json:"encrypted_password,omitempty"`
}

// Validate reports whether the WebDAV settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.URL) == "" {
		return fmt.Errorf("kopia webdav storage requires url")
	}

	hasUsername := strings.TrimSpace(s.Username) != ""
	hasPassword := strings.TrimSpace(s.EncryptedPassword) != ""

	if hasUsername != hasPassword {
		return fmt.Errorf("kopia webdav storage requires username and encrypted_password to be set together")
	}

	return nil
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	args := []string{"--url=" + urlJoin(s.URL, repoName)}

	if s.Username != "" {
		password, err := secrets.Decrypt(s.EncryptedPassword)
		if err != nil {
			return storage.Invocation{}, fmt.Errorf("failed to decrypt kopia webdav password: %w", err)
		}
		args = append(args, "--webdav-username="+s.Username, "--webdav-password="+password)
	}

	return storage.Invocation{Kind: Name, Args: args}, nil
}

// EnsureCollection creates repoName's WebDAV collection (directory) via
// MKCOL if it doesn't already exist. Unlike kopia's other storage
// providers, its WebDAV client never creates the target directory itself —
// confirmed by driving it against a real WebDAV server and finding no
// MKCOL request in the server's logs, just PUT attempts straight into a
// directory that doesn't exist yet, failing with a filesystem-level
// "no such file or directory" translated to an HTTP 403 — it assumes the
// URL it's given already points at an existing, empty collection. So
// Backend.createArgs calls this before "repository create webdav", the
// same way it MkdirAll's a local directory before "repository create
// filesystem" (see repository.go's createArgs doc comment). A 405 (Method
// Not Allowed — MKCOL's response for a collection that already exists) is
// not an error; every other non-2xx status is.
func (s Storage) EnsureCollection(repoName string, secrets shared.SecretStore) error {
	url := urlJoin(s.URL, repoName)

	req, err := http.NewRequest("MKCOL", url, nil)
	if err != nil {
		return fmt.Errorf("failed to build MKCOL request for %s: %w", url, err)
	}

	if s.Username != "" {
		password, err := secrets.Decrypt(s.EncryptedPassword)
		if err != nil {
			return fmt.Errorf("failed to decrypt kopia webdav password: %w", err)
		}
		req.SetBasicAuth(s.Username, password)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create WebDAV collection %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusMethodNotAllowed {
		return nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to create WebDAV collection %s: unexpected status %s", url, resp.Status)
	}

	return nil
}

// urlJoin appends repoName as a path segment onto a base URL. Unlike
// storage.ObjectPrefix, this can't use "path".Join: that would Clean the
// result and collapse the "//" right after a URL's scheme
// (path.Join("https://host", "repo") -> "https:/host/repo", which is
// broken), since path.Join assumes its input is a plain filesystem-style
// path, not a URL with a scheme. Local to this package since webdav is the
// only storage type addressing a URL rather than a directory or key
// prefix.
func urlJoin(base string, repoName string) string {
	return strings.TrimSuffix(base, "/") + "/" + repoName
}

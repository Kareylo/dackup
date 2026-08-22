// Package sftp implements storage.Provider for kopia repositories stored
// under a directory reached over SFTP.
package sftp

import (
	"dackup/internal/backend/kopia/storage"
	"dackup/internal/shared"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// Name is the identifier written to kopia.Config.StorageType (and kopia's
// own "repository create/connect sftp" subcommand).
const Name = "sftp"

// DefaultPort is used when Storage.Port is zero.
const DefaultPort = 22

// Storage configures a directory reached over SFTP as repository storage.
type Storage struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`

	// Path is the remote base directory; each repository gets its own
	// subdirectory under it.
	Path string `json:"path,omitempty"`

	// KeyfilePath is a path (on the machine running dackup) to an SSH
	// private key. Not itself a secret dackup manages — like borg's Bin or
	// gcs.Storage.CredentialsFilePath, it's a filesystem path the operator
	// is responsible for securing. Mutually exclusive with
	// EncryptedPassword; exactly one must be set.
	KeyfilePath string `json:"keyfile_path,omitempty"`

	// EncryptedPassword is ciphertext produced by a shared.SecretStore.
	// Mutually exclusive with KeyfilePath.
	EncryptedPassword string `json:"encrypted_password,omitempty"`

	// KnownHostsPath is an optional path to a known_hosts file for host key
	// verification. Empty leaves kopia's own default behavior in place.
	KnownHostsPath string `json:"known_hosts_path,omitempty"`
}

// Validate reports whether the SFTP settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.Host) == "" {
		return fmt.Errorf("kopia sftp storage requires host")
	}

	if strings.TrimSpace(s.Username) == "" {
		return fmt.Errorf("kopia sftp storage requires username")
	}

	if strings.TrimSpace(s.Path) == "" {
		return fmt.Errorf("kopia sftp storage requires path")
	}

	hasKeyfile := strings.TrimSpace(s.KeyfilePath) != ""
	hasPassword := strings.TrimSpace(s.EncryptedPassword) != ""

	if hasKeyfile == hasPassword {
		return fmt.Errorf("kopia sftp storage requires exactly one of keyfile_path or encrypted_password")
	}

	return nil
}

func (s Storage) port() int {
	if s.Port != 0 {
		return s.Port
	}

	return DefaultPort
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	args := []string{
		"--host=" + s.Host,
		"--port=" + strconv.Itoa(s.port()),
		"--username=" + s.Username,
		"--path=" + path.Join(s.Path, repoName),
	}

	if s.KnownHostsPath != "" {
		args = append(args, "--known-hosts="+s.KnownHostsPath)
	}

	if s.KeyfilePath != "" {
		args = append(args, "--keyfile="+s.KeyfilePath)
	} else {
		password, err := secrets.Decrypt(s.EncryptedPassword)
		if err != nil {
			return storage.Invocation{}, fmt.Errorf("failed to decrypt kopia sftp password: %w", err)
		}
		args = append(args, "--sftp-password="+password)
	}

	return storage.Invocation{Kind: Name, Args: args}, nil
}

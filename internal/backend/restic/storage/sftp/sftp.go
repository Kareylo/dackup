// Package sftp implements storage.Provider for restic repositories stored
// under a directory reached over SFTP.
package sftp

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/shared"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// Name is the identifier written to restic.Config.StorageType.
const Name = "sftp"

// DefaultPort is used when Storage.Port is zero.
const DefaultPort = 22

// Storage configures a directory reached over SFTP as repository storage.
// Unlike kopia's own sftp storage type, restic's sftp backend has no
// dackup-managed secret at all: restic shells out to the system ssh/sftp
// client for the connection, so authentication is whatever that client is
// already configured for (an SSH agent, ~/.ssh/config, or the KeyfilePath
// below) — restic itself accepts no password flag for this backend (see
// restic.Config's package doc comment / AGENTS.md's restic section).
type Storage struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`

	// Path is the remote base directory; each repository gets its own
	// subdirectory under it.
	Path string `json:"path,omitempty"`

	// KeyfilePath is a path (on the machine running dackup) to an SSH
	// private key, passed to ssh via -i. Not itself a secret dackup
	// manages — like gcs.Storage.CredentialsFilePath, it's a filesystem
	// path the operator is responsible for securing. Empty means "use
	// whatever ssh's own default identity/agent resolution finds".
	KeyfilePath string `json:"keyfile_path,omitempty"`

	// KnownHostsPath is an optional path to a known_hosts file, passed to
	// ssh via -o UserKnownHostsFile. Empty leaves ssh's own default
	// (~/.ssh/known_hosts, interactive host-key prompting) in place.
	KnownHostsPath string `json:"known_hosts_path,omitempty"`
}

// Validate reports whether the SFTP settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.Host) == "" {
		return fmt.Errorf("restic sftp storage requires host")
	}

	if strings.TrimSpace(s.Username) == "" {
		return fmt.Errorf("restic sftp storage requires username")
	}

	if strings.TrimSpace(s.Path) == "" {
		return fmt.Errorf("restic sftp storage requires path")
	}

	return nil
}

func (s Storage) port() int {
	if s.Port != 0 {
		return s.Port
	}

	return DefaultPort
}

// BuildInvocation implements storage.Provider. A non-default port, an
// explicit keyfile, or a known_hosts override is expressed via restic's
// documented "-o sftp.command=..." global option, which overrides the ssh
// command restic's sftp backend otherwise runs implicitly (plain
// "ssh user@host -s sftp") — restic's own repository URI form
// (sftp:user@host:/path) has no port or host-key-verification field of its
// own.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	repository := fmt.Sprintf("sftp:%s@%s:%s", s.Username, s.Host, path.Join(s.Path, repoName))

	var args []string
	if s.port() != DefaultPort || s.KeyfilePath != "" || s.KnownHostsPath != "" {
		sshCommand := []string{"ssh"}
		if s.KeyfilePath != "" {
			sshCommand = append(sshCommand, "-i", s.KeyfilePath)
		}
		if s.KnownHostsPath != "" {
			sshCommand = append(sshCommand, "-o", "UserKnownHostsFile="+s.KnownHostsPath)
		}
		sshCommand = append(sshCommand, s.Username+"@"+s.Host, "-p", strconv.Itoa(s.port()), "-s", "sftp")

		args = []string{"-o", "sftp.command=" + strings.Join(sshCommand, " ")}
	}

	return storage.Invocation{Repository: repository, Args: args}, nil
}

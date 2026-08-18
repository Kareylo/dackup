// Package rclone implements storage.Provider for restic repositories stored
// in an rclone remote (as set up independently by the operator via
// `rclone config`). Restic reaches it by driving the rclone binary itself,
// so this covers any of the many storage services rclone supports without
// dackup needing a dedicated storage type for each one — mirrors
// internal/backend/kopia/storage/rclone's role for kopia.
package rclone

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/shared"
	"fmt"
	"path"
	"strings"
)

// Name is the identifier written to restic.Config.StorageType.
const Name = "rclone"

// Storage configures an rclone remote as repository storage.
type Storage struct {
	// RemoteName is the rclone remote's name, exactly as it appears in the
	// operator's rclone.conf (e.g. "b2remote", "gdrive"). Restic addresses
	// it as "rclone:<RemoteName>:<path>".
	RemoteName string `json:"remote_name,omitempty"`

	// RemotePath is the base path within the remote; each repository gets
	// its own subdirectory under it.
	RemotePath string `json:"remote_path,omitempty"`

	// RcloneExePath is a path (on the machine running dackup) to the
	// rclone binary, passed via restic's "-o rclone.program=" option. Empty
	// means "rclone", resolved via PATH. Not itself a secret — same
	// reasoning as sftp.Storage.KeyfilePath.
	RcloneExePath string `json:"rclone_exe_path,omitempty"`

	// ConfigFilePath is an optional path to a non-default rclone.conf, sent
	// via the RCLONE_CONFIG environment variable — rclone's own convention.
	// Not a secret dackup manages — rclone's own credentials for
	// RemoteName live inside this file, which the operator is responsible
	// for securing, the same way dackup never looks inside
	// sftp.Storage.KeyfilePath or gcs.Storage.CredentialsFilePath either.
	ConfigFilePath string `json:"config_file_path,omitempty"`
}

// Validate reports whether the rclone settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.RemoteName) == "" {
		return fmt.Errorf("restic rclone storage requires remote_name")
	}

	return nil
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	repository := fmt.Sprintf("rclone:%s:%s", s.RemoteName, path.Join(s.RemotePath, repoName))

	var args []string
	if s.RcloneExePath != "" {
		args = []string{"-o", "rclone.program=" + s.RcloneExePath}
	}

	var env []string
	if s.ConfigFilePath != "" {
		env = []string{"RCLONE_CONFIG=" + s.ConfigFilePath}
	}

	return storage.Invocation{Repository: repository, Env: env, Args: args}, nil
}

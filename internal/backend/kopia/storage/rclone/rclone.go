// Package rclone implements storage.Provider for kopia repositories stored
// in an rclone remote (as set up independently by the operator via
// `rclone config`). Kopia reaches it by driving the rclone binary itself,
// so this covers any of the many storage services rclone supports without
// dackup needing a dedicated storage type for each one.
package rclone

import (
	"dackup/internal/backend/kopia/storage"
	"dackup/internal/shared"
	"fmt"
	"path"
	"strings"
)

// Name is the identifier written to kopia.Config.StorageType (and kopia's
// own "repository create/connect rclone" subcommand).
const Name = "rclone"

// Storage configures an rclone remote as repository storage.
type Storage struct {
	// RemoteName is the rclone remote's name, exactly as it appears in the
	// operator's rclone.conf (e.g. "b2remote", "gdrive"). Kopia addresses
	// it as "<RemoteName>:<path>".
	RemoteName string `json:"remote_name,omitempty"`

	// RemotePath is the base path within the remote; each repository gets
	// its own subdirectory under it.
	RemotePath string `json:"remote_path,omitempty"`

	// RcloneExePath is a path (on the machine running dackup) to the
	// rclone binary. Empty means "rclone", resolved via PATH. Not itself a
	// secret — same reasoning as sftp.Storage.KeyfilePath.
	RcloneExePath string `json:"rclone_exe_path,omitempty"`

	// ConfigFilePath is an optional path to a non-default rclone.conf. Not
	// a secret dackup manages — rclone's own credentials for RemoteName
	// live inside this file, which the operator is responsible for
	// securing, the same way dackup never looks inside
	// sftp.Storage.KeyfilePath or gcs.Storage.CredentialsFilePath either.
	ConfigFilePath string `json:"config_file_path,omitempty"`
}

// Validate reports whether the rclone settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.RemoteName) == "" {
		return fmt.Errorf("kopia rclone storage requires remote_name")
	}

	return nil
}

func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	args := []string{"--remote-path=" + s.RemoteName + ":" + path.Join(s.RemotePath, repoName)}
	if s.RcloneExePath != "" {
		args = append(args, "--rclone-exe="+s.RcloneExePath)
	}

	var env []string
	if s.ConfigFilePath != "" {
		env = []string{"RCLONE_CONFIG=" + s.ConfigFilePath}
	}

	return storage.Invocation{Kind: Name, Args: args, Env: env}, nil
}

package kopia

import (
	"dackup/internal/shared"
	"fmt"
)

// Backend drives the kopia CLI to implement backend.Backend and
// backend.GroupedBackend. ReposRoot is the top-level DackupConfig.BackendDir
// — always used for each repository's local kopia config file, and (only
// when Config's storage type is StorageFilesystem) for the repository data
// itself, at filepath.Join(ReposRoot, group.Name).
type Backend struct {
	Config    Config
	ReposRoot string
	Runner    shared.CommandRunner
	Logger    shared.Logger
	Options   *shared.Options
	Secrets   shared.SecretStore
	FS        shared.FileSystem
}

// Name returns the backend identifier "kopia".
func (backend Backend) Name() string {
	return Name
}

// BinaryName returns the kopia binary this Backend will invoke, resolved
// from Config.Bin (falling back to DefaultBin) — implements the optional
// backend.BinaryChecker interface so preflight can verify it's on PATH.
func (backend Backend) BinaryName() string {
	return backend.Config.bin()
}

// Backup implements the plain backend.Backend fallback used when a caller
// doesn't know about container groups: stagingDir itself is snapshotted as
// a single source into the global repository.
func (backend Backend) Backup(stagingDir string) error {
	return backend.snapshotRepo(stagingDir, backend.Config.GlobalRepoName, []string{"."})
}

// Restore implements the plain backend.Backend fallback, restoring the
// latest snapshot of stagingDir from the global repository.
func (backend Backend) Restore(stagingDir string) error {
	return backend.restoreRepo(stagingDir, backend.Config.GlobalRepoName, []string{"."})
}

// BackupGroups snapshots each group's paths (one kopia snapshot per path)
// into its own repository, then snapshots stagingDir as a whole into the
// global repository too. A group with no configured paths is skipped (its
// repository is left untouched) rather than snapshotting the whole
// stagingDir under its name.
func (backend Backend) BackupGroups(stagingDir string, groups []shared.BackendGroup) error {
	if err := backend.validateGroupNames(groups); err != nil {
		return err
	}

	for _, group := range groups {
		if len(group.Paths) == 0 {
			backend.log("INFO", fmt.Sprintf("No paths configured for group %s; skipping its kopia repository", group.Name))
			continue
		}

		if err := backend.snapshotRepo(stagingDir, group.Name, group.Paths); err != nil {
			return err
		}
	}

	return backend.snapshotRepo(stagingDir, backend.Config.GlobalRepoName, []string{"."})
}

// RestoreGroups restores the latest snapshot of each of a group's paths
// from that group's own repository into stagingDir. The global repository
// is left alone — restoring from it is a separate, not-yet-implemented
// operation, since it would restore everything rather than matching
// backup's per-group granularity.
func (backend Backend) RestoreGroups(stagingDir string, groups []shared.BackendGroup) error {
	if err := backend.validateGroupNames(groups); err != nil {
		return err
	}

	for _, group := range groups {
		if err := backend.restoreRepo(stagingDir, group.Name, group.Paths); err != nil {
			return err
		}
	}

	return nil
}

func (backend Backend) validateGroupNames(groups []shared.BackendGroup) error {
	for _, group := range groups {
		if group.Name == backend.Config.GlobalRepoName {
			return fmt.Errorf("container group %q collides with the configured global_repo_name; rename one of them", group.Name)
		}
	}

	return nil
}

// The methods below resolve Backend's optional dependency fields
// (Runner/FS/Secrets) to their real implementations when unset, the same
// pattern borg.Backend uses — see internal/backend/borg/borg.go.

func (backend Backend) envRunner() (shared.EnvCommandRunner, error) {
	envRunner, ok := backend.runner().(shared.EnvCommandRunner)
	if !ok {
		return nil, fmt.Errorf("command runner does not support the environment variables kopia requires")
	}

	return envRunner, nil
}

func (backend Backend) fileSystem() shared.FileSystem {
	if backend.FS != nil {
		return backend.FS
	}

	return shared.OSFileSystem{}
}

func (backend Backend) runner() shared.CommandRunner {
	if backend.Runner != nil {
		return backend.Runner
	}

	return shared.OSCommandRunner{}
}

func (backend Backend) secretStore() shared.SecretStore {
	if backend.Secrets != nil {
		return backend.Secrets
	}

	return shared.AESFileSecretStore{}
}

func (backend Backend) dryRun() bool {
	return backend.Options != nil && backend.Options.DryRun
}

func (backend Backend) log(level string, message string) {
	if backend.Logger == nil {
		return
	}

	backend.Logger.Log(level, message)
}

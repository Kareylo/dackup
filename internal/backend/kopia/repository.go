package kopia

import (
	"dackup/internal/backend/kopia/storage"
	"dackup/internal/backend/kopia/storage/filesystem"
	"dackup/internal/shared"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// snapshotRepo connects (or creates) the named repository, then creates one
// kopia snapshot per entry in relativePaths. Kopia snapshots a single
// directory per invocation — unlike borg's "create one archive from several
// top-level paths" — so a multi-path group becomes several snapshots in the
// same repository rather than one.
func (backend Backend) snapshotRepo(stagingDir string, repoName string, relativePaths []string) error {
	if backend.dryRun() {
		backend.log("INFO", fmt.Sprintf("[dry-run] Would create kopia snapshots for repository %q from %s (paths: %v)", repoName, stagingDir, relativePaths))
		return nil
	}

	envRunner, err := backend.envRunner()
	if err != nil {
		return err
	}

	passwordEnv, err := backend.passwordEnv()
	if err != nil {
		return err
	}

	configPath := backend.configPath(repoName)

	if err := backend.ensureRepo(envRunner, passwordEnv, repoName, configPath); err != nil {
		return err
	}

	for _, relativePath := range relativePaths {
		source := filepath.Join(stagingDir, relativePath)

		backend.log("INFO", fmt.Sprintf("Creating kopia snapshot of %s in repository %q", source, repoName))

		if err := envRunner.RunInDirWithEnv("", passwordEnv, backend.Config.bin(), "snapshot", "create", source, "--config-file="+configPath); err != nil {
			return fmt.Errorf("kopia snapshot create failed for %s: %w", source, err)
		}
	}

	return nil
}

// restoreRepo connects to the named repository (if it exists) and restores
// the latest snapshot of each entry in relativePaths back into its place
// under stagingDir. It never creates a repository — a restore against a
// repository that doesn't exist (or can't be reached) is a graceful no-op,
// not an error, since there's nothing to restore either way.
func (backend Backend) restoreRepo(stagingDir string, repoName string, relativePaths []string) error {
	if backend.dryRun() {
		backend.log("INFO", fmt.Sprintf("[dry-run] Would restore the latest kopia snapshots for repository %q into %s (paths: %v)", repoName, stagingDir, relativePaths))
		return nil
	}

	envRunner, err := backend.envRunner()
	if err != nil {
		return err
	}

	passwordEnv, err := backend.passwordEnv()
	if err != nil {
		return err
	}

	configPath := backend.configPath(repoName)

	if err := backend.fileSystem().MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return fmt.Errorf("failed to create kopia config directory: %w", err)
	}

	connectArgs, connectEnv, err := backend.connectArgs(repoName, configPath)
	if err != nil {
		return err
	}

	if err := envRunner.RunInDirWithEnv("", append(append([]string{}, passwordEnv...), connectEnv...), backend.Config.bin(), connectArgs...); err != nil {
		backend.log("WARN", fmt.Sprintf("Kopia repository %q does not exist yet (or could not be reached: %v); nothing to restore", repoName, err))
		return nil
	}

	for _, relativePath := range relativePaths {
		target := filepath.Join(stagingDir, relativePath)

		snapshotID, err := backend.latestSnapshotID(envRunner, passwordEnv, target, configPath)
		if err != nil {
			return err
		}

		if snapshotID == "" {
			backend.log("WARN", fmt.Sprintf("Kopia repository %q has no snapshots of %s yet; nothing to restore", repoName, target))
			continue
		}

		if err := backend.fileSystem().MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("failed to create restore target %s: %w", target, err)
		}

		backend.log("INFO", fmt.Sprintf("Restoring kopia snapshot %s into %s", snapshotID, target))

		if err := envRunner.RunInDirWithEnv("", passwordEnv, backend.Config.bin(), "snapshot", "restore", snapshotID, target, "--config-file="+configPath); err != nil {
			return fmt.Errorf("kopia snapshot restore failed for %s: %w", snapshotID, err)
		}
	}

	return nil
}

type kopiaSnapshotListEntry struct {
	ID string `json:"id"`
}

func (backend Backend) latestSnapshotID(envRunner shared.EnvCommandRunner, env []string, source string, configPath string) (string, error) {
	output, err := envRunner.OutputWithEnv(env, backend.Config.bin(), "snapshot", "list", source, "--json", "--config-file="+configPath)
	if err != nil {
		return "", fmt.Errorf("failed to list kopia snapshots of %s: %w", source, err)
	}

	var entries []kopiaSnapshotListEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return "", fmt.Errorf("failed to parse kopia snapshot list for %s: %w", source, err)
	}

	if len(entries) == 0 {
		return "", nil
	}

	return entries[len(entries)-1].ID, nil
}

// ensureRepo connects to repoName's repository, or creates it if connecting
// fails, then applies Config.Compression as the repository's global policy
// if set. Unlike borg (whose CLI is stateless per invocation), kopia
// requires an explicit connect before any other command can use a
// repository, and has no separate "does this repository already exist"
// check that works uniformly across every storage type — so rather than
// reimplementing an existence check per storage type (a bucket HEAD
// request, an SFTP stat, ...), this drives the same "try connect, and if
// that fails, create" idiom kopia's own docs use for non-interactive setup.
// Connecting is repeated on every call rather than assumed to persist from
// a previous dackup run, so this doesn't depend on configPath surviving on
// disk between invocations.
func (backend Backend) ensureRepo(envRunner shared.EnvCommandRunner, passwordEnv []string, repoName string, configPath string) error {
	if err := backend.fileSystem().MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return fmt.Errorf("failed to create kopia config directory: %w", err)
	}

	connectArgs, connectEnv, err := backend.connectArgs(repoName, configPath)
	if err != nil {
		return err
	}

	env := append(append([]string{}, passwordEnv...), connectEnv...)

	connectErr := envRunner.RunInDirWithEnv("", env, backend.Config.bin(), connectArgs...)
	if connectErr != nil {
		createArgs, createEnv, err := backend.createArgs(repoName, configPath)
		if err != nil {
			return err
		}

		env := append(append([]string{}, passwordEnv...), createEnv...)

		if createErr := envRunner.RunInDirWithEnv("", env, backend.Config.bin(), createArgs...); createErr != nil {
			return fmt.Errorf("failed to connect to or create kopia repository %q: connect: %v; create: %w", repoName, connectErr, createErr)
		}

		backend.log("INFO", fmt.Sprintf("Initialized kopia repository %q (%s)", repoName, backend.Config.storageType()))
	}

	if strings.TrimSpace(backend.Config.Compression) == "" {
		return nil
	}

	if err := envRunner.RunInDirWithEnv("", passwordEnv, backend.Config.bin(), "policy", "set", "--global", "--compression="+backend.Config.Compression, "--config-file="+configPath); err != nil {
		return fmt.Errorf("failed to set kopia compression policy for %q: %w", repoName, err)
	}

	return nil
}

// connectArgs builds the full "repository connect <type> ..." argument list
// (everything after "kopia") plus any extra env vars the storage backend
// needs beyond the repository password, for repoName's repository.
func (backend Backend) connectArgs(repoName string, configPath string) ([]string, []string, error) {
	invocation, err := backend.storageInvocation(repoName)
	if err != nil {
		return nil, nil, err
	}

	args := append([]string{"repository", "connect", invocation.Kind}, invocation.Args...)
	args = append(args, "--config-file="+configPath)

	return args, invocation.Env, nil
}

// createArgs builds the full "repository create <type> ..." argument list,
// mirroring connectArgs. For the filesystem storage type it also ensures
// the local repository directory exists first, since kopia's filesystem
// provider expects the target directory to already be there. This is the
// one place Backend still branches on a specific storage kind rather than
// going through storage.Provider — local directory creation needs
// shared.FileSystem, which providers deliberately don't depend on (adding
// it to the interface just for this one case would force the other seven
// providers to carry a dependency they never use).
func (backend Backend) createArgs(repoName string, configPath string) ([]string, []string, error) {
	invocation, err := backend.storageInvocation(repoName)
	if err != nil {
		return nil, nil, err
	}

	if invocation.Kind == filesystem.Name {
		if err := backend.fileSystem().MkdirAll(backend.repoPath(repoName), 0o700); err != nil {
			return nil, nil, fmt.Errorf("failed to create kopia repository directory %s: %w", backend.repoPath(repoName), err)
		}
	}

	args := append([]string{"repository", "create", invocation.Kind}, invocation.Args...)
	args = append(args, "--config-file="+configPath)

	return args, invocation.Env, nil
}

// storageInvocation resolves Config's active storage type into its
// storage.Provider (see storage/provider.go) and asks it to build the
// invocation for repoName's repository.
func (backend Backend) storageInvocation(repoName string) (storage.Invocation, error) {
	provider, err := backend.Config.provider(backend.ReposRoot)
	if err != nil {
		return storage.Invocation{}, err
	}

	return provider.BuildInvocation(repoName, backend.secretStore())
}

func (backend Backend) passwordEnv() ([]string, error) {
	password, err := backend.secretStore().Decrypt(backend.Config.EncryptedPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt kopia password: %w", err)
	}

	return []string{passwordEnvVar + "=" + password}, nil
}

// repoPath is only meaningful for the filesystem storage type — the local
// directory holding repoName's repository data.
func (backend Backend) repoPath(repoName string) string {
	return filepath.Join(backend.ReposRoot, repoName)
}

func (backend Backend) configPath(repoName string) string {
	return filepath.Join(backend.ReposRoot, configFileSubdir, repoName+".config")
}

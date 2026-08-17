package restic

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/shared"
	"fmt"
	"path/filepath"
)

// snapshotRepo ensures the named repository exists, then creates one restic
// snapshot covering every entry in relativePaths in a single invocation —
// restic, like borg, accepts several source paths per "backup" call, unlike
// kopia which snapshots one directory per invocation.
func (backend Backend) snapshotRepo(stagingDir string, repoName string, relativePaths []string) error {
	if backend.dryRun() {
		backend.log("INFO", fmt.Sprintf("[dry-run] Would create a restic snapshot in repository %q from %s (paths: %v)", repoName, stagingDir, relativePaths))
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

	invocation, err := backend.repositoryInvocation(repoName)
	if err != nil {
		return err
	}

	env := backend.commandEnv(passwordEnv, invocation)

	if err := backend.ensureRepo(envRunner, env, invocation); err != nil {
		return err
	}

	args := append(append([]string{}, invocation.Args...), "backup", "-r", invocation.Repository)
	args = append(args, relativePaths...)

	backend.log("INFO", fmt.Sprintf("Creating restic snapshot in repository %q from %s (paths: %v)", repoName, stagingDir, relativePaths))

	if err := envRunner.RunInDirWithEnv(stagingDir, env, backend.Config.bin(), args...); err != nil {
		return fmt.Errorf("restic backup failed for repository %q: %w", repoName, err)
	}

	return nil
}

// restoreRepo restores the latest snapshot from the named repository into
// stagingDir. It never creates a repository — a restore against a
// repository that doesn't exist (or can't be reached) is a graceful no-op,
// not an error, since there's nothing to restore either way.
func (backend Backend) restoreRepo(stagingDir string, repoName string) error {
	if backend.dryRun() {
		backend.log("INFO", fmt.Sprintf("[dry-run] Would restore the latest restic snapshot from repository %q into %s", repoName, stagingDir))
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

	invocation, err := backend.repositoryInvocation(repoName)
	if err != nil {
		return err
	}

	env := backend.commandEnv(passwordEnv, invocation)

	if !backend.repoExists(envRunner, env, invocation) {
		backend.log("WARN", fmt.Sprintf("Restic repository %q does not exist yet; nothing to restore", repoName))
		return nil
	}

	if err := backend.fileSystem().MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("failed to create staging directory %s: %w", stagingDir, err)
	}

	backend.log("INFO", fmt.Sprintf("Restoring the latest restic snapshot from repository %q into %s", repoName, stagingDir))

	args := append(append([]string{}, invocation.Args...), "restore", "latest", "-r", invocation.Repository, "--target", stagingDir)

	if err := envRunner.RunInDirWithEnv("", env, backend.Config.bin(), args...); err != nil {
		return fmt.Errorf("restic restore failed for repository %q: %w", repoName, err)
	}

	return nil
}

// ensureRepo initializes the repository named by invocation if it doesn't
// already exist. Unlike kopia (which requires an explicit connect step),
// restic is stateless per invocation like borg — but unlike borg (a
// local-directory-only CLI whose repository existence can be checked with a
// plain filesystem Stat), restic's storage types range from local
// directories to remote object stores, so existence is checked by driving
// the CLI itself via repoExists ("restic cat config"), restic's own
// documented way to probe a repository without side effects (see restic's
// "Scripting" docs) — any nonzero exit is treated as "not present yet", the
// same broad-but-simple approach kopia's connect-then-create-fallback idiom
// uses for its own different reason (no uniform existence check across
// kopia's storage types either).
func (backend Backend) ensureRepo(envRunner shared.EnvCommandRunner, env []string, invocation storage.Invocation) error {
	if backend.repoExists(envRunner, env, invocation) {
		return nil
	}

	args := append(append([]string{}, invocation.Args...), "init", "-r", invocation.Repository)

	if err := envRunner.RunInDirWithEnv("", env, backend.Config.bin(), args...); err != nil {
		return fmt.Errorf("failed to initialize restic repository %s: %w", invocation.Repository, err)
	}

	backend.log("INFO", fmt.Sprintf("Initialized restic repository %s", invocation.Repository))
	return nil
}

func (backend Backend) repoExists(envRunner shared.EnvCommandRunner, env []string, invocation storage.Invocation) bool {
	args := append(append([]string{}, invocation.Args...), "cat", "config", "-r", invocation.Repository)

	_, err := envRunner.OutputWithEnv(env, backend.Config.bin(), args...)
	return err == nil
}

// repositoryInvocation resolves Config's active storage type into its
// storage.Provider (see storage/provider.go) and asks it to build the
// invocation for repoName's repository.
func (backend Backend) repositoryInvocation(repoName string) (storage.Invocation, error) {
	provider, err := backend.Config.provider(backend.ReposRoot)
	if err != nil {
		return storage.Invocation{}, err
	}

	return provider.BuildInvocation(repoName, backend.secretStore())
}

func (backend Backend) passwordEnv() ([]string, error) {
	password, err := backend.secretStore().Decrypt(backend.Config.EncryptedPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt restic password: %w", err)
	}

	return []string{passwordEnvVar + "=" + password}, nil
}

// commandEnv combines passwordEnv, invocation's own storage-credential env,
// and a fixed RESTIC_CACHE_DIR pointed under ReposRoot. Restic caches
// repository metadata locally by default under $HOME/.cache/restic (or
// $XDG_CACHE_HOME) — confirmed by driving the real CLI, which fails outright
// ("unable to open cache: mkdir ... read-only file system") the moment that
// default location isn't writable by whatever user invokes dackup. Since
// dackup already keeps every other piece of this backend's local state
// under ReposRoot rather than depending on the invoking user's home
// directory, the cache gets the same treatment instead of assuming $HOME is
// writable — a real constraint for e.g. a restricted systemd service user.
func (backend Backend) commandEnv(passwordEnv []string, invocation storage.Invocation) []string {
	env := append(append([]string{}, passwordEnv...), invocation.Env...)
	return append(env, "RESTIC_CACHE_DIR="+filepath.Join(backend.ReposRoot, ".restic-cache"))
}

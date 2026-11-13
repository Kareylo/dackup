package restic

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/shared"
	"errors"
	"fmt"
	"path/filepath"
)

// resticExitRepoDoesNotExist is restic's documented exit code for "repository
// does not exist" (restic >= 0.17; a wrong password is 12, most other
// failures 1).
const resticExitRepoDoesNotExist = 10

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
// stagingDir. It never creates a repository. A repository restic reports
// as missing (exit 10) is a graceful no-op, since nothing was ever backed
// up to it; any other failure to open it (wrong password, unreachable
// storage) is an error, so an outage is never mistaken for "nothing to
// restore".
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

	exists, err := backend.repoExists(envRunner, env, invocation)
	if err != nil {
		return err
	}

	if !exists {
		backend.log("WARN", fmt.Sprintf("Restic repository %q does not exist yet; nothing to restore", repoName))
		return nil
	}

	if err := backend.fileSystem().MkdirAll(stagingDir, shared.StagingDirMode); err != nil {
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
// "Scripting" docs). Only exit code 10 ("repository does not exist") leads
// to "restic init"; any other failure is returned, so a repository that
// merely couldn't be opened is never re-initialized.
func (backend Backend) ensureRepo(envRunner shared.EnvCommandRunner, env []string, invocation storage.Invocation) error {
	exists, err := backend.repoExists(envRunner, env, invocation)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	args := append(append([]string{}, invocation.Args...), "init", "-r", invocation.Repository)

	if err := envRunner.RunInDirWithEnv("", env, backend.Config.bin(), args...); err != nil {
		return fmt.Errorf("failed to initialize restic repository %s: %w", invocation.Repository, err)
	}

	backend.log("INFO", fmt.Sprintf("Initialized restic repository %s", invocation.Repository))
	return nil
}

// repoExists probes the repository with "restic cat config". It reports
// false only for restic's "repository does not exist" exit code, and an
// error for every other failure.
func (backend Backend) repoExists(envRunner shared.EnvCommandRunner, env []string, invocation storage.Invocation) (bool, error) {
	args := append(append([]string{}, invocation.Args...), "cat", "config", "-r", invocation.Repository)

	_, err := envRunner.OutputWithEnv(env, backend.Config.bin(), args...)
	if err == nil {
		return true, nil
	}

	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) && exitErr.ExitCode() == resticExitRepoDoesNotExist {
		return false, nil
	}

	return false, fmt.Errorf("failed to open restic repository %s: %w", invocation.Repository, err)
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

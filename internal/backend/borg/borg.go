// Package borg implements backend.Backend and backend.GroupedBackend by
// driving the borg CLI. It only depends on internal/shared (not
// internal/backend) so it can be referenced from internal/backend's
// GroupedBackend interface (via shared.BackendGroup) without an import
// cycle back from internal/backend's Factory, which must import this
// package to construct a Backend.
package borg

import (
	"dackup/internal/shared"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Name is the backend identifier written to DackupConfig.Backend.
	Name = "borg"

	// DefaultBin is used when Config.Bin is empty; resolved via PATH.
	DefaultBin = "borg"

	// DefaultGlobalRepoName is used when Config.GlobalRepoName is empty.
	DefaultGlobalRepoName = "global"

	// DefaultEncryption is used when Config.Encryption is empty.
	DefaultEncryption = "repokey"

	passphraseEnvVar = "BORG_PASSPHRASE"
)

// Config is this backend's own typed settings, decoded from
// DackupConfig.BackendSettings when Backend == Name. Repository storage
// itself lives under the top-level DackupConfig.BackendDir, not here — see
// AGENTS.md's "Backend interface" section for why that field lives at the
// top level instead of inside backend_settings.
type Config struct {
	// Bin is the path to the borg binary. Empty means "borg", resolved via
	// PATH.
	Bin string `json:"bin,omitempty"`

	// GlobalRepoName names the repository (a subdirectory of BackendDir)
	// that receives a full copy of everything staged, in addition to each
	// container-group's own repository.
	GlobalRepoName string `json:"global_repo_name,omitempty"`

	// Encryption is passed to `borg init --encryption=` the first time
	// each repository is created. One of: none, authenticated,
	// authenticated-blake2, repokey, repokey-blake2, keyfile,
	// keyfile-blake2.
	Encryption string `json:"encryption,omitempty"`

	// EncryptedPassphrase is ciphertext produced by a shared.SecretStore,
	// never a plaintext passphrase. Required unless Encryption is "none".
	EncryptedPassphrase string `json:"encrypted_passphrase,omitempty"`

	// Compression is passed to `borg create --compression`, e.g.
	// "zstd,6". Empty uses borg's own default (lz4).
	Compression string `json:"compression,omitempty"`
}

// DefaultConfig returns a Config with GlobalRepoName and Encryption set to
// their defaults; every other field is left empty.
func DefaultConfig() Config {
	return Config{
		GlobalRepoName: DefaultGlobalRepoName,
		Encryption:     DefaultEncryption,
	}
}

// Validate reports whether config is well-formed. It does not check
// GlobalRepoName against actual container-group names — that requires the
// groups passed to BackupGroups/RestoreGroups, so it's checked there
// instead (see Backend.validateGroupNames).
func (config Config) Validate() error {
	if strings.TrimSpace(config.GlobalRepoName) == "" {
		return fmt.Errorf("borg global_repo_name cannot be empty")
	}

	if strings.TrimSpace(config.Encryption) == "" {
		return fmt.Errorf("borg encryption cannot be empty")
	}

	if config.Encryption != "none" && strings.TrimSpace(config.EncryptedPassphrase) == "" {
		return fmt.Errorf("borg encryption %q requires encrypted_passphrase to be set", config.Encryption)
	}

	return nil
}

// ParseConfig decodes raw backend_settings JSON into a Config, applying
// defaults for any field raw doesn't set, then validates the result.
func ParseConfig(raw json.RawMessage) (Config, error) {
	config := DefaultConfig()

	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &config); err != nil {
			return Config{}, fmt.Errorf("failed to parse borg backend settings: %w", err)
		}
	}

	if err := config.Validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

func (config Config) bin() string {
	if strings.TrimSpace(config.Bin) != "" {
		return config.Bin
	}

	return DefaultBin
}

// Backend drives the borg CLI to implement backend.Backend and
// backend.GroupedBackend. ReposRoot is the top-level
// DackupConfig.BackendDir; each container-group gets its own repository at
// filepath.Join(ReposRoot, group.Name), plus one more at
// filepath.Join(ReposRoot, Config.GlobalRepoName) holding a full copy of
// everything staged.
type Backend struct {
	Config    Config
	ReposRoot string
	Runner    shared.CommandRunner
	Logger    shared.Logger
	Options   *shared.Options
	Secrets   shared.SecretStore
	FS        shared.FileSystem
	Clock     func() time.Time
}

// Name returns the backend identifier "borg".
func (backend Backend) Name() string {
	return Name
}

// Backup implements the plain backend.Backend fallback used when a caller
// doesn't know about container groups: everything under stagingDir goes
// into a single archive in the global repository.
func (backend Backend) Backup(stagingDir string) error {
	return backend.archiveRepo(stagingDir, backend.Config.GlobalRepoName, []string{"."})
}

// Restore implements the plain backend.Backend fallback, extracting the
// latest archive from the global repository into stagingDir.
func (backend Backend) Restore(stagingDir string) error {
	return backend.extractRepo(stagingDir, backend.Config.GlobalRepoName)
}

// BackupGroups archives each group into its own repository, scoped to that
// group's paths, then archives everything under stagingDir into the global
// repository too. A group with no configured paths is skipped (its
// repository is left untouched) rather than archiving the whole
// stagingDir under its name.
func (backend Backend) BackupGroups(stagingDir string, groups []shared.BackendGroup) error {
	if err := backend.validateGroupNames(groups); err != nil {
		return err
	}

	for _, group := range groups {
		if len(group.Paths) == 0 {
			backend.log("INFO", fmt.Sprintf("No paths configured for group %s; skipping its borg repository", group.Name))
			continue
		}

		if err := backend.archiveRepo(stagingDir, group.Name, group.Paths); err != nil {
			return err
		}
	}

	return backend.archiveRepo(stagingDir, backend.Config.GlobalRepoName, []string{"."})
}

// RestoreGroups extracts the latest archive from each group's own
// repository into stagingDir. The global repository is left alone —
// restoring from it is a separate, not-yet-implemented operation, since it
// would restore everything rather than matching backup's per-group
// granularity.
func (backend Backend) RestoreGroups(stagingDir string, groups []shared.BackendGroup) error {
	if err := backend.validateGroupNames(groups); err != nil {
		return err
	}

	for _, group := range groups {
		if err := backend.extractRepo(stagingDir, group.Name); err != nil {
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

func (backend Backend) archiveRepo(stagingDir string, repoName string, relativePaths []string) error {
	repoPath := backend.repoPath(repoName)

	if backend.dryRun() {
		backend.log("INFO", fmt.Sprintf("[dry-run] Would create a borg archive in %s from %s (paths: %v)", repoPath, stagingDir, relativePaths))
		return nil
	}

	envRunner, err := backend.envRunner()
	if err != nil {
		return err
	}

	env, err := backend.passphraseEnv()
	if err != nil {
		return err
	}

	if err := backend.ensureRepo(envRunner, env, repoPath); err != nil {
		return err
	}

	archive := repoPath + "::" + backend.archiveName()

	args := []string{"create"}
	if strings.TrimSpace(backend.Config.Compression) != "" {
		args = append(args, "--compression", backend.Config.Compression)
	}
	args = append(args, archive)
	args = append(args, relativePaths...)

	backend.log("INFO", fmt.Sprintf("Creating borg archive %s", archive))

	if err := envRunner.RunInDirWithEnv(stagingDir, env, backend.Config.bin(), args...); err != nil {
		return fmt.Errorf("borg create failed for %s: %w", archive, err)
	}

	return nil
}

func (backend Backend) extractRepo(stagingDir string, repoName string) error {
	repoPath := backend.repoPath(repoName)

	if backend.dryRun() {
		backend.log("INFO", fmt.Sprintf("[dry-run] Would extract the latest borg archive from %s into %s", repoPath, stagingDir))
		return nil
	}

	if !backend.repoInitialized(repoPath) {
		backend.log("WARN", fmt.Sprintf("Borg repository %s does not exist yet; nothing to restore", repoPath))
		return nil
	}

	envRunner, err := backend.envRunner()
	if err != nil {
		return err
	}

	env, err := backend.passphraseEnv()
	if err != nil {
		return err
	}

	archiveName, err := backend.latestArchive(envRunner, env, repoPath)
	if err != nil {
		return err
	}

	if archiveName == "" {
		backend.log("WARN", fmt.Sprintf("Borg repository %s has no archives yet; nothing to restore", repoPath))
		return nil
	}

	if err := backend.fileSystem().MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("failed to create staging directory %s: %w", stagingDir, err)
	}

	archive := repoPath + "::" + archiveName

	backend.log("INFO", fmt.Sprintf("Extracting borg archive %s into %s", archive, stagingDir))

	if err := envRunner.RunInDirWithEnv(stagingDir, env, backend.Config.bin(), "extract", archive); err != nil {
		return fmt.Errorf("borg extract failed for %s: %w", archive, err)
	}

	return nil
}

func (backend Backend) latestArchive(envRunner shared.EnvCommandRunner, env []string, repoPath string) (string, error) {
	output, err := envRunner.OutputWithEnv(env, backend.Config.bin(), "list", repoPath, "--last", "1", "--short")
	if err != nil {
		return "", fmt.Errorf("failed to list archives in %s: %w", repoPath, err)
	}

	return strings.TrimSpace(string(output)), nil
}

func (backend Backend) ensureRepo(envRunner shared.EnvCommandRunner, env []string, repoPath string) error {
	if backend.repoInitialized(repoPath) {
		return nil
	}

	if err := backend.fileSystem().MkdirAll(repoPath, 0o700); err != nil {
		return fmt.Errorf("failed to create borg repository directory %s: %w", repoPath, err)
	}

	if err := envRunner.RunInDirWithEnv("", env, backend.Config.bin(), "init", "--encryption="+backend.Config.Encryption, repoPath); err != nil {
		return fmt.Errorf("failed to initialize borg repository %s: %w", repoPath, err)
	}

	backend.log("INFO", fmt.Sprintf("Initialized borg repository %s", repoPath))
	return nil
}

// repoInitialized reports whether repoPath already holds a borg repository,
// by checking for the "config" file every initialized local repo has.
func (backend Backend) repoInitialized(repoPath string) bool {
	_, err := backend.fileSystem().Stat(filepath.Join(repoPath, "config"))
	return err == nil
}

func (backend Backend) passphraseEnv() ([]string, error) {
	if backend.Config.Encryption == "none" {
		return nil, nil
	}

	passphrase, err := backend.secretStore().Decrypt(backend.Config.EncryptedPassphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt borg passphrase: %w", err)
	}

	return []string{passphraseEnvVar + "=" + passphrase}, nil
}

func (backend Backend) repoPath(name string) string {
	return filepath.Join(backend.ReposRoot, name)
}

func (backend Backend) archiveName() string {
	return backend.clock()().UTC().Format("20060102-150405")
}

func (backend Backend) envRunner() (shared.EnvCommandRunner, error) {
	envRunner, ok := backend.runner().(shared.EnvCommandRunner)
	if !ok {
		return nil, fmt.Errorf("command runner does not support the environment variables borg requires")
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

func (backend Backend) clock() func() time.Time {
	if backend.Clock != nil {
		return backend.Clock
	}

	return time.Now
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

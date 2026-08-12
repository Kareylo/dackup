package kopia

import (
	"dackup/internal/backend/kopia/storage/filesystem"
	"dackup/internal/shared"
	"fmt"
	"os"
	"strings"
	"testing"
)

// fakeCall records one invocation made through fakeEnvRunner, distinguishing
// RunInDirWithEnv (dir/env set, no output) from OutputWithEnv (env set, dir
// always empty).
type fakeCall struct {
	dir  string
	env  []string
	name string
	args []string
}

type fakeEnvRunner struct {
	calls   []fakeCall
	runErr  error
	runErrs map[string]error
	outputs map[string][]byte
	outErrs map[string]error
}

func (r *fakeEnvRunner) Run(name string, args ...string) error {
	return r.RunInDirWithEnv("", nil, name, args...)
}

func (r *fakeEnvRunner) Output(name string, args ...string) ([]byte, error) {
	return r.OutputWithEnv(nil, name, args...)
}

func (r *fakeEnvRunner) LookPath(file string) (string, error) { return file, nil }

func (r *fakeEnvRunner) RunInDirWithEnv(dir string, env []string, name string, args ...string) error {
	r.calls = append(r.calls, fakeCall{dir: dir, env: env, name: name, args: args})

	key := strings.Join(args, " ")
	if err, ok := r.runErrs[key]; ok {
		return err
	}

	return r.runErr
}

func (r *fakeEnvRunner) OutputWithEnv(env []string, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, fakeCall{env: env, name: name, args: args})

	key := strings.Join(args, " ")
	if err, ok := r.outErrs[key]; ok {
		return nil, err
	}

	return r.outputs[key], nil
}

// runnerWithoutEnvSupport implements shared.CommandRunner but not
// shared.EnvCommandRunner, for testing the capability-missing error path.
type runnerWithoutEnvSupport struct{}

func (runnerWithoutEnvSupport) Run(name string, args ...string) error              { return nil }
func (runnerWithoutEnvSupport) Output(name string, args ...string) ([]byte, error) { return nil, nil }
func (runnerWithoutEnvSupport) LookPath(file string) (string, error)               { return file, nil }

type fakeFileSystem struct {
	existing map[string]bool
	mkdirs   []string
}

func newFakeFileSystem(existing ...string) *fakeFileSystem {
	fs := &fakeFileSystem{existing: make(map[string]bool)}
	for _, path := range existing {
		fs.existing[path] = true
	}
	return fs
}

func (fs *fakeFileSystem) Stat(name string) (os.FileInfo, error) {
	if fs.existing[name] {
		return nil, nil
	}
	return nil, os.ErrNotExist
}

func (fs *fakeFileSystem) MkdirAll(path string, perm os.FileMode) error {
	fs.mkdirs = append(fs.mkdirs, path)
	fs.existing[path] = true
	return nil
}

func (fs *fakeFileSystem) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return nil, fmt.Errorf("fakeFileSystem does not support OpenFile")
}

type fakeLogger struct {
	messages []string
}

func (l *fakeLogger) Log(level string, message string) {
	l.messages = append(l.messages, level+": "+message)
}

type fakeSecretStore struct{}

func (fakeSecretStore) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (fakeSecretStore) Decrypt(ciphertext string) (string, error) {
	value, ok := strings.CutPrefix(ciphertext, "enc:")
	if !ok {
		return "", fmt.Errorf("not encrypted with fakeSecretStore")
	}
	return value, nil
}

func testBackend(runner *fakeEnvRunner, fs *fakeFileSystem, logger *fakeLogger) Backend {
	return Backend{
		Config: Config{
			GlobalRepoName:    "global",
			EncryptedPassword: "enc:hunter2",
		},
		ReposRoot: "/repos",
		Runner:    runner,
		Logger:    logger,
		Options:   &shared.Options{},
		Secrets:   fakeSecretStore{},
		FS:        fs,
	}
}

const globalConfigPath = "/repos/.kopia-config/global.config"

func connectArgsKey(kind string, extra ...string) string {
	args := append([]string{"repository", "connect", kind}, extra...)
	args = append(args, "--config-file="+globalConfigPath)
	return strings.Join(args, " ")
}

// --- Config / Validate / ParseConfig ---
//
// Storage-type-specific Validate/ParseConfig/storageInvocation coverage
// lives in each storage_<type>_test.go alongside the type it tests; what
// remains here is Config-level behavior that isn't owned by any one
// storage type.

func TestConfig_ValidateRequiresGlobalRepoName(t *testing.T) {
	config := Config{EncryptedPassword: "enc:secret"}
	if err := config.Validate(); err == nil {
		t.Fatal("expected error for empty global_repo_name")
	}
}

func TestConfig_ValidateRequiresEncryptedPassword(t *testing.T) {
	config := Config{GlobalRepoName: "global"}
	if err := config.Validate(); err == nil {
		t.Fatal("expected error for empty encrypted_password")
	}

	config.EncryptedPassword = "enc:secret"
	if err := config.Validate(); err != nil {
		t.Fatalf("expected no error once encrypted_password is set, got %v", err)
	}
}

func TestConfig_ValidateUnknownStorageTypeReturnsError(t *testing.T) {
	config := Config{GlobalRepoName: "global", EncryptedPassword: "enc:secret", StorageType: "dropbox"}
	if err := config.Validate(); err == nil {
		t.Fatal("expected error for unknown storage_type")
	}
}

func TestParseConfig_AppliesDefaults(t *testing.T) {
	if _, err := ParseConfig(nil); err == nil {
		t.Fatal("expected error: no encrypted_password provided")
	}

	config, err := ParseConfig([]byte(`{"encrypted_password":"enc:secret"}`))
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}

	if config.GlobalRepoName != DefaultGlobalRepoName {
		t.Fatalf("expected default global_repo_name %q, got %q", DefaultGlobalRepoName, config.GlobalRepoName)
	}

	if config.storageType() != filesystem.Name {
		t.Fatalf("expected default storage type %q, got %q", filesystem.Name, config.storageType())
	}
}

func TestParseConfig_InvalidJSONReturnsError(t *testing.T) {
	if _, err := ParseConfig([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- Backup / Restore (plain, ungrouped) ---

func TestBackend_Backup_DryRunMakesNoCalls(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, newFakeFileSystem(), &fakeLogger{})
	backend.Options = &shared.Options{DryRun: true}

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no runner calls in dry-run, got %+v", runner.calls)
	}
}

func TestBackend_Backup_ConnectsToExistingRepoThenCreatesSnapshot(t *testing.T) {
	runner := &fakeEnvRunner{}
	fs := newFakeFileSystem()
	backend := testBackend(runner, fs, &fakeLogger{})

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 calls (connect, snapshot create), got %d: %+v", len(runner.calls), runner.calls)
	}

	connectCall := runner.calls[0]
	wantConnectArgs := []string{"repository", "connect", "filesystem", "--path=/repos/global", "--config-file=" + globalConfigPath}
	if !equalArgs(connectCall.args, wantConnectArgs) {
		t.Fatalf("expected connect args %v, got %v", wantConnectArgs, connectCall.args)
	}
	if !containsEnv(connectCall.env, "KOPIA_PASSWORD=hunter2") {
		t.Fatalf("expected connect call to carry decrypted password, got env %v", connectCall.env)
	}

	snapshotCall := runner.calls[1]
	wantSnapshotArgs := []string{"snapshot", "create", "/staging", "--config-file=" + globalConfigPath}
	if !equalArgs(snapshotCall.args, wantSnapshotArgs) {
		t.Fatalf("expected snapshot create args %v, got %v", wantSnapshotArgs, snapshotCall.args)
	}
}

func TestBackend_Backup_CreatesRepoWhenConnectFails(t *testing.T) {
	runner := &fakeEnvRunner{
		runErrs: map[string]error{
			connectArgsKey("filesystem", "--path=/repos/global"): fmt.Errorf("not a repository"),
		},
	}
	fs := newFakeFileSystem()
	logger := &fakeLogger{}
	backend := testBackend(runner, fs, logger)

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected 3 calls (connect, create, snapshot create), got %d: %+v", len(runner.calls), runner.calls)
	}

	createCall := runner.calls[1]
	wantCreateArgs := []string{"repository", "create", "filesystem", "--path=/repos/global", "--config-file=" + globalConfigPath}
	if !equalArgs(createCall.args, wantCreateArgs) {
		t.Fatalf("expected create args %v, got %v", wantCreateArgs, createCall.args)
	}

	if !hasMessageContaining(logger.messages, "Initialized kopia repository") {
		t.Fatalf("expected an initialization log message, got %v", logger.messages)
	}

	found := false
	for _, dir := range fs.mkdirs {
		if dir == "/repos/global" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the local repository directory to be created before create, mkdirs: %v", fs.mkdirs)
	}
}

func TestBackend_Backup_ReturnsErrorWhenBothConnectAndCreateFail(t *testing.T) {
	runner := &fakeEnvRunner{runErr: fmt.Errorf("boom")}
	backend := testBackend(runner, newFakeFileSystem(), &fakeLogger{})

	if err := backend.Backup("/staging"); err == nil {
		t.Fatal("expected error when both connect and create fail")
	}
}

func TestBackend_Backup_AppliesCompressionPolicy(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, newFakeFileSystem(), &fakeLogger{})
	backend.Config.Compression = "zstd"

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected 3 calls (connect, policy set, snapshot create), got %d: %+v", len(runner.calls), runner.calls)
	}

	wantPolicyArgs := []string{"policy", "set", "--global", "--compression=zstd", "--config-file=" + globalConfigPath}
	if !equalArgs(runner.calls[1].args, wantPolicyArgs) {
		t.Fatalf("expected policy set args %v, got %v", wantPolicyArgs, runner.calls[1].args)
	}
}

func TestBackend_Backup_MissingEnvSupportReturnsError(t *testing.T) {
	backend := Backend{
		Config:    Config{GlobalRepoName: "global", EncryptedPassword: "enc:hunter2"},
		ReposRoot: "/repos",
		Runner:    runnerWithoutEnvSupport{},
		Secrets:   fakeSecretStore{},
		FS:        newFakeFileSystem(),
	}

	if err := backend.Backup("/staging"); err == nil {
		t.Fatal("expected error when the command runner doesn't support env vars")
	}
}

func TestBackend_Restore_DryRunMakesNoCalls(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, newFakeFileSystem(), &fakeLogger{})
	backend.Options = &shared.Options{DryRun: true}

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no runner calls in dry-run, got %+v", runner.calls)
	}
}

func TestBackend_Restore_RepoNotReachableIsGracefulNoOp(t *testing.T) {
	runner := &fakeEnvRunner{
		runErrs: map[string]error{
			connectArgsKey("filesystem", "--path=/repos/global"): fmt.Errorf("not a repository"),
		},
	}
	logger := &fakeLogger{}
	backend := testBackend(runner, newFakeFileSystem(), logger)

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected only the failed connect attempt, got %+v", runner.calls)
	}

	if !hasMessageContaining(logger.messages, "does not exist yet") {
		t.Fatalf("expected a log message about the unreachable repo, got %v", logger.messages)
	}
}

func TestBackend_Restore_EmptySnapshotListIsGracefulNoOp(t *testing.T) {
	runner := &fakeEnvRunner{
		outputs: map[string][]byte{
			"snapshot list /staging --json --config-file=" + globalConfigPath: []byte(`[]`),
		},
	}
	logger := &fakeLogger{}
	backend := testBackend(runner, newFakeFileSystem(), logger)

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if !hasMessageContaining(logger.messages, "no snapshots") {
		t.Fatalf("expected a log message about no snapshots, got %v", logger.messages)
	}

	for _, call := range runner.calls {
		if call.args[0] == "snapshot" && call.args[1] == "restore" {
			t.Fatalf("expected no restore call when there are no snapshots, got %+v", call)
		}
	}
}

func TestBackend_Restore_RestoresLatestSnapshot(t *testing.T) {
	runner := &fakeEnvRunner{
		outputs: map[string][]byte{
			"snapshot list /staging --json --config-file=" + globalConfigPath: []byte(`[{"id":"k1a"},{"id":"k2b"}]`),
		},
	}
	fs := newFakeFileSystem()
	backend := testBackend(runner, fs, &fakeLogger{})

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected 3 calls (connect, list, restore), got %d: %+v", len(runner.calls), runner.calls)
	}

	restoreCall := runner.calls[2]
	wantArgs := []string{"snapshot", "restore", "k2b", "/staging", "--config-file=" + globalConfigPath}
	if !equalArgs(restoreCall.args, wantArgs) {
		t.Fatalf("expected restore args %v, got %v", wantArgs, restoreCall.args)
	}

	found := false
	for _, dir := range fs.mkdirs {
		if dir == "/staging" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected restore target to be created before restore, mkdirs: %v", fs.mkdirs)
	}
}

// --- BackupGroups / RestoreGroups ---

func TestBackend_BackupGroups_SnapshotsEachGroupPathAndTheGlobalRepo(t *testing.T) {
	runner := &fakeEnvRunner{}
	fs := newFakeFileSystem()
	backend := testBackend(runner, fs, &fakeLogger{})

	groups := []shared.BackendGroup{
		{Name: "paperless", Paths: []string{"data/paperless", "data/paperless_db"}},
		{Name: "adguard", Paths: []string{"config/adguard"}},
	}

	if err := backend.BackupGroups("/staging", groups); err != nil {
		t.Fatalf("BackupGroups returned error: %v", err)
	}

	var snapshotSources []string
	for _, call := range runner.calls {
		if len(call.args) >= 2 && call.args[0] == "snapshot" && call.args[1] == "create" {
			snapshotSources = append(snapshotSources, call.args[2])
		}
	}

	want := []string{"/staging/data/paperless", "/staging/data/paperless_db", "/staging/config/adguard", "/staging"}
	if !equalArgs(snapshotSources, want) {
		t.Fatalf("expected snapshot sources %v, got %v", want, snapshotSources)
	}
}

func TestBackend_BackupGroups_SkipsGroupWithNoPaths(t *testing.T) {
	runner := &fakeEnvRunner{}
	logger := &fakeLogger{}
	backend := testBackend(runner, newFakeFileSystem(), logger)

	groups := []shared.BackendGroup{{Name: "empty-group"}}

	if err := backend.BackupGroups("/staging", groups); err != nil {
		t.Fatalf("BackupGroups returned error: %v", err)
	}

	for _, call := range runner.calls {
		if strings.Contains(strings.Join(call.args, " "), "/repos/empty-group") {
			t.Fatalf("expected no repository touched for the empty group, got %+v", call)
		}
	}

	if !hasMessageContaining(logger.messages, "No paths configured for group empty-group") {
		t.Fatalf("expected a skip log message, got %v", logger.messages)
	}
}

func TestBackend_BackupGroups_RejectsGroupNameCollidingWithGlobalRepoName(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, newFakeFileSystem(), &fakeLogger{})

	groups := []shared.BackendGroup{{Name: "global", Paths: []string{"data"}}}

	if err := backend.BackupGroups("/staging", groups); err == nil {
		t.Fatal("expected error when a group name collides with global_repo_name")
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no calls when validation fails up front, got %+v", runner.calls)
	}
}

func TestBackend_RestoreGroups_RestoresFromEachGroupRepoOnly(t *testing.T) {
	runner := &fakeEnvRunner{
		outputs: map[string][]byte{
			"snapshot list /staging/data/paperless --json --config-file=/repos/.kopia-config/paperless.config": []byte(`[{"id":"p1"}]`),
			"snapshot list /staging/config/adguard --json --config-file=/repos/.kopia-config/adguard.config":   []byte(`[{"id":"a1"}]`),
		},
	}
	fs := newFakeFileSystem()
	backend := testBackend(runner, fs, &fakeLogger{})

	groups := []shared.BackendGroup{
		{Name: "paperless", Paths: []string{"data/paperless"}},
		{Name: "adguard", Paths: []string{"config/adguard"}},
	}

	if err := backend.RestoreGroups("/staging", groups); err != nil {
		t.Fatalf("RestoreGroups returned error: %v", err)
	}

	for _, call := range runner.calls {
		if strings.Contains(strings.Join(call.args, " "), "/repos/global") {
			t.Fatal("expected RestoreGroups to never touch the global repository")
		}
	}

	var restoredIDs []string
	for _, call := range runner.calls {
		if len(call.args) >= 2 && call.args[0] == "snapshot" && call.args[1] == "restore" {
			restoredIDs = append(restoredIDs, call.args[2])
		}
	}

	want := []string{"p1", "a1"}
	if !equalArgs(restoredIDs, want) {
		t.Fatalf("expected restored snapshot ids %v, got %v", want, restoredIDs)
	}
}

// --- storageInvocation dispatch ---

func TestBackend_StorageInvocation_UnknownTypeReturnsError(t *testing.T) {
	backend := testBackend(&fakeEnvRunner{}, newFakeFileSystem(), &fakeLogger{})
	backend.Config.StorageType = "dropbox"

	if _, err := backend.storageInvocation("myrepo"); err == nil {
		t.Fatal("expected error for unknown storage type")
	}
}

// --- Name ---

func TestBackend_Name(t *testing.T) {
	if (Backend{}).Name() != "kopia" {
		t.Fatalf("expected name %q, got %q", "kopia", (Backend{}).Name())
	}
}

// --- helpers ---

func containsEnv(env []string, want string) bool {
	for _, value := range env {
		if value == want {
			return true
		}
	}
	return false
}

func equalArgs(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func hasMessageContaining(messages []string, substr string) bool {
	for _, message := range messages {
		if strings.Contains(message, substr) {
			return true
		}
	}
	return false
}

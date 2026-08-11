package borg

import (
	"dackup/internal/shared"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeCall records one invocation made through fakeEnvRunner, distinguishing
// RunInDirWithEnv (dir/env set, no output) from OutputWithEnv (env set,
// dir always empty).
type fakeCall struct {
	dir  string
	env  []string
	name string
	args []string
}

type fakeEnvRunner struct {
	calls   []fakeCall
	runErr  error
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

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func testBackend(runner *fakeEnvRunner, fs *fakeFileSystem, logger *fakeLogger) Backend {
	return Backend{
		Config: Config{
			GlobalRepoName: "global",
			Encryption:     "repokey",
		},
		ReposRoot: "/repos",
		Runner:    runner,
		Logger:    logger,
		Options:   &shared.Options{},
		Secrets:   fakeSecretStore{},
		FS:        fs,
		Clock:     fixedClock(time.Date(2026, 8, 11, 10, 30, 0, 0, time.UTC)),
	}
}

// --- Config ---

func TestConfig_ValidateRequiresGlobalRepoName(t *testing.T) {
	config := Config{Encryption: "none"}
	if err := config.Validate(); err == nil {
		t.Fatal("expected error for empty global_repo_name")
	}
}

func TestConfig_ValidateRequiresEncryption(t *testing.T) {
	config := Config{GlobalRepoName: "global"}
	if err := config.Validate(); err == nil {
		t.Fatal("expected error for empty encryption")
	}
}

func TestConfig_ValidateRequiresPassphraseUnlessEncryptionNone(t *testing.T) {
	config := Config{GlobalRepoName: "global", Encryption: "repokey"}
	if err := config.Validate(); err == nil {
		t.Fatal("expected error when repokey encryption has no encrypted_passphrase")
	}

	config.EncryptedPassphrase = "enc:secret"
	if err := config.Validate(); err != nil {
		t.Fatalf("expected no error once encrypted_passphrase is set, got %v", err)
	}

	none := Config{GlobalRepoName: "global", Encryption: "none"}
	if err := none.Validate(); err != nil {
		t.Fatalf("expected no error for encryption none without a passphrase, got %v", err)
	}
}

func TestParseConfig_AppliesDefaults(t *testing.T) {
	config, err := ParseConfig(nil)
	if err == nil {
		t.Fatal("expected error: default encryption (repokey) requires a passphrase")
	}

	config, err = ParseConfig([]byte(`{"encryption":"none"}`))
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}

	if config.GlobalRepoName != DefaultGlobalRepoName {
		t.Fatalf("expected default global_repo_name %q, got %q", DefaultGlobalRepoName, config.GlobalRepoName)
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

func TestBackend_Backup_InitializesRepoThenCreatesArchive(t *testing.T) {
	runner := &fakeEnvRunner{}
	fs := newFakeFileSystem()
	backend := testBackend(runner, fs, &fakeLogger{})
	backend.Config.EncryptedPassphrase = "enc:hunter2"

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 calls (init, create), got %d: %+v", len(runner.calls), runner.calls)
	}

	initCall := runner.calls[0]
	if initCall.args[0] != "init" || initCall.args[1] != "--encryption=repokey" || initCall.args[2] != "/repos/global" {
		t.Fatalf("unexpected init call: %+v", initCall)
	}
	if !containsEnv(initCall.env, "BORG_PASSPHRASE=hunter2") {
		t.Fatalf("expected init call to carry decrypted passphrase, got env %v", initCall.env)
	}

	createCall := runner.calls[1]
	wantArgs := []string{"create", "/repos/global::20260811-103000", "."}
	if !equalArgs(createCall.args, wantArgs) {
		t.Fatalf("expected create args %v, got %v", wantArgs, createCall.args)
	}
	if createCall.dir != "/staging" {
		t.Fatalf("expected create to run in /staging, got %q", createCall.dir)
	}
}

func TestBackend_Backup_SkipsInitWhenRepoAlreadyExists(t *testing.T) {
	runner := &fakeEnvRunner{}
	fs := newFakeFileSystem("/repos/global/config")
	backend := testBackend(runner, fs, &fakeLogger{})
	backend.Config.EncryptedPassphrase = "enc:hunter2"

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(runner.calls) != 1 || runner.calls[0].args[0] != "create" {
		t.Fatalf("expected only a create call, got %+v", runner.calls)
	}
}

func TestBackend_Backup_NoneEncryptionSetsNoPassphraseEnv(t *testing.T) {
	runner := &fakeEnvRunner{}
	fs := newFakeFileSystem("/repos/global/config")
	backend := testBackend(runner, fs, &fakeLogger{})
	backend.Config.Encryption = "none"

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(runner.calls[0].env) != 0 {
		t.Fatalf("expected no env vars for encryption none, got %v", runner.calls[0].env)
	}
}

func TestBackend_Backup_AppliesCompressionFlag(t *testing.T) {
	runner := &fakeEnvRunner{}
	fs := newFakeFileSystem("/repos/global/config")
	backend := testBackend(runner, fs, &fakeLogger{})
	backend.Config.Encryption = "none"
	backend.Config.Compression = "zstd,6"

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	wantArgs := []string{"create", "--compression", "zstd,6", "/repos/global::20260811-103000", "."}
	if !equalArgs(runner.calls[0].args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, runner.calls[0].args)
	}
}

func TestBackend_Backup_MissingEnvSupportReturnsError(t *testing.T) {
	backend := Backend{
		Config:    Config{GlobalRepoName: "global", Encryption: "none"},
		ReposRoot: "/repos",
		Runner:    runnerWithoutEnvSupport{},
		FS:        newFakeFileSystem(),
	}

	if err := backend.Backup("/staging"); err == nil {
		t.Fatal("expected error when the command runner doesn't support env vars")
	}
}

func TestBackend_Restore_DryRunMakesNoCalls(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, newFakeFileSystem("/repos/global/config"), &fakeLogger{})
	backend.Options = &shared.Options{DryRun: true}

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no runner calls in dry-run, got %+v", runner.calls)
	}
}

func TestBackend_Restore_RepoNotInitializedIsGracefulNoOp(t *testing.T) {
	runner := &fakeEnvRunner{}
	logger := &fakeLogger{}
	backend := testBackend(runner, newFakeFileSystem(), logger)

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no calls when repo doesn't exist, got %+v", runner.calls)
	}

	if !hasMessageContaining(logger.messages, "does not exist yet") {
		t.Fatalf("expected a log message about the missing repo, got %v", logger.messages)
	}
}

func TestBackend_Restore_EmptyRepoIsGracefulNoOp(t *testing.T) {
	runner := &fakeEnvRunner{
		outputs: map[string][]byte{
			"list /repos/global --last 1 --short": []byte(""),
		},
	}
	logger := &fakeLogger{}
	backend := testBackend(runner, newFakeFileSystem("/repos/global/config"), logger)
	backend.Config.EncryptedPassphrase = "enc:hunter2"

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if !hasMessageContaining(logger.messages, "no archives yet") {
		t.Fatalf("expected a log message about no archives, got %v", logger.messages)
	}
}

func TestBackend_Restore_ExtractsLatestArchive(t *testing.T) {
	runner := &fakeEnvRunner{
		outputs: map[string][]byte{
			"list /repos/global --last 1 --short": []byte("20260810-020000\n"),
		},
	}
	fs := newFakeFileSystem("/repos/global/config")
	backend := testBackend(runner, fs, &fakeLogger{})
	backend.Config.EncryptedPassphrase = "enc:hunter2"

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 calls (list, extract), got %d: %+v", len(runner.calls), runner.calls)
	}

	extractCall := runner.calls[1]
	wantArgs := []string{"extract", "/repos/global::20260810-020000"}
	if !equalArgs(extractCall.args, wantArgs) {
		t.Fatalf("expected extract args %v, got %v", wantArgs, extractCall.args)
	}
	if extractCall.dir != "/staging" {
		t.Fatalf("expected extract to run in /staging, got %q", extractCall.dir)
	}

	found := false
	for _, dir := range fs.mkdirs {
		if dir == "/staging" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected staging directory to be created before extract, mkdirs: %v", fs.mkdirs)
	}
}

// --- BackupGroups / RestoreGroups ---

func TestBackend_BackupGroups_ArchivesEachGroupAndTheGlobalRepo(t *testing.T) {
	runner := &fakeEnvRunner{}
	fs := newFakeFileSystem()
	backend := testBackend(runner, fs, &fakeLogger{})
	backend.Config.EncryptedPassphrase = "enc:hunter2"

	groups := []shared.BackendGroup{
		{Name: "paperless", Paths: []string{"data/paperless", "data/paperless_db"}},
		{Name: "adguard", Paths: []string{"config/adguard"}},
	}

	if err := backend.BackupGroups("/staging", groups); err != nil {
		t.Fatalf("BackupGroups returned error: %v", err)
	}

	var createCalls []fakeCall
	for _, call := range runner.calls {
		if call.args[0] == "create" {
			createCalls = append(createCalls, call)
		}
	}

	if len(createCalls) != 3 {
		t.Fatalf("expected 3 create calls (paperless, adguard, global), got %d: %+v", len(createCalls), createCalls)
	}

	if !equalArgs(createCalls[0].args[1:], []string{"/repos/paperless::20260811-103000", "data/paperless", "data/paperless_db"}) {
		t.Fatalf("unexpected paperless create args: %v", createCalls[0].args)
	}

	if !equalArgs(createCalls[1].args[1:], []string{"/repos/adguard::20260811-103000", "config/adguard"}) {
		t.Fatalf("unexpected adguard create args: %v", createCalls[1].args)
	}

	if !equalArgs(createCalls[2].args[1:], []string{"/repos/global::20260811-103000", "."}) {
		t.Fatalf("unexpected global create args: %v", createCalls[2].args)
	}
}

func TestBackend_BackupGroups_SkipsGroupWithNoPaths(t *testing.T) {
	runner := &fakeEnvRunner{}
	logger := &fakeLogger{}
	backend := testBackend(runner, newFakeFileSystem(), logger)
	backend.Config.EncryptedPassphrase = "enc:hunter2"

	groups := []shared.BackendGroup{{Name: "empty-group"}}

	if err := backend.BackupGroups("/staging", groups); err != nil {
		t.Fatalf("BackupGroups returned error: %v", err)
	}

	for _, call := range runner.calls {
		if call.args[0] == "create" && strings.Contains(call.args[1], "/repos/empty-group::") {
			t.Fatalf("expected no archive created for the empty group, got %+v", call)
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

func TestBackend_RestoreGroups_ExtractsFromEachGroupRepoOnly(t *testing.T) {
	runner := &fakeEnvRunner{
		outputs: map[string][]byte{
			"list /repos/paperless --last 1 --short": []byte("20260810-020000\n"),
			"list /repos/adguard --last 1 --short":   []byte("20260810-030000\n"),
		},
	}
	fs := newFakeFileSystem("/repos/paperless/config", "/repos/adguard/config")
	backend := testBackend(runner, fs, &fakeLogger{})
	backend.Config.EncryptedPassphrase = "enc:hunter2"

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

	var extractArchives []string
	for _, call := range runner.calls {
		if call.args[0] == "extract" {
			extractArchives = append(extractArchives, call.args[1])
		}
	}

	want := []string{"/repos/paperless::20260810-020000", "/repos/adguard::20260810-030000"}
	if !equalArgs(extractArchives, want) {
		t.Fatalf("expected extract archives %v, got %v", want, extractArchives)
	}
}

// --- Name ---

func TestBackend_Name(t *testing.T) {
	if (Backend{}).Name() != "borg" {
		t.Fatalf("expected name %q, got %q", "borg", (Backend{}).Name())
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

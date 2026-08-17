package restic

import (
	"dackup/internal/backend/restic/storage/azure"
	"dackup/internal/backend/restic/storage/b2"
	"dackup/internal/backend/restic/storage/gcs"
	"dackup/internal/backend/restic/storage/rclone"
	"dackup/internal/backend/restic/storage/rest"
	"dackup/internal/backend/restic/storage/s3"
	"dackup/internal/backend/restic/storage/sftp"
	"dackup/internal/backend/restic/storage/swift"
	"dackup/internal/shared"
	"fmt"
	"os"
	"strings"
	"testing"
)

// fakeCall records one invocation made through fakeEnvRunner, distinguishing
// RunInDirWithEnv (dir/env set) from OutputWithEnv (env set, dir always
// empty).
type fakeCall struct {
	dir  string
	env  []string
	name string
	args []string
}

type fakeEnvRunner struct {
	calls   []fakeCall
	runErrs map[string]error
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

	return nil
}

func (r *fakeEnvRunner) OutputWithEnv(env []string, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, fakeCall{env: env, name: name, args: args})

	key := strings.Join(args, " ")
	if err, ok := r.outErrs[key]; ok {
		return nil, err
	}

	return nil, nil
}

// runnerWithoutEnvSupport implements shared.CommandRunner but not
// shared.EnvCommandRunner, for testing the capability-missing error path.
type runnerWithoutEnvSupport struct{}

func (runnerWithoutEnvSupport) Run(name string, args ...string) error              { return nil }
func (runnerWithoutEnvSupport) Output(name string, args ...string) ([]byte, error) { return nil, nil }
func (runnerWithoutEnvSupport) LookPath(file string) (string, error)               { return file, nil }

type fakeFileSystem struct {
	mkdirs []string
}

func (fs *fakeFileSystem) Stat(name string) (os.FileInfo, error) { return nil, os.ErrNotExist }

func (fs *fakeFileSystem) MkdirAll(path string, perm os.FileMode) error {
	fs.mkdirs = append(fs.mkdirs, path)
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

// --- Config ---

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
}

func TestConfig_ValidateSucceedsWithFilesystemDefault(t *testing.T) {
	config := Config{GlobalRepoName: "global", EncryptedPassword: "enc:secret"}
	if err := config.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestParseConfig_AppliesDefaults(t *testing.T) {
	if _, err := ParseConfig(nil); err == nil {
		t.Fatal("expected error: encrypted_password is required")
	}

	config, err := ParseConfig([]byte(`{"encrypted_password":"enc:secret"}`))
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

func TestParseConfig_UnknownStorageTypeReturnsError(t *testing.T) {
	_, err := ParseConfig([]byte(`{"encrypted_password":"enc:secret","storage_type":"dropbox"}`))
	if err == nil {
		t.Fatal("expected error for unknown storage_type")
	}
}

func TestConfig_Provider_MissingSettingsBlockReturnsError(t *testing.T) {
	for _, storageType := range []string{"s3", "sftp", "b2", "azure", "gcs", "rclone", "rest", "swift"} {
		config := Config{StorageType: storageType}
		if _, err := config.provider(""); err == nil {
			t.Fatalf("expected error for storage_type %q with no matching settings block", storageType)
		}
	}
}

func TestConfig_Provider_ResolvesEachStorageType(t *testing.T) {
	config := Config{
		StorageType: "s3",
		S3:          &s3.Storage{Endpoint: "s3.example.com", Bucket: "b", AccessKeyID: "id", EncryptedSecretAccessKey: "enc:k"},
	}
	if _, err := config.provider(""); err != nil {
		t.Fatalf("s3 provider returned error: %v", err)
	}

	config = Config{StorageType: "sftp", SFTP: &sftp.Storage{Host: "h", Username: "u", Path: "/p"}}
	if _, err := config.provider(""); err != nil {
		t.Fatalf("sftp provider returned error: %v", err)
	}

	config = Config{StorageType: "b2", B2: &b2.Storage{Bucket: "b", AccountID: "id", EncryptedAccountKey: "enc:k"}}
	if _, err := config.provider(""); err != nil {
		t.Fatalf("b2 provider returned error: %v", err)
	}

	config = Config{StorageType: "azure", Azure: &azure.Storage{Container: "c", AccountName: "a", EncryptedAccountKey: "enc:k"}}
	if _, err := config.provider(""); err != nil {
		t.Fatalf("azure provider returned error: %v", err)
	}

	config = Config{StorageType: "gcs", GCS: &gcs.Storage{Bucket: "b", CredentialsFilePath: "/creds.json"}}
	if _, err := config.provider(""); err != nil {
		t.Fatalf("gcs provider returned error: %v", err)
	}

	config = Config{StorageType: "rclone", Rclone: &rclone.Storage{RemoteName: "r"}}
	if _, err := config.provider(""); err != nil {
		t.Fatalf("rclone provider returned error: %v", err)
	}

	config = Config{StorageType: "rest", Rest: &rest.Storage{URL: "https://backup.example.com"}}
	if _, err := config.provider(""); err != nil {
		t.Fatalf("rest provider returned error: %v", err)
	}

	config = Config{StorageType: "swift", Swift: &swift.Storage{Container: "c", AuthURL: "https://keystone.example.com", Username: "u", EncryptedPassword: "enc:p"}}
	if _, err := config.provider(""); err != nil {
		t.Fatalf("swift provider returned error: %v", err)
	}
}

// --- Backup / Restore (plain, ungrouped) ---

func TestBackend_Backup_DryRunMakesNoCalls(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, &fakeFileSystem{}, &fakeLogger{})
	backend.Options = &shared.Options{DryRun: true}

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no runner calls in dry-run, got %+v", runner.calls)
	}
}

func TestBackend_Backup_InitializesRepoThenBacksUp(t *testing.T) {
	runner := &fakeEnvRunner{
		outErrs: map[string]error{"cat config -r /repos/global": fmt.Errorf("repository does not exist")},
	}
	backend := testBackend(runner, &fakeFileSystem{}, &fakeLogger{})

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected 3 calls (cat config, init, backup), got %d: %+v", len(runner.calls), runner.calls)
	}

	catCall := runner.calls[0]
	if !equalArgs(catCall.args, []string{"cat", "config", "-r", "/repos/global"}) {
		t.Fatalf("unexpected cat config call: %+v", catCall)
	}
	if !containsEnv(catCall.env, "RESTIC_PASSWORD=hunter2") {
		t.Fatalf("expected cat config call to carry decrypted password, got env %v", catCall.env)
	}

	initCall := runner.calls[1]
	if !equalArgs(initCall.args, []string{"init", "-r", "/repos/global"}) {
		t.Fatalf("unexpected init call: %+v", initCall)
	}

	backupCall := runner.calls[2]
	wantArgs := []string{"backup", "-r", "/repos/global", "."}
	if !equalArgs(backupCall.args, wantArgs) {
		t.Fatalf("expected backup args %v, got %v", wantArgs, backupCall.args)
	}
	if backupCall.dir != "/staging" {
		t.Fatalf("expected backup to run in /staging, got %q", backupCall.dir)
	}
}

func TestBackend_Backup_SkipsInitWhenRepoAlreadyExists(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, &fakeFileSystem{}, &fakeLogger{})

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 calls (cat config, backup), got %d: %+v", len(runner.calls), runner.calls)
	}

	if runner.calls[1].args[0] != "backup" {
		t.Fatalf("expected second call to be backup, got %+v", runner.calls[1])
	}
}

func TestBackend_Backup_InvalidStorageTypeReturnsError(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, &fakeFileSystem{}, &fakeLogger{})
	backend.Config.StorageType = "dropbox"

	if err := backend.Backup("/staging"); err == nil {
		t.Fatal("expected error for an unresolvable storage type")
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no runner calls when the storage type can't be resolved, got %+v", runner.calls)
	}
}

func TestBackend_Backup_UndecryptablePasswordReturnsError(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, &fakeFileSystem{}, &fakeLogger{})
	backend.Config.EncryptedPassword = "not-encrypted-with-fakeSecretStore"

	if err := backend.Backup("/staging"); err == nil {
		t.Fatal("expected error when the password can't be decrypted")
	}
}

func TestBackend_Backup_MissingEnvSupportReturnsError(t *testing.T) {
	backend := Backend{
		Config:    Config{GlobalRepoName: "global", EncryptedPassword: "enc:secret"},
		ReposRoot: "/repos",
		Runner:    runnerWithoutEnvSupport{},
		Secrets:   fakeSecretStore{},
		FS:        &fakeFileSystem{},
	}

	if err := backend.Backup("/staging"); err == nil {
		t.Fatal("expected error when the command runner doesn't support env vars")
	}
}

func TestBackend_Restore_DryRunMakesNoCalls(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, &fakeFileSystem{}, &fakeLogger{})
	backend.Options = &shared.Options{DryRun: true}

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no runner calls in dry-run, got %+v", runner.calls)
	}
}

func TestBackend_Restore_RepoNotInitializedIsGracefulNoOp(t *testing.T) {
	runner := &fakeEnvRunner{
		outErrs: map[string]error{"cat config -r /repos/global": fmt.Errorf("repository does not exist")},
	}
	logger := &fakeLogger{}
	backend := testBackend(runner, &fakeFileSystem{}, logger)

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected only the cat config probe call, got %+v", runner.calls)
	}

	if !hasMessageContaining(logger.messages, "does not exist yet") {
		t.Fatalf("expected a log message about the missing repo, got %v", logger.messages)
	}
}

func TestBackend_Restore_RestoresLatestSnapshot(t *testing.T) {
	runner := &fakeEnvRunner{}
	fs := &fakeFileSystem{}
	backend := testBackend(runner, fs, &fakeLogger{})

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 calls (cat config, restore), got %d: %+v", len(runner.calls), runner.calls)
	}

	restoreCall := runner.calls[1]
	wantArgs := []string{"restore", "latest", "-r", "/repos/global", "--target", "/staging"}
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
		t.Fatalf("expected staging directory to be created before restore, mkdirs: %v", fs.mkdirs)
	}
}

// --- BackupGroups / RestoreGroups ---

func TestBackend_BackupGroups_SnapshotsEachGroupAndTheGlobalRepo(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, &fakeFileSystem{}, &fakeLogger{})

	groups := []shared.BackendGroup{
		{Name: "paperless", Paths: []string{"data/paperless", "data/paperless_db"}},
		{Name: "adguard", Paths: []string{"config/adguard"}},
	}

	if err := backend.BackupGroups("/staging", groups); err != nil {
		t.Fatalf("BackupGroups returned error: %v", err)
	}

	var backupCalls []fakeCall
	for _, call := range runner.calls {
		if call.args[0] == "backup" {
			backupCalls = append(backupCalls, call)
		}
	}

	if len(backupCalls) != 3 {
		t.Fatalf("expected 3 backup calls (paperless, adguard, global), got %d: %+v", len(backupCalls), backupCalls)
	}

	if !equalArgs(backupCalls[0].args[1:], []string{"-r", "/repos/paperless", "data/paperless", "data/paperless_db"}) {
		t.Fatalf("unexpected paperless backup args: %v", backupCalls[0].args)
	}

	if !equalArgs(backupCalls[1].args[1:], []string{"-r", "/repos/adguard", "config/adguard"}) {
		t.Fatalf("unexpected adguard backup args: %v", backupCalls[1].args)
	}

	if !equalArgs(backupCalls[2].args[1:], []string{"-r", "/repos/global", "."}) {
		t.Fatalf("unexpected global backup args: %v", backupCalls[2].args)
	}
}

func TestBackend_BackupGroups_SkipsGroupWithNoPaths(t *testing.T) {
	runner := &fakeEnvRunner{}
	logger := &fakeLogger{}
	backend := testBackend(runner, &fakeFileSystem{}, logger)

	groups := []shared.BackendGroup{{Name: "empty-group"}}

	if err := backend.BackupGroups("/staging", groups); err != nil {
		t.Fatalf("BackupGroups returned error: %v", err)
	}

	for _, call := range runner.calls {
		if call.args[0] == "backup" && strings.Contains(strings.Join(call.args, " "), "/repos/empty-group") {
			t.Fatalf("expected no snapshot created for the empty group, got %+v", call)
		}
	}

	if !hasMessageContaining(logger.messages, "No paths configured for group empty-group") {
		t.Fatalf("expected a skip log message, got %v", logger.messages)
	}
}

func TestBackend_BackupGroups_RejectsGroupNameCollidingWithGlobalRepoName(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, &fakeFileSystem{}, &fakeLogger{})

	groups := []shared.BackendGroup{{Name: "global", Paths: []string{"data"}}}

	if err := backend.BackupGroups("/staging", groups); err == nil {
		t.Fatal("expected error when a group name collides with global_repo_name")
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no calls when validation fails up front, got %+v", runner.calls)
	}
}

func TestBackend_RestoreGroups_RestoresFromEachGroupRepoOnly(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, &fakeFileSystem{}, &fakeLogger{})

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

	var restoreRepos []string
	for _, call := range runner.calls {
		if call.args[0] == "restore" {
			restoreRepos = append(restoreRepos, call.args[3])
		}
	}

	want := []string{"/repos/paperless", "/repos/adguard"}
	if !equalArgs(restoreRepos, want) {
		t.Fatalf("expected restore repositories %v, got %v", want, restoreRepos)
	}
}

// --- Name / BinaryName ---

func TestBackend_Name(t *testing.T) {
	if (Backend{}).Name() != "restic" {
		t.Fatalf("expected name %q, got %q", "restic", (Backend{}).Name())
	}
}

func TestBackend_BinaryName_DefaultsToRestic(t *testing.T) {
	if (Backend{}).BinaryName() != "restic" {
		t.Fatalf("expected binary name %q, got %q", "restic", (Backend{}).BinaryName())
	}
}

func TestBackend_BinaryName_UsesConfiguredBin(t *testing.T) {
	backend := Backend{Config: Config{Bin: "/usr/local/bin/restic"}}
	if backend.BinaryName() != "/usr/local/bin/restic" {
		t.Fatalf("expected configured bin, got %q", backend.BinaryName())
	}
}

func TestBackend_RestoreGroups_RejectsGroupNameCollidingWithGlobalRepoName(t *testing.T) {
	runner := &fakeEnvRunner{}
	backend := testBackend(runner, &fakeFileSystem{}, &fakeLogger{})

	groups := []shared.BackendGroup{{Name: "global", Paths: []string{"data"}}}

	if err := backend.RestoreGroups("/staging", groups); err == nil {
		t.Fatal("expected error when a group name collides with global_repo_name")
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no calls when validation fails up front, got %+v", runner.calls)
	}
}

func TestBackend_UnsetDependencies_UseRealDefaults(t *testing.T) {
	backend := Backend{}

	if _, ok := backend.fileSystem().(shared.OSFileSystem); !ok {
		t.Fatalf("expected default FileSystem to be OSFileSystem, got %#v", backend.fileSystem())
	}

	if _, ok := backend.runner().(shared.OSCommandRunner); !ok {
		t.Fatalf("expected default CommandRunner to be OSCommandRunner, got %#v", backend.runner())
	}

	if _, ok := backend.secretStore().(shared.AESFileSecretStore); !ok {
		t.Fatalf("expected default SecretStore to be AESFileSecretStore, got %#v", backend.secretStore())
	}

	// log must not panic when Logger is nil.
	backend.log("INFO", "no logger configured")
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

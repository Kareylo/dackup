//go:build integration

// Borg's integration test drives the real borg CLI against local temp
// directories, verifying dackup's actual invocations work against a real
// binary rather than just against fakeEnvRunner's scripted expectations in
// borg_test.go. Unlike Kopia's per-storage-type integration tests
// (internal/backend/kopia/storage/*/integration_*_test.go), Borg only ever
// talks to a local repository directory — there is no remote storage type
// to emulate, so this needs no docker compose container, just the borg
// binary itself. See AGENTS.md's "Borg integration tests" section.
package borg

import (
	"bytes"
	"dackup/internal/shared"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireBorgBinary skips the test if the borg CLI isn't on PATH.
func requireBorgBinary(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("borg"); err != nil {
		t.Skip("borg binary not found on PATH; skipping integration test")
	}
}

type integrationLogger struct {
	t *testing.T
}

func (logger integrationLogger) Log(level string, message string) {
	logger.t.Logf("[%s] %s", level, message)
}

// capturingCommandRunner runs real commands via os/exec, the same as
// shared.OSCommandRunner, but folds each command's combined stdout+stderr
// into any error it returns and echoes it via t.Logf (visible with
// "go test -v"). shared.OSCommandRunner — what production code and this
// package's own unit tests otherwise default to — discards output
// entirely, which turns a real borg failure into an undiagnosable "exit
// status 1". Mirrors internal/backend/kopia/integration_helpers.go's type
// of the same name; not shared with it since borg has no cross-package
// storage-type boundary to justify a non-test helper file.
type capturingCommandRunner struct {
	t *testing.T
}

func (r capturingCommandRunner) Run(name string, args ...string) error {
	return r.RunInDirWithEnv("", nil, name, args...)
}

func (r capturingCommandRunner) Output(name string, args ...string) ([]byte, error) {
	return r.OutputWithEnv(nil, name, args...)
}

func (r capturingCommandRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (r capturingCommandRunner) RunInDirWithEnv(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), env...)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	r.t.Logf("$ %s %s\n%s", name, strings.Join(args, " "), strings.TrimSpace(output.String()))

	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(output.String()))
	}

	return nil
}

func (r capturingCommandRunner) OutputWithEnv(env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	r.t.Logf("$ %s %s\n%s", name, strings.Join(args, " "), strings.TrimSpace(stdout.String()+stderr.String()))

	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

// newIntegrationBackend builds a Backend against config, using the real
// borg CLI and filesystem, rooted at a fresh t.TempDir() repository root.
// Secrets is a fresh AESFileSecretStore keyed under its own t.TempDir()
// rather than the real ~/.config/dackup/secret.key or the committed
// test/secret.key fixture — borg needs no portable, cross-machine fixture
// the way Kopia's remote-storage integration tests do (nothing here
// depends on credentials generated on a different machine), so a
// passphrase can just be generated fresh inside each test that needs one.
func newIntegrationBackend(t *testing.T, config Config) Backend {
	t.Helper()

	return Backend{
		Config:    config,
		ReposRoot: t.TempDir(),
		Runner:    capturingCommandRunner{t: t},
		Logger:    integrationLogger{t: t},
		Options:   &shared.Options{},
		Secrets:   shared.AESFileSecretStore{KeyPath: filepath.Join(t.TempDir(), "secret.key")},
	}
}

// writeIntegrationTestFile creates a fresh staging directory containing one
// known file, returning the directory, the file's path, and its content.
func writeIntegrationTestFile(t *testing.T) (stagingDir string, testFilePath string, testContent []byte) {
	t.Helper()

	stagingDir = t.TempDir()
	testFilePath = filepath.Join(stagingDir, "hello.txt")
	testContent = []byte("hello from dackup's borg integration test\n")

	if err := os.WriteFile(testFilePath, testContent, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	return stagingDir, testFilePath, testContent
}

// verifyRestoreRoundTrip deletes testFilePath (proving restore actually
// restores it, rather than the assertion trivially passing because the
// file was never removed), then restores stagingDir and checks its
// content matches testContent. Backup and Restore must be called with the
// same stagingDir, matching real dackup usage where restore always writes
// back into the same data_dir a backup was taken from.
func verifyRestoreRoundTrip(t *testing.T, backend Backend, stagingDir string, testFilePath string, testContent []byte) {
	t.Helper()

	if err := os.Remove(testFilePath); err != nil {
		t.Fatalf("failed to remove test file before restore: %v", err)
	}

	if err := backend.Restore(stagingDir); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	restoredContent, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}

	if string(restoredContent) != string(testContent) {
		t.Fatalf("restored content %q does not match original %q", restoredContent, testContent)
	}
}

func TestIntegration_Borg_UnencryptedBackupRestoreRoundTrip(t *testing.T) {
	requireBorgBinary(t)

	backend := newIntegrationBackend(t, Config{
		GlobalRepoName: DefaultGlobalRepoName,
		Encryption:     "none",
	})

	stagingDir, testFilePath, testContent := writeIntegrationTestFile(t)

	if err := backend.Backup(stagingDir); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	verifyRestoreRoundTrip(t, backend, stagingDir, testFilePath, testContent)
}

func TestIntegration_Borg_EncryptedBackupRestoreRoundTrip(t *testing.T) {
	requireBorgBinary(t)

	backend := newIntegrationBackend(t, Config{
		GlobalRepoName: DefaultGlobalRepoName,
		Encryption:     "repokey",
	})

	encryptedPassphrase, err := backend.Secrets.Encrypt("integration-test-passphrase")
	if err != nil {
		t.Fatalf("failed to encrypt test passphrase: %v", err)
	}
	backend.Config.EncryptedPassphrase = encryptedPassphrase

	stagingDir, testFilePath, testContent := writeIntegrationTestFile(t)

	if err := backend.Backup(stagingDir); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	verifyRestoreRoundTrip(t, backend, stagingDir, testFilePath, testContent)
}

func TestIntegration_Borg_CompressionFlagIsAccepted(t *testing.T) {
	requireBorgBinary(t)

	backend := newIntegrationBackend(t, Config{
		GlobalRepoName: DefaultGlobalRepoName,
		Encryption:     "none",
		Compression:    "zstd,6",
	})

	stagingDir, testFilePath, testContent := writeIntegrationTestFile(t)

	if err := backend.Backup(stagingDir); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	verifyRestoreRoundTrip(t, backend, stagingDir, testFilePath, testContent)
}

func TestIntegration_Borg_GroupedBackupRestoreRoundTrip(t *testing.T) {
	requireBorgBinary(t)

	backend := newIntegrationBackend(t, Config{
		GlobalRepoName: DefaultGlobalRepoName,
		Encryption:     "none",
	})

	stagingDir := t.TempDir()
	groupAPath := filepath.Join(stagingDir, "app-a")
	groupBPath := filepath.Join(stagingDir, "app-b")

	if err := os.MkdirAll(groupAPath, 0o755); err != nil {
		t.Fatalf("failed to create group a directory: %v", err)
	}
	if err := os.MkdirAll(groupBPath, 0o755); err != nil {
		t.Fatalf("failed to create group b directory: %v", err)
	}

	fileA := filepath.Join(groupAPath, "a.txt")
	fileB := filepath.Join(groupBPath, "b.txt")
	contentA := []byte("group a content\n")
	contentB := []byte("group b content\n")

	if err := os.WriteFile(fileA, contentA, 0o644); err != nil {
		t.Fatalf("failed to write group a file: %v", err)
	}
	if err := os.WriteFile(fileB, contentB, 0o644); err != nil {
		t.Fatalf("failed to write group b file: %v", err)
	}

	groups := []shared.BackendGroup{
		{Name: "group-a", Paths: []string{"app-a"}},
		{Name: "group-b", Paths: []string{"app-b"}},
	}

	if err := backend.BackupGroups(stagingDir, groups); err != nil {
		t.Fatalf("BackupGroups returned error: %v", err)
	}

	if err := os.Remove(fileA); err != nil {
		t.Fatalf("failed to remove group a file before restore: %v", err)
	}
	if err := os.Remove(fileB); err != nil {
		t.Fatalf("failed to remove group b file before restore: %v", err)
	}

	if err := backend.RestoreGroups(stagingDir, groups); err != nil {
		t.Fatalf("RestoreGroups returned error: %v", err)
	}

	gotA, err := os.ReadFile(fileA)
	if err != nil {
		t.Fatalf("failed to read restored group a file: %v", err)
	}
	if string(gotA) != string(contentA) {
		t.Fatalf("restored group a content %q does not match original %q", gotA, contentA)
	}

	gotB, err := os.ReadFile(fileB)
	if err != nil {
		t.Fatalf("failed to read restored group b file: %v", err)
	}
	if string(gotB) != string(contentB) {
		t.Fatalf("restored group b content %q does not match original %q", gotB, contentB)
	}
}

// TestIntegration_Borg_RestoreFromUninitializedRepoIsGracefulNoOp mirrors
// TestBackend_Restore_RepoNotInitializedIsGracefulNoOp in borg_test.go
// against the real CLI: restoring from a repository that was never backed
// up to should warn and return nil, not error, matching the same
// no-op-on-first-restore behavior the filesystem-only unit test asserts.
func TestIntegration_Borg_RestoreFromUninitializedRepoIsGracefulNoOp(t *testing.T) {
	requireBorgBinary(t)

	backend := newIntegrationBackend(t, Config{
		GlobalRepoName: DefaultGlobalRepoName,
		Encryption:     "none",
	})

	stagingDir := t.TempDir()

	if err := backend.Restore(stagingDir); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
}

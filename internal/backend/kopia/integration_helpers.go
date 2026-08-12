//go:build integration

// Package kopia's integration test support lives here rather than in a
// _test.go file because it's shared across package boundaries: each
// storage type's integration test (internal/backend/kopia/storage/<type>/
// integration_<type>_test.go) needs to build a real kopia.Backend and drive
// it, but Go test files are only visible within their own package's test
// binary — a _test.go file in package kopia is invisible to package
// azure_test even though azure_test imports kopia. Putting these helpers in
// an ordinary (exported, non-test) file, still gated by the "integration"
// build tag so it never ships in a normal build, makes them importable from
// every storage subpackage's external test package. See AGENTS.md's "Kopia
// storage integration tests" section.
package kopia

import (
	"bytes"
	"dackup/internal/shared"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// IntegrationConfigPath resolves a path under test/ to the corresponding
// config.<type>.json fixture. The relative depth (five levels) is fixed at
// the storage subpackage directory a per-backend integration test actually
// runs from — internal/backend/kopia/storage/<type> -> storage -> kopia ->
// backend -> internal -> repo root — since "go test" sets the working
// directory to the package under test, not to wherever this helper
// function happens to be defined.
func IntegrationConfigPath(name string) string {
	return filepath.Join("..", "..", "..", "..", "..", "test", name)
}

// integrationSecretStore decrypts the fixture configs' encrypted_* fields.
// It's a dedicated test-only key committed at test/secret.key — never the
// real ~/.config/dackup/secret.key — so the fixtures decrypt identically
// on any machine that clones the repo, not just the one that generated
// them.
func integrationSecretStore() shared.SecretStore {
	return shared.AESFileSecretStore{KeyPath: IntegrationConfigPath("secret.key")}
}

// testLogger routes Backend's log messages through t.Logf, so verbose
// kopia CLI output only shows up with "go test -v".
type testLogger struct {
	t *testing.T
}

func (logger testLogger) Log(level string, message string) {
	logger.t.Logf("[%s] %s", level, message)
}

// capturingCommandRunner runs real commands via os/exec, the same as
// shared.OSCommandRunner, but folds each command's combined stdout+stderr
// into any error it returns and echoes it via t.Logf (visible with
// "go test -v"). shared.OSCommandRunner — what production code and this
// package's own unit tests otherwise default to — discards output
// entirely, which turns a real kopia failure into an undiagnosable "exit
// status 1" (see Backend.ensureRepo's error wrapping, which only has
// whatever the underlying error's Error() text already contains). Secret-
// bearing flag values (--*password*=, --*key*=, --*secret*=) are redacted
// before logging or including in the error, in case this is ever pointed
// at real, non-fixture credentials.
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
	r.log(name, args, output.String())

	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(redactSecretArgs(output.String())))
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
	r.log(name, args, stdout.String()+stderr.String())

	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(redactSecretArgs(stderr.String())))
	}

	return stdout.Bytes(), nil
}

func (r capturingCommandRunner) log(name string, args []string, output string) {
	r.t.Logf("$ %s %s\n%s", name, redactSecretArgs(strings.Join(args, " ")), strings.TrimSpace(output))
}

// redactSecretArgs masks the value of any "--flag=value" whose flag name
// contains "password", "key", or "secret" (case-insensitive) — covers
// every secret-bearing flag this package's storage types pass on the
// command line (--sftp-password, --webdav-password, --storage-key,
// --key for b2, ...). Command output is passed through this too, since
// kopia sometimes echoes the command line it ran into its own error text.
func redactSecretArgs(text string) string {
	fields := strings.Fields(text)
	for i, field := range fields {
		flag, _, ok := strings.Cut(field, "=")
		if !ok || !strings.HasPrefix(flag, "--") {
			continue
		}

		lower := strings.ToLower(flag)
		if strings.Contains(lower, "password") || strings.Contains(lower, "key") || strings.Contains(lower, "secret") {
			fields[i] = flag + "=[redacted]"
		}
	}

	return strings.Join(fields, " ")
}

// RequireKopiaBinary skips the test if the kopia CLI isn't on PATH — every
// integration test needs it regardless of which storage type it targets.
func RequireKopiaBinary(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("kopia"); err != nil {
		t.Skip("kopia binary not found on PATH; skipping integration test")
	}
}

// RequireReachable skips the test if address isn't accepting TCP
// connections within a short timeout — the corresponding test/compose.yml
// container is presumably not running.
func RequireReachable(t *testing.T, address string) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		t.Skipf("%s is not reachable (%v); is docker compose -f test/compose.yml up -d running?", address, err)
	}
	conn.Close()
}

// LoadIntegrationConfig reads and parses the kopia settings out of the
// DackupConfig fixture at test/<name>, decrypting its encrypted_* fields
// via integrationSecretStore.
func LoadIntegrationConfig(t *testing.T, name string) Config {
	t.Helper()

	dackupConfig, err := shared.ReadDackupConfig(IntegrationConfigPath(name))
	if err != nil {
		t.Fatalf("failed to read %s: %v", name, err)
	}

	kopiaConfig, err := ParseConfig(dackupConfig.BackendSettings)
	if err != nil {
		t.Fatalf("failed to parse kopia settings from %s: %v", name, err)
	}

	return kopiaConfig
}

// NewIntegrationBackend builds a Backend against config, using the real
// kopia CLI and filesystem. ReposRoot is always a fresh t.TempDir() rather
// than the fixture's own backend_dir (which names a real path like
// /opt/apps_docker/kopia-repos that may not exist on the machine running
// the test) — this only affects where the local per-repository
// .kopia-config file lives, not the remote repository identity, so it's
// safe to override for test isolation without needing that directory to
// pre-exist.
func NewIntegrationBackend(t *testing.T, config Config) Backend {
	t.Helper()

	return Backend{
		Config:    config,
		ReposRoot: t.TempDir(),
		Runner:    capturingCommandRunner{t: t},
		Logger:    testLogger{t: t},
		Options:   &shared.Options{},
		Secrets:   integrationSecretStore(),
		FS:        shared.OSFileSystem{},
	}
}

// RunBackupRestoreRoundTrip backs up a directory containing one known file,
// then verifies the restore side via VerifyRestoreRoundTrip. Used by every
// storage type except azure, which needs to inspect Backup's error before
// deciding whether to fail or skip (see
// storage/azure/integration_azure_test.go) and so calls
// WriteIntegrationTestFile/VerifyRestoreRoundTrip directly instead of going
// through this wrapper.
func RunBackupRestoreRoundTrip(t *testing.T, backend Backend) {
	t.Helper()

	stagingDir, testFilePath, testContent := WriteIntegrationTestFile(t)

	if err := backend.Backup(stagingDir); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	VerifyRestoreRoundTrip(t, backend, stagingDir, testFilePath, testContent)
}

// WriteIntegrationTestFile creates a fresh staging directory containing one
// known file, returning the directory, the file's path, and its content —
// the fixture RunBackupRestoreRoundTrip (and TestIntegration_Azure
// directly) hands to Backend.Backup.
func WriteIntegrationTestFile(t *testing.T) (stagingDir string, testFilePath string, testContent []byte) {
	t.Helper()

	stagingDir = t.TempDir()
	testFilePath = filepath.Join(stagingDir, "hello.txt")
	testContent = []byte("hello from dackup's kopia integration test\n")

	if err := os.WriteFile(testFilePath, testContent, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	return stagingDir, testFilePath, testContent
}

// VerifyRestoreRoundTrip deletes testFilePath (proving restore actually
// restores it, rather than the assertion trivially passing because the
// file was never removed), then restores stagingDir and checks its content
// matches testContent.
//
// Backup and Restore must be called with the *same* directory path: kopia
// keys a repository's snapshots by the literal source path string given to
// "snapshot create", so Restore only finds a snapshot when called with the
// path Backup used — matching real dackup usage, where restore always
// writes back into the same data_dir a backup was taken from.
func VerifyRestoreRoundTrip(t *testing.T, backend Backend, stagingDir string, testFilePath string, testContent []byte) {
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

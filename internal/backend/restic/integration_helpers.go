//go:build integration

// Package restic's integration test support lives here rather than in a
// _test.go file for the same reason internal/backend/kopia/integration_helpers.go
// does: each storage type's integration test
// (internal/backend/restic/storage/<type>/integration_<type>_test.go) needs
// to build a real restic.Backend and drive it, but Go test files are only
// visible within their own package's test binary. Putting these helpers in
// an ordinary (exported, non-test) file, still gated by the "integration"
// build tag so it never ships in a normal build, makes them importable from
// every storage subpackage's external test package. See AGENTS.md's
// "Restic integration tests" section.
package restic

import (
	"bytes"
	"context"
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
// config.restic-<type>.json fixture, mirroring
// kopia.IntegrationConfigPath's fixed relative depth (five levels: the
// storage subpackage directory a per-backend integration test actually
// runs from — internal/backend/restic/storage/<type> -> storage -> restic
// -> backend -> internal -> repo root).
func IntegrationConfigPath(name string) string {
	return filepath.Join("..", "..", "..", "..", "..", "test", name)
}

// integrationSecretStore decrypts the fixture configs' encrypted_* fields,
// via the same repo-committed test/secret.key kopia's own integration
// tests use — never the real ~/.config/dackup/secret.key — so the fixtures
// decrypt identically on any machine that clones the repo.
func integrationSecretStore() shared.SecretStore {
	return shared.AESFileSecretStore{KeyPath: IntegrationConfigPath("secret.key")}
}

// testLogger routes Backend's log messages through t.Logf, so verbose
// restic CLI output only shows up with "go test -v".
type testLogger struct {
	t *testing.T
}

func (logger testLogger) Log(level string, message string) {
	logger.t.Logf("[%s] %s", level, message)
}

// capturingCommandRunner runs real commands via os/exec, the same as
// shared.OSCommandRunner, but folds each command's combined stdout+stderr
// into any error it returns and echoes it via t.Logf (visible with
// "go test -v") — mirrors kopia.capturingCommandRunner and
// borg.capturingCommandRunner for the same reason: shared.OSCommandRunner
// discards output entirely, which turns a real restic failure into an
// undiagnosable "exit status 1". Secret-bearing flag values (--o
// *password*=, --o *key*=, --o *secret*=, and any env-style KEY=value
// argument whose key contains those words) are redacted before logging, in
// case this is ever pointed at real, non-fixture credentials.
type capturingCommandRunner struct {
	t *testing.T
}

// commandTimeout bounds every restic invocation this runner makes. It must
// stay well under restic's own backend retry ceiling — confirmed against
// restic 0.19.1's internal/global/global.go, every backend is wrapped in
// retry.New(be, 15*time.Minute, ...), and that 15-minute
// exponential-backoff retry budget is hardcoded at that call site with no
// CLI flag or env var to shorten it (--stuck-request-timeout, default 5m,
// is a different thing: it only covers requests that never receive a
// response at all, not requests that fail fast and get retried, which is
// what happens here). Without this timeout, a persistent-looking backend
// error — e.g. TestIntegration_Azure's known, permanent Azurite TLS
// mismatch (see isKnownAzuriteAddressingIncompatibility) — makes restic
// retry for up to 15 minutes before ever returning control to Go, which
// reads as a hung test rather than the graceful, fast skip the test
// actually implements. restic logs each retry attempt's error to
// stdout/stderr as it happens (confirmed directly: a first attempt errors
// within ~1s), so the known-incompatibility signature is already present
// in the captured output long before this timeout fires, letting the test
// still recognize and skip it — just without waiting 15 minutes to do so.
const commandTimeout = 60 * time.Second

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
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
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
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
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

// redactSecretArgs masks the value of any "-o key=value" restic global
// option whose key contains "password", "key", or "secret"
// (case-insensitive) — covers restic's own secret-bearing options (e.g.
// sftp.command embedding a keyfile path is not secret, but nothing else
// this package's storage types pass via -o currently is; kept broad as a
// safety net).
func redactSecretArgs(text string) string {
	fields := strings.Fields(text)
	for i, field := range fields {
		key, _, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}

		lower := strings.ToLower(key)
		if strings.Contains(lower, "password") || strings.Contains(lower, "key") || strings.Contains(lower, "secret") {
			fields[i] = key + "=[redacted]"
		}
	}

	return strings.Join(fields, " ")
}

// RequireResticBinary skips the test if the restic CLI isn't on PATH —
// every integration test needs it regardless of which storage type it
// targets.
func RequireResticBinary(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic binary not found on PATH; skipping integration test")
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

// LoadIntegrationConfig reads and parses the restic settings out of the
// DackupConfig fixture at test/<name>, decrypting its encrypted_* fields
// via integrationSecretStore.
func LoadIntegrationConfig(t *testing.T, name string) Config {
	t.Helper()

	dackupConfig, err := shared.ReadDackupConfig(IntegrationConfigPath(name))
	if err != nil {
		t.Fatalf("failed to read %s: %v", name, err)
	}

	resticConfig, err := ParseConfig(dackupConfig.BackendSettings)
	if err != nil {
		t.Fatalf("failed to parse restic settings from %s: %v", name, err)
	}

	return resticConfig
}

// NewIntegrationBackend builds a Backend against config, using the real
// restic CLI and filesystem. ReposRoot is always a fresh t.TempDir() rather
// than the fixture's own backend_dir, for the same reason
// kopia.NewIntegrationBackend does — it only affects where a local
// filesystem-type repository would live, not a remote repository's
// identity, so it's safe to override for test isolation.
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
// then verifies the restore side via VerifyRestoreRoundTrip.
func RunBackupRestoreRoundTrip(t *testing.T, backend Backend) {
	t.Helper()

	stagingDir, testFilePath, testContent := WriteIntegrationTestFile(t)

	if err := backend.Backup(stagingDir); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	VerifyRestoreRoundTrip(t, backend, stagingDir, testFilePath, testContent)
}

// WriteIntegrationTestFile creates a fresh staging directory containing one
// known file, returning the directory, the file's path, and its content.
func WriteIntegrationTestFile(t *testing.T) (stagingDir string, testFilePath string, testContent []byte) {
	t.Helper()

	stagingDir = t.TempDir()
	testFilePath = filepath.Join(stagingDir, "hello.txt")
	testContent = []byte("hello from dackup's restic integration test\n")

	if err := os.WriteFile(testFilePath, testContent, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	return stagingDir, testFilePath, testContent
}

// VerifyRestoreRoundTrip deletes testFilePath (proving restore actually
// restores it, rather than the assertion trivially passing because the
// file was never removed), then restores stagingDir and checks its content
// matches testContent. Backup and Restore must be called with the same
// stagingDir, matching real dackup usage where restore always writes back
// into the same data_dir a backup was taken from.
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

//go:build integration

// Restic's local (filesystem storage type) integration test drives the
// real restic CLI against local temp directories, verifying dackup's
// actual invocations work against a real binary rather than just against
// fakeEnvRunner's scripted expectations in restic_test.go. Mirrors
// internal/backend/borg/integration_borg_test.go's shape (no docker compose
// container needed here — filesystem storage is local only), but every
// round trip is encrypted, since restic (unlike borg) has no unencrypted
// mode. See AGENTS.md's "Restic integration tests" section.
package restic

import (
	"dackup/internal/shared"
	"os"
	"path/filepath"
	"testing"
)

// newLocalIntegrationBackend builds a Backend for the filesystem storage
// type directly, rather than through NewIntegrationBackend/
// integrationSecretStore — those exist for the storage-subpackage tests,
// which read a portable, committed test/secret.key-encrypted fixture (see
// IntegrationConfigPath's doc comment, whose fixed relative-path depth
// assumes a storage/<type> subpackage, one level deeper than this
// package). The local filesystem test needs no such portable fixture — it
// never leaves this machine — so, like borg's own local integration test
// (internal/backend/borg/integration_borg_test.go), it just generates a
// fresh throwaway passphrase and secret key under its own t.TempDir().
func newLocalIntegrationBackend(t *testing.T) Backend {
	t.Helper()

	backend := Backend{
		Config:    Config{GlobalRepoName: DefaultGlobalRepoName},
		ReposRoot: t.TempDir(),
		Runner:    capturingCommandRunner{t: t},
		Logger:    testLogger{t: t},
		Options:   &shared.Options{},
		Secrets:   shared.AESFileSecretStore{KeyPath: filepath.Join(t.TempDir(), "secret.key")},
		FS:        shared.OSFileSystem{},
	}

	encryptedPassword, err := backend.Secrets.Encrypt("integration-test-password")
	if err != nil {
		t.Fatalf("failed to encrypt test password: %v", err)
	}
	backend.Config.EncryptedPassword = encryptedPassword

	return backend
}

func TestIntegration_Restic_BackupRestoreRoundTrip(t *testing.T) {
	RequireResticBinary(t)

	RunBackupRestoreRoundTrip(t, newLocalIntegrationBackend(t))
}

func TestIntegration_Restic_GroupedBackupRestoreRoundTrip(t *testing.T) {
	RequireResticBinary(t)

	backend := newLocalIntegrationBackend(t)

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

// TestIntegration_Restic_RestoreFromUninitializedRepoIsGracefulNoOp mirrors
// TestBackend_Restore_RepoNotInitializedIsGracefulNoOp in restic_test.go
// against the real CLI: restoring from a repository that was never backed
// up to should warn and return nil, not error.
func TestIntegration_Restic_RestoreFromUninitializedRepoIsGracefulNoOp(t *testing.T) {
	RequireResticBinary(t)

	backend := newLocalIntegrationBackend(t)

	stagingDir := t.TempDir()

	if err := backend.Restore(stagingDir); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
}

// TestIntegration_Restic_RestoreWithWrongPasswordFails checks against the
// real CLI that a repository restic cannot open (here: wrong password,
// exit 12) fails the restore instead of being reported as "nothing to
// restore".
func TestIntegration_Restic_RestoreWithWrongPasswordFails(t *testing.T) {
	RequireResticBinary(t)

	backend := newLocalIntegrationBackend(t)

	stagingDir, _, _ := WriteIntegrationTestFile(t)
	if err := backend.Backup(stagingDir); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	wrongPassword, err := backend.Secrets.Encrypt("not-the-repository-password")
	if err != nil {
		t.Fatalf("failed to encrypt wrong password: %v", err)
	}
	backend.Config.EncryptedPassword = wrongPassword

	if err := backend.Restore(t.TempDir()); err == nil {
		t.Fatal("expected Restore with the wrong password to return an error")
	}
}

// TestIntegration_Restic_RestoreGroupsRemovesFilesMissingFromArchive checks against the real restic CLI that a grouped restore leaves no file
// in a restored path that the archive doesn't have: stray.txt is created in
// app-a after the backup and must be gone after the restore, while
// other/keep.txt, outside every group's paths, must be untouched.
func TestIntegration_Restic_RestoreGroupsRemovesFilesMissingFromArchive(t *testing.T) {
	RequireResticBinary(t)

	backend := newLocalIntegrationBackend(t)
	stagingDir := t.TempDir()
	fileA := filepath.Join(stagingDir, "app-a", "a.txt")
	strayFile := filepath.Join(stagingDir, "app-a", "stray.txt")
	keepFile := filepath.Join(stagingDir, "other", "keep.txt")

	for _, path := range []string{fileA, keepFile} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("content of "+filepath.Base(path)+"\n"), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}

	groups := []shared.BackendGroup{{Name: "group-a", Paths: []string{"app-a"}}}

	if err := backend.BackupGroups(stagingDir, groups); err != nil {
		t.Fatalf("BackupGroups returned error: %v", err)
	}

	if err := os.WriteFile(strayFile, []byte("not in the archive\n"), 0o644); err != nil {
		t.Fatalf("failed to write stray file: %v", err)
	}

	if err := backend.RestoreGroups(stagingDir, groups); err != nil {
		t.Fatalf("RestoreGroups returned error: %v", err)
	}

	if _, err := os.Stat(fileA); err != nil {
		t.Fatalf("expected the archived file to be restored: %v", err)
	}

	if _, err := os.Stat(strayFile); !os.IsNotExist(err) {
		t.Fatalf("expected the file missing from the archive to be gone after restore, stat err = %v", err)
	}

	if _, err := os.Stat(keepFile); err != nil {
		t.Fatalf("expected the file outside every group path to be untouched: %v", err)
	}
}

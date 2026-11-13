//go:build integration

// Integration tests in this file drive the real kopia CLI against local
// filesystem storage, so they need no emulator container, only a "kopia"
// binary (they skip without one). They run via
// "go test -tags=integration ./internal/backend/kopia/...".
package filesystem_test

import (
	"dackup/internal/backend/kopia"
	"dackup/internal/shared"
	"os"
	"path/filepath"
	"testing"
)

// TestIntegration_Filesystem_RestoreGroupsRemovesFilesMissingFromArchive checks against the real kopia CLI that a grouped restore leaves no file
// in a restored path that the archive doesn't have: stray.txt is created in
// app-a after the backup and must be gone after the restore, while
// other/keep.txt, outside every group's paths, must be untouched.
func TestIntegration_Filesystem_RestoreGroupsRemovesFilesMissingFromArchive(t *testing.T) {
	kopia.RequireKopiaBinary(t)

	backend := kopia.NewIntegrationBackend(t, kopia.Config{GlobalRepoName: kopia.DefaultGlobalRepoName})

	encryptedPassword, err := backend.Secrets.Encrypt("integration-test-password")
	if err != nil {
		t.Fatalf("failed to encrypt test password: %v", err)
	}
	backend.Config.EncryptedPassword = encryptedPassword
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

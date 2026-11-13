package kopia

import (
	"dackup/internal/backend/kopia/storage/sftp"
	"dackup/internal/shared"
	"strings"
	"testing"
)

// sftpWarningBackend returns a dry-run Backend using kopia sftp storage, so
// the public methods reach their warning without running any command.
func sftpWarningBackend(storage *sftp.Storage, logger *fakeLogger) Backend {
	backend := testBackend(&fakeEnvRunner{}, newFakeFileSystem(), logger)
	backend.Options = &shared.Options{DryRun: true}
	backend.Config.StorageType = sftp.Name
	backend.Config.SFTP = storage

	return backend
}

func passwordSFTPStorage() *sftp.Storage {
	return &sftp.Storage{Host: "backup.example.com", Username: "dackup", Path: "/srv/backups", EncryptedPassword: "enc:hunter2"}
}

func keyfileSFTPStorage() *sftp.Storage {
	return &sftp.Storage{Host: "backup.example.com", Username: "dackup", Path: "/srv/backups", KeyfilePath: "/home/dackup/.ssh/id_ed25519"}
}

// sftpPasswordWarnings returns the WARN lines about the SFTP password being
// visible in the process list.
func sftpPasswordWarnings(logger *fakeLogger) []string {
	var warnings []string
	for _, message := range logger.messages {
		if strings.HasPrefix(message, "WARN: ") && strings.Contains(message, "process list") {
			warnings = append(warnings, message)
		}
	}

	return warnings
}

var twoGroups = []shared.BackendGroup{
	{Name: "app", Paths: []string{"app"}},
	{Name: "db", Paths: []string{"db"}},
}

func TestBackend_SFTPPasswordAuth_WarnsOncePerCall(t *testing.T) {
	cases := []struct {
		name string
		run  func(Backend) error
	}{
		{"Backup", func(b Backend) error { return b.Backup("/staging") }},
		{"Restore", func(b Backend) error { return b.Restore("/staging") }},
		{"BackupGroups", func(b Backend) error { return b.BackupGroups("/staging", twoGroups) }},
		{"RestoreGroups", func(b Backend) error { return b.RestoreGroups("/staging", twoGroups) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := &fakeLogger{}

			if err := tc.run(sftpWarningBackend(passwordSFTPStorage(), logger)); err != nil {
				t.Fatalf("%s returned error: %v", tc.name, err)
			}

			warnings := sftpPasswordWarnings(logger)
			if len(warnings) != 1 {
				t.Fatalf("expected exactly one process-list warning, got %d: %v", len(warnings), logger.messages)
			}

			if !strings.Contains(warnings[0], "keyfile_path") {
				t.Fatalf("expected the warning to recommend keyfile_path, got: %s", warnings[0])
			}
		})
	}
}

func TestBackend_SFTPPasswordAuth_WarningNeverContainsThePassword(t *testing.T) {
	logger := &fakeLogger{}

	if err := sftpWarningBackend(passwordSFTPStorage(), logger).Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	for _, message := range logger.messages {
		if strings.Contains(message, "hunter2") {
			t.Fatalf("expected no log line to contain the password, got: %s", message)
		}
	}
}

func TestBackend_SFTPKeyfileAuth_DoesNotWarn(t *testing.T) {
	logger := &fakeLogger{}

	if err := sftpWarningBackend(keyfileSFTPStorage(), logger).BackupGroups("/staging", twoGroups); err != nil {
		t.Fatalf("BackupGroups returned error: %v", err)
	}

	if warnings := sftpPasswordWarnings(logger); len(warnings) != 0 {
		t.Fatalf("expected no process-list warning with keyfile auth, got: %v", warnings)
	}
}

func TestBackend_NonSFTPStorage_DoesNotWarn(t *testing.T) {
	logger := &fakeLogger{}
	backend := testBackend(&fakeEnvRunner{}, newFakeFileSystem(), logger)
	backend.Options = &shared.Options{DryRun: true}

	// A leftover SFTP block must not trigger the warning when another
	// storage type is selected.
	backend.Config.SFTP = passwordSFTPStorage()

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if warnings := sftpPasswordWarnings(logger); len(warnings) != 0 {
		t.Fatalf("expected no process-list warning for filesystem storage, got: %v", warnings)
	}
}

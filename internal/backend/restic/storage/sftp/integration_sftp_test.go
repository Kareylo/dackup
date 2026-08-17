//go:build integration

// Integration tests in this file drive the real restic CLI against the
// local emulator containers defined in test/compose.yml (start them with
// "docker compose -f test/compose.yml up -d" first). They're excluded from
// the default "go test ./..." run and only run via
// "go test -tags=integration ./internal/backend/restic/...". Each test
// skips gracefully (rather than failing) when its prerequisite isn't
// reachable.
package sftp_test

import (
	"dackup/internal/backend/restic"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestIntegration_SFTP backs up to and restores from the test_restic_sftp
// service in test/compose.yml, using test/config.restic-sftp.json.
//
// Two fields are overridden after loading the fixture rather than baked
// into the committed JSON:
//
//   - keyfile_path: the fixture's own relative path would resolve against
//     the JSON file's own location if taken literally, not against restic's
//     working directory when it runs, so this points it at the committed
//     throwaway keypair (test/restic_sftp_key/) via
//     restic.IntegrationConfigPath instead — mirrors why kopia's own sftp
//     integration test does the same for known_hosts_path.
//   - known_hosts_path: test_restic_sftp's host key is regenerated on every
//     "docker compose up" (atmoz/sftp doesn't persist it across restarts
//     unless a volume is mounted for /etc/ssh), so this fetches the live
//     host key via ssh-keyscan, exactly like kopia's own sftp integration
//     test already does for the same reason.
func TestIntegration_SFTP(t *testing.T) {
	restic.RequireResticBinary(t)
	restic.RequireReachable(t, "localhost:2223")

	if _, err := exec.LookPath("ssh-keyscan"); err != nil {
		t.Skip("ssh-keyscan not found on PATH; skipping integration test")
	}

	knownHosts, err := exec.Command("ssh-keyscan", "-p", "2223", "-T", "5", "localhost").Output()
	if err != nil || len(knownHosts) == 0 {
		t.Skipf("ssh-keyscan against localhost:2223 failed (%v); is test_restic_sftp up?", err)
	}

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(knownHostsPath, knownHosts, 0o600); err != nil {
		t.Fatalf("failed to write known_hosts file: %v", err)
	}

	config := restic.LoadIntegrationConfig(t, "config.restic-sftp.json")
	config.SFTP.KeyfilePath = restic.IntegrationConfigPath("restic_sftp_key/id_ed25519")
	config.SFTP.KnownHostsPath = knownHostsPath

	backend := restic.NewIntegrationBackend(t, config)

	restic.RunBackupRestoreRoundTrip(t, backend)
}

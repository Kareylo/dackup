//go:build integration

// Integration tests in this file drive the real kopia CLI against the
// local emulator containers defined in test/compose.yml (start them with
// "docker compose -f test/compose.yml up -d" first). They're excluded from
// the default "go test ./..." run — which must stay hermetic and not
// depend on Docker or a "kopia" binary being present — and only run via
// "go test -tags=integration ./internal/backend/kopia/...". Each test
// skips gracefully (rather than failing) when its prerequisite (the kopia
// binary, or the target container) isn't reachable, so the suite degrades
// cleanly on a machine that hasn't started the containers.
package sftp_test

import (
	"dackup/internal/backend/kopia"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestIntegration_SFTP backs up to and restores from the test_sftp service
// in test/compose.yml, using test/config.sftp.json.
//
// The fixture leaves sftp.known_hosts_path empty because the container's
// host key is regenerated on every "docker compose up" (atmoz/sftp doesn't
// persist it across restarts unless a volume is mounted for /etc/ssh) —
// baking a specific key into the committed fixture would go stale the
// moment someone recreates the container. Instead this test fetches the
// live host key via ssh-keyscan and overrides KnownHostsPath with it,
// mirroring how an operator would handle a genuinely ephemeral host in
// practice.
func TestIntegration_SFTP(t *testing.T) {
	kopia.RequireKopiaBinary(t)
	kopia.RequireReachable(t, "localhost:2222")

	if _, err := exec.LookPath("ssh-keyscan"); err != nil {
		t.Skip("ssh-keyscan not found on PATH; skipping integration test")
	}

	knownHosts, err := exec.Command("ssh-keyscan", "-p", "2222", "-T", "5", "localhost").Output()
	if err != nil || len(knownHosts) == 0 {
		t.Skipf("ssh-keyscan against localhost:2222 failed (%v); is test_sftp up?", err)
	}

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(knownHostsPath, knownHosts, 0o600); err != nil {
		t.Fatalf("failed to write known_hosts file: %v", err)
	}

	config := kopia.LoadIntegrationConfig(t, "config.sftp.json")
	config.SFTP.KnownHostsPath = knownHostsPath

	backend := kopia.NewIntegrationBackend(t, config)

	kopia.RunBackupRestoreRoundTrip(t, backend)
}
